package model

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func ArcaFeedbackDeliveryQueue(ctx context.Context, now int64) ([]ArcaFeedbackNotification, error) {
	var notices []ArcaFeedbackNotification
	err := arcaFeedbackDB(ctx).Where("delivery_state = ? AND delivery_at <= ?", "pending", now).
		Order("delivery_at, id").Limit(10).Find(&notices).Error
	return notices, err
}

// ClaimArcaFeedbackDelivery advances the due time while an external delivery is
// in flight. A delayed worker cannot finish a subsequently renewed lease.
func ClaimArcaFeedbackDelivery(ctx context.Context, noticeID, now int64) (string, error) {
	lease := uuid.NewString()
	result := arcaFeedbackDB(ctx).Model(&ArcaFeedbackNotification{}).
		Where("id = ? AND delivery_state = ? AND delivery_at <= ?", noticeID, "pending", now).
		UpdateColumns(map[string]any{"delivery_at": now + 120, "delivery_lease": lease})
	if result.RowsAffected == 0 || result.Error != nil {
		return "", result.Error
	}
	return lease, nil
}

func FinishArcaFeedbackDelivery(ctx context.Context, notice ArcaFeedbackNotification, lease string, now int64, delivered bool) error {
	updates := map[string]any{"delivery_attempts": notice.DeliveryAttempts + 1, "delivery_lease": nil}
	switch {
	case delivered:
		updates["delivery_state"], updates["delivery_at"] = "complete", nil
	case notice.DeliveryAttempts >= 4:
		updates["delivery_state"], updates["delivery_at"] = "failed", nil
	default:
		updates["delivery_at"] = now + (int64(60) << min(notice.DeliveryAttempts, 4))
	}
	return arcaFeedbackDB(ctx).Model(&ArcaFeedbackNotification{}).
		Where("id = ? AND delivery_lease = ?", notice.ID, lease).UpdateColumns(updates).Error
}

func ArcaFeedbackDeliveryStillVisible(ctx context.Context, id string) (bool, error) {
	var issue ArcaFeedbackIssue
	err := findArcaFeedbackIssue(arcaFeedbackDB(ctx).Select("id"), id, &issue)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}
