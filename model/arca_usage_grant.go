package model

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// ArcaUsageGrantSpec is copied from a code into an immutable user grant.
// Family membership is resolved at issuance so a later policy edit cannot
// silently expand an already distributed code.
type ArcaUsageGrantSpec struct {
	Models          []string `json:"models,omitempty"`
	Family          string   `json:"family,omitempty"`
	StartsAt        int64    `json:"starts_at,omitempty"`
	EndsAt          int64    `json:"ends_at,omitempty"`
	ValidForSeconds int64    `json:"valid_for_seconds,omitempty"`
}

type ArcaUsageGrant struct {
	ID           int      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	UserID       int      `json:"-" gorm:"not null;index:idx_arca_grant_user_time,priority:1"`
	Name         string   `json:"name" gorm:"size:80;not null"`
	GrantedAt    int64    `json:"granted_at" gorm:"not null"`
	StartsAt     int64    `json:"starts_at" gorm:"not null;index:idx_arca_grant_user_time,priority:2"`
	EndsAt       int64    `json:"ends_at" gorm:"not null"`
	AmountMicros int64    `json:"amount_micros" gorm:"not null"`
	Models       []string `json:"models,omitempty" gorm:"serializer:json;type:text"`
	Family       string   `json:"family,omitempty" gorm:"size:128"`
}

type ArcaUsageGrantBalance struct {
	ArcaUsageGrant
	RemainingMicros int64 `json:"remaining_micros"`
}

// ArcaBudgetGrantUse attributes a portion of a request to one grant. All
// adjustments of these rows serialize on the same per-user budget account.
type ArcaBudgetGrantUse struct {
	UserID    int    `gorm:"primaryKey;autoIncrement:false;index:idx_arca_grant_spend,priority:1"`
	RequestID string `gorm:"primaryKey;size:128"`
	GrantID   int    `gorm:"primaryKey;autoIncrement:false;index:idx_arca_grant_spend,priority:2"`
	Position  int    `gorm:"not null"`
	Micros    int64  `gorm:"not null"`
}

var ErrArcaGrantUnavailable = errors.New("redemption code is invalid, unavailable or expired")

// ResolveArcaUsageGrantSpec validates issuer input and snapshots a selected
// family. Model aliases are the same public aliases used by the relay.
func ResolveArcaUsageGrantSpec(ctx context.Context, spec, previous *ArcaUsageGrantSpec, now time.Time) (*ArcaUsageGrantSpec, error) {
	if spec == nil {
		return nil, nil
	}
	result := *spec
	latest := now.AddDate(10, 0, 0).Unix()
	if result.StartsAt < 0 || result.EndsAt < 0 || result.StartsAt > latest || result.EndsAt > latest || result.ValidForSeconds < 0 ||
		result.ValidForSeconds > int64(365*24*time.Hour/time.Second) ||
		(result.EndsAt != 0 && result.EndsAt <= max(result.StartsAt, now.Unix())) ||
		(result.Family != "" && len(result.Models) != 0) || len(result.Models) > 100 {
		return nil, errors.New("invalid usage grant scope or time window")
	}
	if result.Family != "" && previous != nil && previous.Family == result.Family && len(previous.Models) > 0 {
		result.Models = previous.Models
	} else if result.Family != "" {
		policy, _, err := LoadArcaEntitlementPolicy(ctx)
		if err != nil {
			return nil, err
		}
		family, ok := policy.ModelFamilies[result.Family]
		if !ok || len(family.Models) == 0 {
			return nil, errors.New("unknown or empty model family")
		}
		result.Models = family.Models
	}
	result.Models = slices.Clone(result.Models)
	for i, alias := range result.Models {
		if len(alias) == 0 || len(alias) > 255 || alias == "*" || strings.TrimSpace(alias) != alias || strings.ContainsAny(alias, "\r\n\x00") {
			return nil, errors.New("invalid usage grant model alias")
		}
		result.Models[i] = alias
	}
	slices.Sort(result.Models)
	result.Models = slices.Compact(result.Models)
	return &result, nil
}

// RedeemArcaUsageGrant consumes a code and creates its non-transferable grant
// in one transaction. The first write serializes the claim even on SQLite.
func RedeemArcaUsageGrant(ctx context.Context, userID int, key string, now time.Time) (ArcaUsageGrantBalance, error) {
	var grant ArcaUsageGrantBalance
	if userID <= 0 || len(key) != 32 || DB == nil {
		return grant, ErrArcaGrantUnavailable
	}
	unix := now.Unix()
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockArcaBudgetAccount(tx, userID); err != nil {
			return err
		}
		keyMatch := clause.Eq{Column: clause.Column{Name: "key"}, Value: key}
		result := tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Model(&Redemption{}).Where(keyMatch).
			Where("status = ? AND (expired_time = 0 OR expired_time > ?)", common.RedemptionCodeStatusEnabled, unix).
			Updates(map[string]any{"status": common.RedemptionCodeStatusUsed, "used_user_id": userID, "redeemed_time": unix})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrArcaGrantUnavailable
		}
		var code Redemption
		if err := tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Where(keyMatch).Take(&code).Error; err != nil {
			return err
		}
		amount, err := arcaBudgetQuotaMicros(code.Quota, common.QuotaPerUnit)
		if err != nil || amount <= 0 {
			return ErrArcaGrantUnavailable
		}
		entry := ArcaUsageGrant{ID: code.Id, UserID: userID, Name: code.Name, GrantedAt: unix, StartsAt: unix, AmountMicros: amount}
		if spec := code.Grant; spec != nil {
			entry.StartsAt = max(unix, spec.StartsAt)
			entry.EndsAt = spec.EndsAt
			if spec.ValidForSeconds > 0 {
				deadline := entry.StartsAt + spec.ValidForSeconds
				if entry.EndsAt == 0 || deadline < entry.EndsAt {
					entry.EndsAt = deadline
				}
			}
			entry.Models, entry.Family = spec.Models, spec.Family
		}
		if entry.EndsAt != 0 && entry.EndsAt <= entry.StartsAt {
			return ErrArcaGrantUnavailable
		}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		grant = ArcaUsageGrantBalance{ArcaUsageGrant: entry, RemainingMicros: amount}
		return nil
	})
	return grant, err
}

func readArcaGrantSpending(db *gorm.DB, userID int, ids []int, exclude string) (map[int]int64, error) {
	spent := make(map[int]int64, len(ids))
	if len(ids) == 0 {
		return spent, nil
	}
	var rows []struct {
		GrantID int
		Total   int64
	}
	requests := db.Model(&ArcaBudgetReservation{}).Select("request_id").
		Where("user_id = ? AND status IN ?", userID, []string{arcaBudgetPending, arcaBudgetSettled})
	q := db.Model(&ArcaBudgetGrantUse{}).
		Where("user_id = ? AND grant_id IN ? AND request_id IN (?)", userID, ids, requests)
	if exclude != "" {
		q = q.Where("request_id <> ?", exclude)
	}
	if err := q.Select("grant_id, SUM(micros) AS total").Group("grant_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		spent[row.GrantID] = row.Total
	}
	return spent, nil
}

func ListArcaUsageGrants(ctx context.Context, userID int, now time.Time) ([]ArcaUsageGrantBalance, error) {
	db := DB.WithContext(ctx)
	var grants []ArcaUsageGrant
	if err := db.Where("user_id = ? AND (ends_at = 0 OR ends_at > ?)", userID, now.Unix()).Order("id desc").Limit(100).Find(&grants).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(grants))
	for _, grant := range grants {
		ids = append(ids, grant.ID)
	}
	spent, err := readArcaGrantSpending(db, userID, ids, "")
	if err != nil {
		return nil, err
	}
	result := make([]ArcaUsageGrantBalance, 0, len(grants))
	for _, grant := range grants {
		result = append(result, ArcaUsageGrantBalance{ArcaUsageGrant: grant, RemainingMicros: max(0, grant.AmountMicros-spent[grant.ID])})
	}
	return result, nil
}

func allocateArcaGrantShortfall(tx *gorm.DB, reservation ArcaBudgetReservation, shortfall int64, publicAlias string, allowPartial bool) ([]ArcaBudgetGrantUse, error) {
	if shortfall == 0 {
		return nil, nil
	}
	var grants []ArcaUsageGrant
	if err := tx.Where("user_id = ? AND starts_at <= ? AND (ends_at = 0 OR ends_at > ?)",
		reservation.UserID, reservation.AdmittedAt, reservation.AdmittedAt).Find(&grants).Error; err != nil {
		return nil, err
	}
	grants = slices.DeleteFunc(grants, func(g ArcaUsageGrant) bool {
		return len(g.Models) != 0 && !slices.Contains(g.Models, publicAlias)
	})
	deadline := func(grant ArcaUsageGrant) int64 {
		if grant.EndsAt == 0 {
			return math.MaxInt64
		}
		return grant.EndsAt
	}
	slices.SortFunc(grants, func(a, b ArcaUsageGrant) int {
		if deadline(a) < deadline(b) {
			return -1
		}
		if deadline(a) > deadline(b) {
			return 1
		}
		if len(a.Models) != 0 && len(b.Models) == 0 {
			return -1
		}
		if len(a.Models) == 0 && len(b.Models) != 0 {
			return 1
		}
		return a.ID - b.ID
	})
	ids := make([]int, 0, len(grants))
	for _, grant := range grants {
		ids = append(ids, grant.ID)
	}
	spent, err := readArcaGrantSpending(tx, reservation.UserID, ids, reservation.RequestID)
	if err != nil {
		return nil, err
	}
	var uses []ArcaBudgetGrantUse
	for _, grant := range grants {
		amount := min(shortfall, max(0, grant.AmountMicros-spent[grant.ID]))
		if amount == 0 {
			continue
		}
		uses = append(uses, ArcaBudgetGrantUse{UserID: reservation.UserID, RequestID: reservation.RequestID, GrantID: grant.ID, Position: len(uses), Micros: amount})
		shortfall -= amount
		if shortfall == 0 {
			return uses, nil
		}
	}
	if allowPartial {
		return uses, nil
	}
	return nil, ErrArcaBudgetExceeded
}
