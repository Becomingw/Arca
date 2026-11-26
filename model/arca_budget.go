package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	arcaBudgetPending  = "pending"
	arcaBudgetSettled  = "settled"
	arcaBudgetReleased = "released"
)

var ErrArcaBudgetExceeded = errors.New("request exceeds Arca budget")

// ArcaBudgetAccount supplies one serialization point for all windows and
// concurrent API keys belonging to the same user.
type ArcaBudgetAccount struct {
	UserID   int   `gorm:"primaryKey;autoIncrement:false"`
	Revision int64 `gorm:"not null"`
}

type ArcaBudgetReservation struct {
	UserID        int     `gorm:"primaryKey;autoIncrement:false;index:idx_arca_budget_user_time,priority:1;index:idx_arca_budget_reconcile,priority:1"`
	RequestID     string  `gorm:"primaryKey;size:128"`
	Model         string  `gorm:"size:255;not null"`
	ModelIdentity string  `gorm:"size:64;not null"`
	AdmittedAt    int64   `gorm:"not null;index:idx_arca_budget_user_time,priority:2"`
	ExpiresAt     int64   `gorm:"not null"`
	NextCheckAt   int64   `gorm:"not null;default:0;index:idx_arca_budget_reconcile,priority:3"`
	Status        string  `gorm:"size:16;not null;index:idx_arca_budget_reconcile,priority:2"`
	CostMicros    int64   `gorm:"not null"`
	QuotaPerUSD   float64 `gorm:"not null"`
}

type ArcaBudgetUsage struct {
	ArcaBudgetWindow
	UsedMicros      int64 `json:"used_micros"`
	ReservedMicros  int64 `json:"reserved_micros"`
	RemainingMicros int64 `json:"remaining_micros"`
}

func arcaBudgetModelIdentity(alias string) string {
	sum := sha256.Sum256([]byte(alias))
	return hex.EncodeToString(sum[:])
}

func arcaBudgetQuotaMicros(quota int, rate float64) (int64, error) {
	if quota < 0 || quota > common.MaxQuota || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, errors.New("invalid Arca budget charge or quota rate")
	}
	micros := decimal.NewFromInt(int64(quota)).Mul(decimal.NewFromInt(arcaUSDScale)).
		DivRound(decimal.NewFromFloat(rate), 0)
	if micros.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return 0, errors.New("Arca budget charge overflows supported range")
	}
	if quota > 0 && micros.IsZero() {
		return 1, nil
	}
	return micros.IntPart(), nil
}

func ReserveArcaBudget(ctx context.Context, userID int, requestID, alias string, quota int, windows []ArcaBudgetWindow, now time.Time) error {
	if DB != nil && LOG_DB != nil && userID > 0 {
		reconcilePendingArcaBudget(ctx, userID, now)
	}
	return reserveArcaBudget(ctx, userID, requestID, alias, quota, windows, now, false)
}

func ResizeArcaBudget(ctx context.Context, userID int, requestID, alias string, quota int, windows []ArcaBudgetWindow, now time.Time) error {
	return reserveArcaBudget(ctx, userID, requestID, alias, quota, windows, now, true)
}

func reserveArcaBudget(ctx context.Context, userID int, requestID, alias string, quota int, windows []ArcaBudgetWindow, now time.Time, resize bool) error {
	if DB == nil || userID <= 0 || requestID == "" || len(requestID) > 128 || alias == "" || len(alias) > 255 {
		return errors.New("invalid Arca budget request identity")
	}
	unix := now.Unix()
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		account := ArcaBudgetAccount{UserID: userID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&account).Error; err != nil {
			return err
		}
		// Take the same write lock before reading the ledger on all dialects,
		// including SQLite which does not implement SELECT FOR UPDATE.
		if err := tx.Model(&account).UpdateColumn("revision", gorm.Expr("revision + 1")).Error; err != nil {
			return err
		}

		reservation := ArcaBudgetReservation{
			UserID: userID, RequestID: requestID, Model: alias, ModelIdentity: arcaBudgetModelIdentity(alias),
			AdmittedAt: unix, ExpiresAt: now.Add(24 * time.Hour).Unix(), Status: arcaBudgetPending,
			QuotaPerUSD: common.QuotaPerUnit,
			NextCheckAt: now.Add(time.Minute).Unix(),
		}
		err := tx.Where("user_id = ? AND request_id = ?", userID, requestID).Take(&reservation).Error
		exists := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if exists != resize || (exists && (reservation.RequestID != requestID || reservation.Model != alias ||
			reservation.Status != arcaBudgetPending || reservation.ExpiresAt <= unix)) {
			return ErrArcaEntitlementConflict
		}
		cost, err := arcaBudgetQuotaMicros(quota, reservation.QuotaPerUSD)
		if err != nil {
			return err
		}
		cost = max(cost, reservation.CostMicros)
		for _, window := range windows {
			used, err := queryArcaBudgetUsage(tx, userID, requestID, window, unix)
			if err != nil {
				return err
			}
			// Subtract in this order so already-exceeded budgets and unexpectedly
			// large totals cannot attempt to overflow by adding another charge.
			if used.UsedMicros > window.LimitMicros || used.ReservedMicros > window.LimitMicros-used.UsedMicros ||
				cost > window.LimitMicros-used.UsedMicros-used.ReservedMicros {
				return ErrArcaBudgetExceeded
			}
		}
		reservation.CostMicros = cost
		if exists {
			return tx.Model(&reservation).UpdateColumn("cost_micros", cost).Error
		}
		return tx.Create(&reservation).Error
	})
}

func FinishArcaBudget(ctx context.Context, userID int, requestID string, actualQuota *int) error {
	return updateArcaBudgetSettlement(ctx, userID, requestID, actualQuota, []string{arcaBudgetPending})
}

// FinishArcaTaskBudget adjusts the initial settlement after the durable
// task transitions to its terminal result. Older tasks have no ledger row.
func FinishArcaTaskBudget(ctx context.Context, userID int, requestID string, actualQuota *int) error {
	return updateArcaBudgetSettlement(ctx, userID, requestID, actualQuota, []string{arcaBudgetPending, arcaBudgetSettled})
}

func updateArcaBudgetSettlement(ctx context.Context, userID int, requestID string, actualQuota *int, eligible []string) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	return updateArcaBudgetSettlementInDB(DB.WithContext(ctx), userID, requestID, actualQuota, eligible, false)
}

func updateArcaBudgetSettlementInDB(db *gorm.DB, userID int, requestID string, actualQuota *int, eligible []string, requireReservation bool) error {
	if userID <= 0 || requestID == "" {
		return errors.New("invalid Arca budget settlement identity")
	}
	query := func() *gorm.DB {
		return db.Model(&ArcaBudgetReservation{}).Where(
			"user_id = ? AND request_id = ? AND status IN ?", userID, requestID, eligible,
		)
	}
	if actualQuota == nil {
		result := query().Updates(map[string]any{"status": arcaBudgetReleased, "cost_micros": 0})
		if result.Error == nil && requireReservation && result.RowsAffected != 1 {
			return ErrArcaEntitlementConflict
		}
		return result.Error
	}
	var reservation ArcaBudgetReservation
	read := query()
	if requireReservation {
		read = lockForUpdate(read)
	}
	if err := read.Take(&reservation).Error; err != nil {
		if !requireReservation && errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if reservation.RequestID != requestID {
		return ErrArcaEntitlementConflict
	}
	cost, err := arcaBudgetQuotaMicros(*actualQuota, reservation.QuotaPerUSD)
	if err != nil {
		return err
	}
	return query().Updates(map[string]any{"status": arcaBudgetSettled, "cost_micros": cost}).Error
}

func ReadArcaBudgetUsage(ctx context.Context, userID int, windows []ArcaBudgetWindow, now time.Time) ([]ArcaBudgetUsage, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	usage := make([]ArcaBudgetUsage, 0, len(windows))
	for _, window := range windows {
		row, err := queryArcaBudgetUsage(DB.WithContext(ctx), userID, "", window, now.Unix())
		if err != nil {
			return nil, err
		}
		usage = append(usage, row)
	}
	return usage, nil
}

func queryArcaBudgetUsage(db *gorm.DB, userID int, exclude string, window ArcaBudgetWindow, now int64) (ArcaBudgetUsage, error) {
	result := ArcaBudgetUsage{ArcaBudgetWindow: window}
	// Pending estimates survive a gateway crash. Discarding them at the
	// timeout would permit a crashed request to erase already-incurred cost.
	// Success or explicit refund replaces this estimate, and otherwise the
	// applicable accounting window ages it out.
	q := db.Model(&ArcaBudgetReservation{}).Where("user_id = ? AND admitted_at >= ? AND admitted_at <= ?", userID, window.Start, now).
		Where("status IN ?", []string{arcaBudgetSettled, arcaBudgetPending})
	if window.End != 0 {
		q = q.Where("admitted_at < ?", window.End)
	}
	if exclude != "" {
		q = q.Where("request_id <> ?", exclude)
	}
	if len(window.Models) > 0 {
		models := make([]string, 0, len(window.Models))
		for _, name := range window.Models {
			models = append(models, arcaBudgetModelIdentity(name))
		}
		q = q.Where("model_identity IN ?", models)
	}
	var sums struct {
		Used     int64
		Reserved int64
	}
	err := q.Select(
		"COALESCE(SUM(CASE WHEN status = ? THEN cost_micros ELSE 0 END), 0) AS used, "+
			"COALESCE(SUM(CASE WHEN status = ? THEN cost_micros ELSE 0 END), 0) AS reserved",
		arcaBudgetSettled, arcaBudgetPending,
	).Scan(&sums).Error
	result.UsedMicros, result.ReservedMicros = sums.Used, sums.Reserved
	result.RemainingMicros = max(0, max(0, window.LimitMicros-sums.Used)-sums.Reserved)
	return result, err
}

// A quoted charge is valid recovery evidence only when the same native
// consumption log confirms its owner, server request ID and amount.
type arcaBudgetChargeFact struct {
	UserID    int
	RequestID string
	Quota     int
}

const arcaBudgetChargeFactKey = "arca_budget_charge_fact"

func MarkArcaBudgetChargeFact(c *gin.Context, userID int, requestID string, quota int) {
	if c != nil && userID > 0 && requestID != "" && quota >= 0 && quota <= common.MaxQuota {
		c.Set(arcaBudgetChargeFactKey, arcaBudgetChargeFact{UserID: userID, RequestID: requestID, Quota: quota})
	}
}

func attachArcaBudgetChargeFact(c *gin.Context, userID int, requestID string, quota int, other *LogOther) *LogOther {
	raw, ok := c.Get(arcaBudgetChargeFactKey)
	fact, matched := raw.(arcaBudgetChargeFact)
	if !ok || !matched || fact.UserID != userID || fact.RequestID != requestID || fact.Quota != quota {
		return other
	}
	if other == nil {
		other = NewLogOther()
	}
	other.SetAdmin("arca_budget_priced_quota", fact.Quota)
	return other
}

func reconcilePendingArcaBudget(ctx context.Context, userID int, now time.Time) {
	// Best-effort and bounded. A missing log leaves the estimated reservation
	// in place instead of releasing it or fabricating a final amount.
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	const reservationBatch = 16
	const logBatch = 64
	unix := now.Unix()
	oldest := now.AddDate(0, 0, -35).Unix()
	db := DB.WithContext(ctx)
	var candidates []string
	if err := db.Model(&ArcaBudgetReservation{}).
		Where("user_id = ? AND status = ? AND admitted_at >= ? AND next_check_at <= ?", userID, arcaBudgetPending, oldest, unix).
		Order("next_check_at ASC").Limit(reservationBatch).Pluck("request_id", &candidates).Error; err != nil || len(candidates) == 0 {
		return
	}
	claim := db.Model(&ArcaBudgetReservation{}).
		Where("user_id = ? AND status = ? AND request_id IN ? AND next_check_at <= ?", userID, arcaBudgetPending, candidates, unix).
		UpdateColumn("next_check_at", now.Add(5*time.Minute).Unix())
	if claim.Error != nil || claim.RowsAffected == 0 {
		return
	}
	var logs []struct {
		RequestId string
		Quota     int
		Other     string
	}
	if err := LOG_DB.WithContext(ctx).Model(&Log{}).Select("request_id", "quota", "other").
		Where("user_id = ? AND type = ? AND created_at >= ? AND request_id IN ?", userID, LogTypeConsume, oldest, candidates).
		Where("other LIKE ?", "%arca_budget_priced_quota%").
		Limit(logBatch + 1).Find(&logs).Error; err != nil || len(logs) > logBatch {
		return
	}
	allowed := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		allowed[candidate] = true
	}
	facts := make(map[string]int, len(candidates))
	disputed := make(map[string]bool)
	for _, row := range logs {
		if !allowed[row.RequestId] || row.Quota < 0 || row.Quota > common.MaxQuota {
			continue
		}
		var metadata struct {
			AdminInfo struct {
				PricedQuota *int `json:"arca_budget_priced_quota"`
			} `json:"admin_info"`
		}
		if err := common.UnmarshalJsonStr(row.Other, &metadata); err != nil ||
			metadata.AdminInfo.PricedQuota == nil || *metadata.AdminInfo.PricedQuota != row.Quota {
			continue
		}
		if previous, found := facts[row.RequestId]; found && previous != row.Quota {
			disputed[row.RequestId] = true
		}
		facts[row.RequestId] = row.Quota
	}
	for requestID, actual := range facts {
		if !disputed[requestID] {
			_ = FinishArcaBudget(ctx, userID, requestID, &actual)
		}
	}
}
