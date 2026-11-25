package model

import "context"

type ArcaMCPChannel struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Type         int    `json:"type"`
	Status       int    `json:"status"`
	TestTime     int64  `json:"last_test_at"`
	ResponseTime int    `json:"latency_ms"`
}

type ArcaMCPModel struct {
	ID        int    `json:"id"`
	ModelName string `json:"model_name"`
	VendorID  int    `json:"vendor_id"`
	Status    int    `json:"status"`
}

func ReadArcaMCPChannels(ctx context.Context, offset, limit int) ([]ArcaMCPChannel, error) {
	channels := make([]ArcaMCPChannel, 0)
	err := arcaFeedbackDB(ctx).Model(&Channel{}).
		Select("id", "name", "type", "status", "test_time", "response_time").
		Order("id").Offset(offset).Limit(limit).Find(&channels).Error
	return channels, err
}

func ReadArcaMCPModels(ctx context.Context, offset, limit int) ([]ArcaMCPModel, error) {
	models := make([]ArcaMCPModel, 0)
	err := arcaFeedbackDB(ctx).Model(&Model{}).
		Select("id", "model_name", "vendor_id", "status").
		Order("id").Offset(offset).Limit(limit).Find(&models).Error
	return models, err
}

func ReadArcaMCPHealth(ctx context.Context) map[string]string {
	primary := "unavailable"
	logs := "unavailable"
	if DB != nil {
		if db, err := DB.DB(); err == nil && db.PingContext(ctx) == nil {
			primary = "available"
		}
	}
	if LOG_DB != nil {
		if db, err := LOG_DB.DB(); err == nil && db.PingContext(ctx) == nil {
			logs = "available"
		}
	}
	return map[string]string{"primary_database": primary, "log_database": logs}
}
