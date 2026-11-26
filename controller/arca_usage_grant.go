package controller

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func ListMyArcaUsageGrants(c *gin.Context) {
	grants, err := model.ListArcaUsageGrants(c.Request.Context(), c.GetInt("id"), time.Now())
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Could not load usage grants"))
		return
	}
	common.ApiSuccess(c, grants)
}

func RedeemArcaUsageCode(c *gin.Context) {
	var request struct {
		Code string `json:"code"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if err := common.DecodeJsonBodyStrict(c.Request.Body, &request); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	grant, err := model.RedeemArcaUsageGrant(c.Request.Context(), c.GetInt("id"), strings.TrimSpace(request.Code), time.Now())
	switch {
	case errors.Is(err, model.ErrArcaGrantUnavailable):
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Redemption code is invalid, unavailable or expired"))
	case err != nil:
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Could not redeem usage grant"))
	default:
		common.ApiSuccess(c, grant)
	}
}

func GetArcaUsageGrantScopes(c *gin.Context) {
	policy, _, err := model.LoadArcaEntitlementPolicy(c.Request.Context())
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Could not load usage grant scopes"))
		return
	}
	models := model.GetEnabledModels()
	slices.Sort(models)
	families := make(map[string]string, len(policy.ModelFamilies))
	for id, family := range policy.ModelFamilies {
		if len(family.Models) > 0 {
			families[id] = family.Name
		}
	}
	common.ApiSuccess(c, gin.H{"models": models, "families": families})
}
