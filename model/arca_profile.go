package model

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ArcaProfile struct {
	UserID               int     `gorm:"primaryKey;autoIncrement:false"`
	AvatarHandle         *string `gorm:"size:36;uniqueIndex"`
	AvatarPNG            []byte
	LeaderboardAnonymous bool `gorm:"not null"`
}

type ArcaActivityWindow struct {
	UserID   int   `gorm:"primaryKey;autoIncrement:false"`
	BucketAt int64 `gorm:"primaryKey;autoIncrement:false;index"`
}

var arcaActivityEnabled atomic.Bool

func EnableArcaActivity() {
	arcaActivityEnabled.Store(true)
}

func ReadArcaProfile(ctx context.Context, userID int) (ArcaProfile, error) {
	var profile ArcaProfile
	err := DB.WithContext(ctx).Select("user_id", "avatar_handle", "leaderboard_anonymous").First(&profile, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return profile, err
}

// arcaWithActiveUser serializes profile and activity writes with deletion so
// requests already in flight cannot recreate rows for removed users.
func arcaWithActiveUser(ctx context.Context, userID int, write func(*gorm.DB) error) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id").Where("id = ? AND status = ?", userID, common.UserStatusEnabled).Take(&user).Error; err != nil {
			return err
		}
		return write(tx)
	})
}

func StoreArcaAvatar(ctx context.Context, userID int, handle *string, png []byte) error {
	return arcaWithActiveUser(ctx, userID, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"avatar_handle", "avatar_png"}),
		}).Create(&ArcaProfile{UserID: userID, AvatarHandle: handle, AvatarPNG: png}).Error
	})
}

func StoreArcaActivityPrivacy(ctx context.Context, userID int, anonymous bool) error {
	return arcaWithActiveUser(ctx, userID, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"leaderboard_anonymous"}),
		}).Create(&ArcaProfile{UserID: userID, LeaderboardAnonymous: anonymous}).Error
	})
}

func ReadArcaAvatar(ctx context.Context, handle string) ([]byte, error) {
	db := DB.WithContext(ctx)
	activeUsers := db.Model(&User{}).Select("id").Where("status = ?", common.UserStatusEnabled)
	var profile ArcaProfile
	err := db.Select("avatar_png").Where("avatar_handle = ? AND user_id IN (?)", handle, activeUsers).Take(&profile).Error
	return profile.AvatarPNG, err
}

func ReadArcaAvatarHandles(ctx context.Context, viewerID int, userIDs []int) (map[int]string, error) {
	db := DB.WithContext(ctx)
	handles := make(map[int]string)
	if len(userIDs) == 0 {
		return handles, nil
	}
	topics := db.Model(&ArcaFeedbackIssue{}).Where("visibility = ? AND deleted_at IS NULL", ArcaFeedbackPublic)
	authors := topics.Session(&gorm.Session{}).Select("owner_id")
	issueIDs := topics.Session(&gorm.Session{}).Select("id")
	participants := db.Model(&ArcaFeedbackEvent{}).Select("actor_id").
		Where("issue_id IN (?) AND visibility = ? AND deleted_at IS NULL", issueIDs, ArcaFeedbackPublic)
	activeUsers := db.Model(&User{}).Select("id").Where("status = ? AND id IN ?", common.UserStatusEnabled, userIDs).
		Where("id = ? OR id IN (?) OR id IN (?)", viewerID, authors, participants)
	var profiles []ArcaProfile
	err := db.Select("user_id", "avatar_handle").
		Where("avatar_handle IS NOT NULL AND user_id IN (?)", activeUsers).Find(&profiles).Error
	for _, profile := range profiles {
		handles[profile.UserID] = *profile.AvatarHandle
	}
	return handles, err
}

func RecordArcaActivity(ctx context.Context, userID int, startedAt int64) error {
	if !arcaActivityEnabled.Load() || userID <= 0 || startedAt <= 0 {
		return nil
	}
	window := ArcaActivityWindow{UserID: userID, BucketAt: startedAt - startedAt%300}
	err := arcaWithActiveUser(ctx, userID, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&window).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

const ArcaActivityMinimumMembers = 3

const arcaActivityDay = "bucket_at - (bucket_at % 86400)"
const arcaActivityBand = "(bucket_at % 86400) - (bucket_at % 21600)"

type ArcaActivityMember struct {
	UserID  int
	Windows int64
}

type ArcaActivityDay struct {
	UserID int
	DayAt  int64
}

type ArcaActivityBand struct {
	StartSecond int64 `json:"start_second"`
	Windows     int64 `json:"windows"`
}

type ArcaMemberUsage struct {
	UserID   int
	Requests int64 `json:"requests"`
	Tokens   int64 `json:"tokens"`
}

type ArcaModelUsage struct {
	Model    string `json:"model"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
}

func arcaActivityActiveUsers(db *gorm.DB) *gorm.DB {
	return db.Model(&User{}).Select("id").Where("status = ?", common.UserStatusEnabled)
}

func ReadArcaActivity(ctx context.Context, start, end int64) ([]ArcaActivityMember, error) {
	db := DB.WithContext(ctx)
	var members []ArcaActivityMember
	err := db.Model(&ArcaActivityWindow{}).Select("user_id, COUNT(*) AS windows").
		Where("bucket_at >= ? AND bucket_at < ? AND user_id IN (?)", start, end, arcaActivityActiveUsers(db)).
		Group("user_id").Order("windows DESC, user_id ASC").Limit(20).Find(&members).Error
	return members, err
}

func ReadArcaActivityDays(ctx context.Context, userIDs []int, start, end int64) ([]ArcaActivityDay, error) {
	var days []ArcaActivityDay
	err := DB.WithContext(ctx).Model(&ArcaActivityWindow{}).Select("user_id, "+arcaActivityDay+" AS day_at").
		Where("bucket_at >= ? AND bucket_at < ? AND user_id IN ?", start, end, userIDs).
		Group("user_id, " + arcaActivityDay).Order("user_id ASC, day_at ASC").Find(&days).Error
	return days, err
}

func ReadArcaActivityBands(ctx context.Context, start, end int64) ([]ArcaActivityBand, error) {
	db := DB.WithContext(ctx)
	bands := make([]ArcaActivityBand, 0, 4)
	err := db.Model(&ArcaActivityWindow{}).Select(arcaActivityBand+" AS start_second, COUNT(*) AS windows").
		Where("bucket_at >= ? AND bucket_at < ? AND user_id IN (?)", start, end, arcaActivityActiveUsers(db)).
		Group(arcaActivityBand).Having("COUNT(DISTINCT user_id) >= ?", ArcaActivityMinimumMembers).
		Order("start_second ASC").Find(&bands).Error
	return bands, err
}

func ReadArcaMemberUsage(ctx context.Context, userIDs []int, start, end int64) ([]ArcaMemberUsage, error) {
	var usage []ArcaMemberUsage
	err := DB.WithContext(ctx).Model(&QuotaData{}).Select("user_id, SUM(count) AS requests, COALESCE(SUM(token_used), 0) AS tokens").
		Where("created_at >= ? AND created_at < ? AND user_id IN ? AND count > 0", start, end, userIDs).
		Group("user_id").Find(&usage).Error
	return usage, err
}

func ReadArcaPopularModels(ctx context.Context, start, end int64) ([]ArcaModelUsage, error) {
	db := DB.WithContext(ctx)
	models := make([]ArcaModelUsage, 0, 10)
	err := db.Model(&QuotaData{}).Select("model_name AS model, SUM(count) AS requests, COALESCE(SUM(token_used), 0) AS tokens").
		Where("created_at >= ? AND created_at < ? AND user_id IN (?) AND model_name <> '' AND count > 0", start, end, arcaActivityActiveUsers(db)).
		Group("model_name").Having("COUNT(DISTINCT user_id) >= ?", ArcaActivityMinimumMembers).
		Order("requests DESC, model_name ASC").Limit(10).Find(&models).Error
	return models, err
}

func ReadArcaActivityNames(ctx context.Context, userIDs []int) (map[int]string, error) {
	db := DB.WithContext(ctx)
	hiddenProfiles := db.Model(&ArcaProfile{}).Select("user_id").Where("leaderboard_anonymous = ?", true)
	var users []User
	err := db.Select("id", "username").Where("status = ? AND id IN ? AND id NOT IN (?)",
		common.UserStatusEnabled, userIDs, hiddenProfiles).Find(&users).Error
	names := make(map[int]string, len(users))
	for _, user := range users {
		names[user.Id] = user.Username
	}
	return names, err
}
