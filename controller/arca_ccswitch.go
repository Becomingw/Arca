package controller

import (
	"net/http"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// arcaCCSwitchUsage is a key-owner view of the active Arca allowance. It keeps
// overlapping plan windows and temporary grants as independent observations.
type arcaCCSwitchUsage struct {
	SubscriptionName string                        `json:"subscription_name,omitempty"`
	State            string                        `json:"state"`
	Timezone         string                        `json:"timezone"`
	Windows          []model.ArcaBudgetUsage       `json:"windows"`
	Grants           []model.ArcaUsageGrantBalance `json:"grants"`
}

func GetArcaCCSwitchUsage(c *gin.Context) {
	now, ctx := time.Now(), c.Request.Context()
	userID := c.GetInt("id")
	unavailable := common.NewMessage("Subscription usage is temporarily unavailable")
	if !model.ArcaEntitlementRuntimeEnabled() {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, unavailable)
		return
	}
	policy, _, err := model.LoadArcaEntitlementPolicy(ctx)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, unavailable)
		return
	}
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	effective := policy.EffectiveForGroup(userID, userGroup)
	groups, err := getModelListGroups(c)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, unavailable)
		return
	}
	configured, err := model.ReadGroupsEnabledModels(ctx, groups.ownerGroups)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, unavailable)
		return
	}
	aliases, err := filterRelayModelNames(c, configured)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, unavailable)
		return
	}
	aliases = slices.DeleteFunc(aliases, func(alias string) bool { return !effective.ModelAccess.Allows(alias) })
	if selected := c.Query("model"); selected != "" {
		if !slices.Contains(aliases, selected) {
			common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("This API key cannot use the selected model"))
			return
		}
		aliases = []string{selected}
	}
	slices.Sort(aliases)
	allWindows, err := policy.Windows(effective, "", now)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, unavailable)
		return
	}
	data := arcaCCSwitchUsage{SubscriptionName: effective.SubscriptionName, State: "unassigned", Timezone: policy.Timezone,
		Windows: []model.ArcaBudgetUsage{}, Grants: []model.ArcaUsageGrantBalance{}}
	switch {
	case effective.ModelAccess.Mode == "deny_all":
		data.State = "paused"
		common.ApiSuccess(c, data)
		return
	case effective.Allocation.Unlimited || (effective.SubscriptionID == "" && len(allWindows) == 0 && effective.Allocation.InternalFunding):
		data.State = "unlimited"
	case effective.SubscriptionID != "" || len(allWindows) > 0:
		data.State = "active"
	}

	budgetAliases := make(map[string][]string, len(aliases))
	for _, alias := range aliases {
		ledgerAlias := service.ArcaBudgetAlias(policy, effective, alias)
		budgetAliases[ledgerAlias] = append(budgetAliases[ledgerAlias], alias)
	}
	eligibleWindows := make([]model.ArcaBudgetWindow, 0, len(allWindows))
	visibleAliases := make([][]string, 0, len(allWindows))
	for _, window := range allWindows {
		var visible []string
		for _, name := range window.Models {
			visible = append(visible, budgetAliases[name]...)
		}
		if len(window.Models) == 0 || len(visible) > 0 {
			eligibleWindows = append(eligibleWindows, window)
			visibleAliases = append(visibleAliases, visible)
		}
	}
	data.Windows, err = model.ReadArcaBudgetUsage(ctx, userID, eligibleWindows, now)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, unavailable)
		return
	}
	// Family spend uses the policy's full family. Filter aliases only after
	// the ledger read, so requests made with other keys still consume the cap.
	for i := range data.Windows {
		slices.Sort(visibleAliases[i])
		data.Windows[i].Models = slices.Compact(visibleAliases[i])
	}
	grants, err := model.ListArcaUsageGrants(ctx, userID, now)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, unavailable)
		return
	}
	for _, grant := range grants {
		if len(grant.Models) > 0 {
			grant.Models = slices.DeleteFunc(grant.Models, func(alias string) bool {
				_, found := slices.BinarySearch(aliases, alias)
				return !found
			})
			if len(grant.Models) == 0 {
				continue
			}
		} else if len(aliases) == 0 {
			continue
		}
		data.Grants = append(data.Grants, grant)
	}
	common.ApiSuccess(c, data)
}
