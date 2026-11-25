package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
	BillingSourceOrganization = "arca_organization"
)

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
// 会话已存在时（任务重试换到另一个渠道）把预扣补到 preConsumedQuota，
// 和首次预扣一样必须由余额覆盖。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo != nil && relayInfo.QuotaClamp != nil {
		return types.NewErrorWithStatusCode(
			relayInfo.QuotaClamp,
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("pre-consume quota cannot be negative: %d", preConsumedQuota),
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if relayInfo != nil && relayInfo.Billing != nil {
		if err := relayInfo.Billing.ReserveBeforeSubmit(preConsumedQuota); err != nil {
			var apiErr *types.NewAPIError
			if errors.As(err, &apiErr) {
				return apiErr
			}
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		}
		return nil
	}
	budget, budgetErr := reserveArcaRequestBudget(c, relayInfo, preConsumedQuota)
	if budgetErr != nil {
		return budgetErr
	}
	var session *BillingSession
	var apiErr *types.NewAPIError
	if budget != nil && budget.entitlement.effective.Allocation.InternalFunding {
		session = &BillingSession{relayInfo: relayInfo, funding: organizationFunding{}}
		apiErr = session.preConsume(c, preConsumedQuota)
	} else {
		session, apiErr = NewBillingSession(c, relayInfo, preConsumedQuota)
	}
	if apiErr != nil {
		if err := budget.finish(nil); err != nil {
			common.SysError(common.LogText("failed to release an organization budget reservation"))
		}
		return apiErr
	}
	session.arcaBudget = budget
	relayInfo.Billing = session
	relayInfo.ArcaFreeBudget = false
	return nil
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if HasArcaBudgetReservation(relayInfo) {
		model.MarkArcaBudgetChargeFact(ctx, relayInfo.UserId, relayInfo.RequestId, actualQuota)
	}
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		delta := actualQuota - preConsumed

		if delta > 0 {
			logger.LogInfo(ctx, common.LogText("charging the difference after pre-consume: %s (actual %s, pre-consumed %s)",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			logger.LogInfo(ctx, common.LogText("refunding the difference after pre-consume: %s (actual %s, pre-consumed %s)",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, common.LogText("pre-consumed quota matches the actual quota, no adjustment: %s (per-request billing)",
				logger.FormatQuota(actualQuota),
			))
		}

		if err := relayInfo.Billing.Settle(actualQuota); err != nil {
			return err
		}

		// 发送额度通知（订阅计费使用订阅剩余额度）
		if actualQuota != 0 {
			switch relayInfo.BillingSource {
			case BillingSourceSubscription:
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			case BillingSourceWallet:
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}
		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	budgetErr := finishArcaFreeBudget(relayInfo, &actualQuota)
	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		return errors.Join(budgetErr, PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true))
	}
	return budgetErr
}
