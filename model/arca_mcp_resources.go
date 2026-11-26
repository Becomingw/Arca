package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

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

type ArcaMCPUser struct {
	ID     int    `json:"id"`
	Group  string `json:"group"`
	Role   int    `json:"role"`
	Status int    `json:"status"`
}

func ReadArcaMCPUsers(ctx context.Context, group, username string, offset, limit int) ([]ArcaMCPUser, error) {
	users := []ArcaMCPUser{}
	query := arcaFeedbackDB(ctx).Model(&User{}).Select([]string{"id", "group", "role", "status"})
	if group != "" {
		query = query.Where(clause.Eq{Column: clause.Column{Name: "group"}, Value: group})
	}
	if username != "" {
		query = query.Where("username = ?", username)
	}
	err := query.Order("id").Offset(offset).Limit(limit).Find(&users).Error
	return users, err
}

func ReadArcaMCPUser(ctx context.Context, id int) (ArcaMCPUser, error) {
	var user ArcaMCPUser
	err := arcaFeedbackDB(ctx).Model(&User{}).Select([]string{"id", "group", "role", "status"}).Take(&user, "id = ?", id).Error
	return user, err
}

type ArcaMCPProvider struct {
	ArcaMCPChannel
	Priority            int64  `json:"priority"`
	Weight              int    `json:"weight"`
	MultiKey            bool   `json:"multi_key"`
	KeyCount            int    `json:"key_count"`
	EnabledKeys         int    `json:"enabled_keys"`
	DisabledKeys        int    `json:"disabled_keys"`
	QuotaProbeStatus    string `json:"quota_probe_status"`
	LastQuotaSuccessAt  int64  `json:"last_quota_success_at"`
	QuotaWindows        int    `json:"quota_windows"`
	KeysWithQuotaErrors int    `json:"keys_with_quota_errors"`
}

// ReadArcaMCPProvider reports persisted observations, including credential
// counts, without selecting the credentials or contact details.
func ReadArcaMCPProvider(ctx context.Context, id int) (ArcaMCPProvider, error) {
	var channel Channel
	err := arcaFeedbackDB(ctx).Select([]string{"id", "name", "type", "status", "test_time", "response_time",
		"priority", "weight", "channel_info", "balance_updated_time", "other_info"}).Take(&channel, "id = ?", id).Error
	if err != nil {
		return ArcaMCPProvider{}, err
	}
	result := ArcaMCPProvider{
		ArcaMCPChannel: ArcaMCPChannel{ID: channel.Id, Name: channel.Name, Type: channel.Type, Status: channel.Status, TestTime: channel.TestTime, ResponseTime: channel.ResponseTime},
		Priority:       channel.GetPriority(), Weight: channel.GetWeight(), MultiKey: channel.ChannelInfo.IsMultiKey,
		QuotaProbeStatus: "not_recorded", LastQuotaSuccessAt: channel.BalanceUpdatedTime,
	}
	if channel.BalanceUpdatedTime > 0 {
		result.QuotaProbeStatus = "available"
	}
	if info := channel.ChannelInfo; info.IsMultiKey {
		result.KeyCount = max(0, info.MultiKeySize)
		for index, status := range info.MultiKeyStatusList {
			if index >= 0 && index < result.KeyCount && status != common.ChannelStatusEnabled {
				result.DisabledKeys++
			}
		}
		result.EnabledKeys = result.KeyCount - result.DisabledKeys
	}
	if detail := channel.GetBalanceDetail(); detail != nil {
		result.QuotaProbeStatus = "available"
		if detail.Error != "" {
			result.QuotaProbeStatus = "error"
		}
		result.QuotaWindows = len(detail.Windows)
		for _, key := range detail.Keys {
			if key.Error != "" {
				result.KeysWithQuotaErrors++
			}
		}
	}
	return result, nil
}

type ArcaMCPModelRoute struct {
	ChannelID     int    `json:"channel_id"`
	Enabled       bool   `json:"enabled"`
	Priority      *int64 `json:"priority"`
	Weight        uint   `json:"weight"`
	ChannelStatus int    `json:"channel_status"`
}

func ReadArcaMCPModelRoutes(ctx context.Context, group, alias string, offset, limit int) ([]ArcaMCPModelRoute, error) {
	routes := []ArcaMCPModelRoute{}
	db := arcaFeedbackDB(ctx)
	err := db.Model(&Ability{}).Select("channel_id", "enabled", "priority", "weight").
		Where(clause.Eq{Column: clause.Column{Name: "group"}, Value: group}).Where("model = ?", alias).
		Order("COALESCE(priority, 0) DESC, weight DESC, channel_id ASC").Offset(offset).Limit(limit).Find(&routes).Error
	if err != nil || len(routes) == 0 {
		return routes, err
	}
	channelIDs := make([]int, 0, len(routes))
	for _, route := range routes {
		channelIDs = append(channelIDs, route.ChannelID)
	}
	var channels []ArcaMCPChannel
	if err := db.Model(&Channel{}).Select("id", "status").Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	statuses := make(map[int]int, len(channels))
	for _, channel := range channels {
		statuses[channel.ID] = channel.Status
	}
	for i := range routes {
		routes[i].ChannelStatus = statuses[routes[i].ChannelID]
	}
	return routes, nil
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
