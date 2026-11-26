package controller

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const arcaEntitlementBodyLimit = 2 << 20

type arcaEntitlementSnapshot struct {
	UserID    int    `json:"user_id"`
	Enabled   bool   `json:"enabled"`
	CostBasis string `json:"cost_basis"`
	Timezone  string `json:"timezone"`
	model.ArcaEffectiveEntitlement
	ModelFamilies    map[string]model.ArcaModelFamily `json:"model_families"`
	Windows          []model.ArcaBudgetUsage          `json:"windows"`
	KeyLimitsEnabled bool                             `json:"key_limits_enabled"`
	KeyModels        []string                         `json:"key_models,omitempty"`
}

func GetArcaEntitlementPolicy(c *gin.Context) {
	policy, version, err := model.LoadArcaEntitlementPolicy(c.Request.Context())
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Database error, please contact the administrator"))
		return
	}
	common.ApiSuccess(c, gin.H{"version": version, "policy": policy})
}

func PutArcaEntitlementPolicy(c *gin.Context) {
	var input struct {
		ExpectedVersion *int64                       `json:"expected_version"`
		Policy          *model.ArcaEntitlementPolicy `json:"policy"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, arcaEntitlementBodyLimit)
	if err := common.DecodeJsonBodyStrict(c.Request.Body, &input); err != nil || input.ExpectedVersion == nil || input.Policy == nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	if err := input.Policy.Validate(); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid input {{error}}", map[string]any{"error": err.Error()}))
		return
	}
	version, err := model.SaveArcaEntitlementPolicy(c.Request.Context(), *input.ExpectedVersion, *input.Policy)
	switch {
	case errors.Is(err, model.ErrArcaEntitlementConflict):
		common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("Invalid parameters"), gin.H{"code": "ARCA_ENTITLEMENT_CONFLICT"})
	case err != nil:
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Database error, please contact the administrator"))
	default:
		common.ApiSuccess(c, gin.H{"version": version, "policy": input.Policy})
	}
}

func GetArcaMyEntitlements(c *gin.Context) {
	policy, _, err := model.LoadArcaEntitlementPolicy(c.Request.Context())
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Database error, please contact the administrator"))
		return
	}
	userID := c.GetInt("id")
	userGroup, err := model.GetUserGroup(userID, false)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Database error, please contact the administrator"))
		return
	}
	snapshot, err := arcaEntitlementPreview(c.Request.Context(), policy, userID, userGroup, nil, time.Now())
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Database error, please contact the administrator"))
		return
	}
	groups := service.GetUserUsableGroups(userGroup)
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	allowed := make(map[string]bool)
	for _, alias := range service.GetGroupsEnabledModels(names) {
		if snapshot.ModelAccess.Allows(alias) {
			allowed[alias] = true
		}
	}
	snapshot.projectPersonalAliases(allowed)
	common.ApiSuccess(c, snapshot)
}

func (s *arcaEntitlementSnapshot) projectPersonalAliases(allowed map[string]bool) {
	hidden := func(name string) bool { return !allowed[name] }
	s.ModelAccess.Models = slices.DeleteFunc(slices.Clone(s.ModelAccess.Models), hidden)
	models := make(map[string]model.ArcaSubPool)
	for alias, allowance := range s.Allocation.Models {
		if alias == "*" || allowed[alias] {
			models[alias] = allowance
		}
	}
	families := make(map[string]model.ArcaSubPool)
	definitions := make(map[string]model.ArcaModelFamily)
	for id, allowance := range s.Allocation.Families {
		definition, ok := s.ModelFamilies[id]
		if !ok {
			continue
		}
		definition.Models = slices.DeleteFunc(slices.Clone(definition.Models), hidden)
		if len(definition.Models) > 0 {
			families[id], definitions[id] = allowance, definition
		}
	}
	s.Allocation.Models = models
	s.Allocation.Families = families
	s.ModelFamilies = definitions
	s.Windows = slices.DeleteFunc(s.Windows, func(usage model.ArcaBudgetUsage) bool {
		switch usage.Scope {
		case "model":
			return !allowed[usage.Name]
		case "family":
			_, visible := definitions[usage.Name]
			return !visible
		default:
			return false
		}
	})
	for i := range s.Windows {
		s.Windows[i].Models = slices.DeleteFunc(slices.Clone(s.Windows[i].Models), hidden)
	}
}

func PreviewArcaEntitlementPolicy(c *gin.Context) {
	var input struct {
		UserID   int                          `json:"user_id"`
		APIKeyID int                          `json:"api_key_id"`
		Config   *model.ArcaEntitlementPolicy `json:"config"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, arcaEntitlementBodyLimit)
	if err := common.DecodeJsonBodyStrict(c.Request.Body, &input); err != nil || input.UserID <= 0 || input.APIKeyID < 0 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	result, status, err := getArcaEntitlementPreview(c.Request.Context(), input.UserID, input.APIKeyID, input.Config)
	if err != nil {
		common.ApiErrorStatus(c, status, err)
		return
	}
	common.ApiSuccess(c, result)
}

func getArcaEntitlementPreview(ctx context.Context, userID, apiKeyID int, proposed *model.ArcaEntitlementPolicy) (gin.H, int, error) {
	if userID <= 0 || apiKeyID < 0 {
		return nil, http.StatusBadRequest, common.NewMessage("Invalid parameters")
	}
	if proposed != nil {
		if err := proposed.Validate(); err != nil {
			return nil, http.StatusBadRequest, common.NewMessage("Invalid input {{error}}", map[string]any{"error": err.Error()})
		}
	}

	unavailable := common.NewMessage("Database error, please contact the administrator")
	user, err := model.GetUserById(userID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, http.StatusNotFound, common.NewMessage("Invalid parameters")
		}
		return nil, http.StatusServiceUnavailable, unavailable
	}
	var token *model.Token
	if apiKeyID != 0 {
		var err error
		token, err = model.GetTokenByIds(apiKeyID, userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, http.StatusNotFound, common.NewMessage("Invalid parameters")
			}
			return nil, http.StatusServiceUnavailable, unavailable
		}
	}
	policy, version, err := model.LoadArcaEntitlementPolicy(ctx)
	if err != nil {
		return nil, http.StatusServiceUnavailable, unavailable
	}
	now := time.Now()
	saved, err := arcaEntitlementPreview(ctx, policy, userID, user.Group, token, now)
	if err != nil {
		return nil, http.StatusServiceUnavailable, unavailable
	}
	var draft *arcaEntitlementSnapshot
	if proposed != nil {
		effective := proposed.EffectiveForGroup(userID, user.Group)
		if _, err := proposed.Windows(effective, "", now); err != nil {
			return nil, http.StatusBadRequest, common.NewMessage("Invalid input {{error}}", map[string]any{"error": err.Error()})
		}
		snapshot, err := arcaEntitlementPreview(ctx, *proposed, userID, user.Group, token, now)
		if err != nil {
			return nil, http.StatusServiceUnavailable, unavailable
		}
		draft = &snapshot
	}
	return gin.H{"version": version, "saved": saved, "proposed": draft}, http.StatusOK, nil
}

func arcaEntitlementPreview(ctx context.Context, policy model.ArcaEntitlementPolicy, userID int, userGroup string, token *model.Token, now time.Time) (arcaEntitlementSnapshot, error) {
	effective := policy.EffectiveForGroup(userID, userGroup)
	result := arcaEntitlementSnapshot{
		UserID: userID, Enabled: model.ArcaEntitlementRuntimeEnabled(), CostBasis: "gateway_priced_usd", Timezone: policy.Timezone,
		ArcaEffectiveEntitlement: effective, ModelFamilies: policy.ModelFamilies,
	}
	if token != nil && token.ModelLimitsEnabled {
		result.KeyLimitsEnabled = true
		result.KeyModels = token.GetModelLimits()
	}
	windows, err := policy.Windows(effective, "", now)
	if err != nil {
		return result, err
	}
	result.Windows, err = model.ReadArcaBudgetUsage(ctx, userID, windows, now)
	return result, err
}
