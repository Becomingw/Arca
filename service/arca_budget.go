package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/gin-gonic/gin"
)

const arcaRelayEntitlementKey = "arca_relay_entitlements"

type arcaRelayEntitlement struct {
	policy    model.ArcaEntitlementPolicy
	effective model.ArcaEffectiveEntitlement
}

type arcaRequestBudget struct {
	tracked     bool
	userID      int
	requestID   string
	alias       string
	entitlement arcaRelayEntitlement
}

func loadArcaRelayEntitlement(c *gin.Context, userID int) (arcaRelayEntitlement, error) {
	if c != nil {
		if cached, ok := c.Get(arcaRelayEntitlementKey); ok {
			if entitlement, ok := cached.(arcaRelayEntitlement); ok {
				return entitlement, nil
			}
		}
	}
	var ctx context.Context = context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	policy, _, err := model.LoadArcaEntitlementPolicy(ctx)
	if err != nil {
		return arcaRelayEntitlement{}, err
	}
	entitlement := arcaRelayEntitlement{policy: policy, effective: policy.Effective(userID)}
	if c != nil {
		c.Set(arcaRelayEntitlementKey, entitlement)
	}
	return entitlement, nil
}

func CheckArcaRelayEntitlement(c *gin.Context, alias string) *types.NewAPIError {
	userID := c.GetInt("id")
	if userID <= 0 || !model.ArcaEntitlementRuntimeEnabled() {
		return nil
	}
	entitlement, err := loadArcaRelayEntitlement(c, userID)
	if err != nil {
		return arcaBudgetAPIError(err)
	}
	if !entitlement.effective.ModelAccess.Allows(alias) {
		return types.NewErrorWithStatusCode(errors.New("model denied by organization policy"), types.ErrorCodeAccessDenied,
			http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	path := c.Request.URL.Path
	// Realtime and legacy Midjourney can bill without a per-request
	// BillingSession. A finite user allowance cannot admit those paths.
	if path == "/v1/realtime" || strings.Contains(path, "/mj/") {
		windows, err := entitlement.policy.Windows(entitlement.effective, "", time.Now())
		if err != nil {
			return arcaBudgetAPIError(err)
		}
		if len(windows) != 0 || entitlement.effective.Allocation.InternalFunding {
			return types.NewErrorWithStatusCode(errors.New("request format is unavailable for a bounded organization budget"),
				types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
	}
	return nil
}

func arcaBudgetAlias(entitlement arcaRelayEntitlement, alias string) string {
	// An explicit model or family alias can distinguish a literal suffix.
	if _, ok := entitlement.effective.Allocation.Models[alias]; ok {
		return alias
	}
	for _, family := range entitlement.policy.ModelFamilies {
		for _, member := range family.Models {
			if member == alias {
				return alias
			}
		}
	}
	return reasoning.BaseModelName(alias)
}

func reserveArcaRequestBudget(c *gin.Context, info *relaycommon.RelayInfo, quota int) (*arcaRequestBudget, *types.NewAPIError) {
	if info == nil || info.UserId <= 0 || !model.ArcaEntitlementRuntimeEnabled() {
		return nil, nil
	}
	entitlement, err := loadArcaRelayEntitlement(c, info.UserId)
	if err != nil {
		return nil, arcaBudgetAPIError(err)
	}
	if !entitlement.effective.ModelAccess.Allows(info.OriginModelName) {
		return nil, types.NewErrorWithStatusCode(errors.New("model denied by organization policy"), types.ErrorCodeAccessDenied,
			http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	budget := &arcaRequestBudget{
		userID: info.UserId, requestID: info.RequestId,
		alias: arcaBudgetAlias(entitlement, info.OriginModelName), entitlement: entitlement,
	}
	now := time.Now()
	windows, err := entitlement.policy.Windows(entitlement.effective, budget.alias, now)
	if err != nil {
		return nil, arcaBudgetAPIError(err)
	}
	if len(windows) == 0 {
		// Keep facts for unrelated aliases while the allocation has a
		// budget. A family membership edit includes earlier usage in its
		// current window, even when the alias was previously outside it.
		allWindows, err := entitlement.policy.Windows(entitlement.effective, "", now)
		if err != nil {
			return nil, arcaBudgetAPIError(err)
		}
		if len(allWindows) == 0 {
			if entitlement.effective.Allocation.InternalFunding {
				return budget, nil
			}
			return nil, nil
		}
	}
	budget.tracked = true
	var ctx context.Context = context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	reserve := model.ReserveArcaBudget
	if info.ArcaFreeBudget {
		reserve = model.ResizeArcaBudget
	}
	if err := reserve(ctx, budget.userID, budget.requestID, budget.alias, quota, windows, now); err != nil {
		return nil, arcaBudgetAPIError(err)
	}
	return budget, nil
}

func (b *arcaRequestBudget) reserve(quota int) error {
	if b == nil || !b.tracked {
		return nil
	}
	now := time.Now()
	windows, err := b.entitlement.policy.Windows(b.entitlement.effective, b.alias, now)
	if err != nil {
		return arcaBudgetAPIError(err)
	}
	if err := model.ResizeArcaBudget(context.Background(), b.userID, b.requestID, b.alias, quota, windows, now); err != nil {
		return arcaBudgetAPIError(err)
	}
	return nil
}

func (b *arcaRequestBudget) finish(actualQuota *int) error {
	if b == nil || !b.tracked {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return model.FinishArcaBudget(ctx, b.userID, b.requestID, actualQuota)
}

// PrepareArcaFreeBilling records price and attribution without choosing
// or touching a wallet or subscription for a zero-price request.
func PrepareArcaFreeBilling(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	budget, err := reserveArcaRequestBudget(c, info, 0)
	if err != nil {
		return err
	}
	if budget != nil {
		info.ArcaFreeBudget = budget.tracked
		if budget.entitlement.effective.Allocation.InternalFunding {
			info.BillingSource = BillingSourceOrganization
		}
	}
	return nil
}

func HasArcaBudgetReservation(info *relaycommon.RelayInfo) bool {
	session, ok := info.Billing.(*BillingSession)
	return info.ArcaFreeBudget || ok && session.arcaBudget != nil && session.arcaBudget.tracked
}

func ReserveArcaSelectedGroup(c *gin.Context, info *relaycommon.RelayInfo, quota int) *types.NewAPIError {
	if quota <= 0 {
		return nil
	}
	if info.ArcaFreeBudget {
		// A previously free request now has a positive price. Select a
		// normal paid funding source using the original billing preference.
		info.PriceData.FreeModel = false
		return PreConsumeBilling(c, quota, info)
	}
	if session, ok := info.Billing.(*BillingSession); ok {
		if err := session.arcaBudget.reserve(quota); err != nil {
			var apiErr *types.NewAPIError
			if errors.As(err, &apiErr) {
				return apiErr
			}
			return arcaBudgetAPIError(err)
		}
	}
	return nil
}

func ReleaseArcaFreeBudget(info *relaycommon.RelayInfo) {
	if err := finishArcaFreeBudget(info, nil); err != nil {
		common.SysError(common.LogText("failed to release an organization budget reservation"))
	}
}

func finishArcaFreeBudget(info *relaycommon.RelayInfo, actualQuota *int) error {
	if info == nil || !info.ArcaFreeBudget {
		return nil
	}
	budget := &arcaRequestBudget{userID: info.UserId, requestID: info.RequestId, tracked: true}
	err := budget.finish(actualQuota)
	if err == nil {
		info.ArcaFreeBudget = false
	}
	return err
}

func finishArcaTaskBudget(task *model.Task, actualQuota *int) {
	if !model.ArcaEntitlementRuntimeEnabled() || task.PrivateData.Execution == nil || task.PrivateData.Execution.RequestID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := model.FinishArcaTaskBudget(ctx, task.UserId, task.PrivateData.Execution.RequestID, actualQuota); err != nil {
		common.SysError(common.LogText("failed to settle an organization task budget"))
	}
}

func arcaBudgetAPIError(err error) *types.NewAPIError {
	switch {
	case errors.Is(err, model.ErrArcaBudgetExceeded):
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
			types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	case errors.Is(err, model.ErrArcaEntitlementConflict):
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusConflict, types.ErrOptionWithSkipRetry())
	default:
		return types.NewErrorWithStatusCode(errors.New("organization policy unavailable"), types.ErrorCodeQueryDataError,
			http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
}
