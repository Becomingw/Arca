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

type ArcaActivityMember struct {
	UserID  int
	Windows int64
}

func ReadArcaActivity(ctx context.Context, start, end int64) ([]ArcaActivityMember, error) {
	var members []ArcaActivityMember
	err := DB.WithContext(ctx).Model(&ArcaActivityWindow{}).Select("user_id, COUNT(*) AS windows").
		Where("bucket_at >= ? AND bucket_at < ?", start, end).
		Group("user_id").Order("windows DESC, user_id ASC").Limit(20).Find(&members).Error
	return members, err
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
