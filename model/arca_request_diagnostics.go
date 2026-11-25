package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const arcaRequestDiagnosticRowLimit = 128

// ArcaRequestDiagnosticEvidence records only observations safe to share in a
// public topic. A zero value represents missing or inaccessible evidence.
type ArcaRequestDiagnosticEvidence struct {
	HasUsageLog        bool
	HasErrorLog        bool
	HasChannelAttempt  bool
	HasAttemptFailures bool
	IsStream           bool
}

// ReadArcaRequestDiagnosticEvidence uses the gateway-assigned request ID and
// checks every matching row in the log search window before deriving any
// owner-visible facts.
func ReadArcaRequestDiagnosticEvidence(ctx context.Context, userID int, requestID string) (ArcaRequestDiagnosticEvidence, error) {
	if userID <= 0 {
		return ArcaRequestDiagnosticEvidence{}, nil
	}
	return readArcaRequestDiagnosticEvidence(ctx, userID, requestID)
}

// ReadArcaAdminRequestDiagnosticEvidence still requires one consistent record
// owner but leaves authorization to the administrative handler.
func ReadArcaAdminRequestDiagnosticEvidence(ctx context.Context, requestID string) (ArcaRequestDiagnosticEvidence, error) {
	return readArcaRequestDiagnosticEvidence(ctx, 0, requestID)
}

func readArcaRequestDiagnosticEvidence(ctx context.Context, userID int, requestID string) (ArcaRequestDiagnosticEvidence, error) {
	var evidence ArcaRequestDiagnosticEvidence
	if requestID == "" {
		return evidence, nil
	}
	if LOG_DB == nil {
		return evidence, gorm.ErrInvalidDB
	}
	var rows []struct {
		RequestId string
		UserId    int
		Type      int
		IsStream  bool
		ChannelId int
	}
	start := common.GetTimestamp() - operation_setting.GetLogSetting().MaxRangeSeconds()
	err := LOG_DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard}).
		Model(&Log{}).
		Select("request_id", "user_id", "type", "is_stream", "channel_id").
		Where("request_id = ? AND created_at >= ?", requestID, start).
		Limit(arcaRequestDiagnosticRowLimit + 1).
		Find(&rows).Error
	if err != nil || len(rows) > arcaRequestDiagnosticRowLimit {
		return evidence, err
	}
	for _, row := range rows {
		// Check exact equality even on case-insensitive database collations.
		if userID == 0 {
			userID = row.UserId
		}
		if row.RequestId != requestID || userID <= 0 || row.UserId != userID {
			return ArcaRequestDiagnosticEvidence{}, nil
		}
		switch row.Type {
		case LogTypeConsume:
			evidence.HasUsageLog = true
		case LogTypeError:
			evidence.HasErrorLog = true
			evidence.HasAttemptFailures = evidence.HasAttemptFailures || row.ChannelId > 0
		default:
			continue
		}
		evidence.HasChannelAttempt = evidence.HasChannelAttempt || row.ChannelId > 0
		evidence.IsStream = evidence.IsStream || row.IsStream
	}
	return evidence, nil
}
