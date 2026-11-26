package model

import (
	"context"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

const arcaOpenCodeChatPath = "/v1/chat/completions"

type arcaOpenCodeConnection struct {
	Channel
	ModelName string
}

// ArcaOpenCodeChatModels selects channel model names whose group and public
// Chat Completions route are enabled. It reads no channel credential columns.
func ArcaOpenCodeChatModels(ctx context.Context, groups []string) ([]string, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	var rows []arcaOpenCodeConnection
	err := DB.WithContext(ctx).Table("abilities").
		Select("abilities.model AS model_name, channels.type, channels.settings, channels.setting, channels.model_mapping").
		Joins("JOIN channels ON abilities.channel_id = channels.id").
		Where("abilities."+commonGroupCol+" IN ? AND abilities.enabled = ? AND channels.status = ?", groups, true, common.ChannelStatusEnabled).
		Distinct().Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if isNonChatOpenCodeModel(row.ModelName) {
			continue
		}
		if row.Type == constant.ChannelTypeTaskPlugin || constant.IsAdvancedCustomChannel(row.Type) {
			filters := []dto.ChannelFilter{
				{Kind: dto.FilterRequestPath, RequestPath: arcaOpenCodeChatPath},
				{Kind: dto.FilterTaskPluginIdentity, RequestPath: arcaOpenCodeChatPath},
			}
			if ok, _ := ChannelSatisfiesFilters(&row.Channel, row.ModelName, filters); !ok {
				continue
			}
		} else if !slices.Contains(common.GetEndpointTypesByChannelType(row.Type, row.ModelName), constant.EndpointTypeOpenAI) {
			continue
		}
		names = append(names, row.ModelName)
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

// Ordinary OpenAI channels use one name list for several API types. These
// advertised upstream model families are never Chat Completions models.
func isNonChatOpenCodeModel(name string) bool {
	name = strings.ToLower(name)
	return common.IsImageGenerationModel(name) ||
		strings.HasPrefix(name, "text-embedding-") ||
		strings.HasPrefix(name, "whisper-") ||
		strings.HasPrefix(name, "tts-") ||
		strings.HasSuffix(name, "-transcribe") ||
		strings.HasSuffix(name, "-tts")
}
