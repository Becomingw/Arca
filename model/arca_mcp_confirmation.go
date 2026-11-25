package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrArcaMCPTicketLimit = errors.New("too many pending MCP confirmations")

// ArcaMCPConfirmation binds a short-lived opaque ticket to one operation and
// credential. Only the hash of the ticket is persisted.
type ArcaMCPConfirmation struct {
	TicketHash  string `gorm:"type:varchar(64);primaryKey"`
	BindingHash string `gorm:"type:varchar(64);not null"`
	ActorID     int    `gorm:"not null"`
	TokenID     int    `gorm:"not null;index:idx_arca_mcp_token_expiry,priority:1"`
	ExpiresAt   int64  `gorm:"not null;index;index:idx_arca_mcp_token_expiry,priority:2"`
}

func StoreArcaMCPConfirmation(ctx context.Context, confirmation ArcaMCPConfirmation) error {
	now := common.GetTimestamp()
	return arcaFeedbackDB(ctx).Transaction(func(tx *gorm.DB) error {
		// A token's owner serializes ticket issuance against their other
		// requests. SQLite serializes writers at the database level.
		var user User
		if err := lockForUpdate(tx).Select("id").First(&user, confirmation.ActorID).Error; err != nil {
			return err
		}
		if err := tx.Where("expires_at <= ?", now).Delete(&ArcaMCPConfirmation{}).Error; err != nil {
			return err
		}
		var pending int64
		if err := tx.Model(&ArcaMCPConfirmation{}).Where("token_id = ? AND expires_at > ?", confirmation.TokenID, now).Count(&pending).Error; err != nil {
			return err
		}
		if pending >= 10 {
			return ErrArcaMCPTicketLimit
		}
		return tx.Create(&confirmation).Error
	})
}

// ConsumeArcaMCPConfirmation atomically removes a matching ticket before the
// mutation. This works across replicas, including concurrent retries.
func ConsumeArcaMCPConfirmation(ctx context.Context, confirmation ArcaMCPConfirmation) (bool, error) {
	result := arcaFeedbackDB(ctx).Where("ticket_hash = ? AND binding_hash = ? AND actor_id = ? AND token_id = ? AND expires_at > ?",
		confirmation.TicketHash, confirmation.BindingHash, confirmation.ActorID, confirmation.TokenID, common.GetTimestamp()).
		Delete(&ArcaMCPConfirmation{})
	return result.RowsAffected == 1, result.Error
}
