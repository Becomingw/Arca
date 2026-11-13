use std::collections::BTreeMap;
use std::sync::Arc;
use std::time::Duration;

use aether_data_contracts::repository::billing::{
    nonnegative_usd_to_usage_policy_cost_units, parse_usage_policy_entitlements, UsagePolicyMetric,
    UsagePolicyWindow, UserPlanEntitlementRecord, USAGE_POLICY_COST_UNITS_PER_USD,
};
use aether_data_contracts::repository::settlement::{
    ReconcileUsagePolicyCostInput, ReserveUsagePolicyCostInput, ReserveUsagePolicyCostOutcome,
    ReserveUsagePolicyRequestInput, ReserveUsagePolicyRequestOutcome,
    UsagePolicyCostReservationState, UsagePolicyCostWindow, UsagePolicyRequestWindow,
};
use aether_runtime::AdmissionPermit;
use aether_runtime_state::{
    RuntimeSemaphoreConfig, RuntimeSemaphoreError, UsageLimitCheck, UsageLimitInput,
    UsageLimitReleaseInput, UsageLimitRule,
};
use chrono::{Datelike, TimeZone, Utc, Weekday};
use tracing::warn;

use crate::control::GatewayControlDecision;
use crate::{AppState, GatewayError};

const POLICY_CACHE_TTL: Duration = Duration::from_secs(5);
const CONCURRENCY_GATE: &str = "plan_usage_concurrency";
const DEFAULT_CALENDAR_TIMEZONE: &str = "Asia/Shanghai";
const COST_RESERVATION_TTL_SECS: u64 = 24 * 60 * 60;
const COST_RESERVATION_SAFE_HISTORY_SECS: u64 = 32 * 24 * 60 * 60;

#[derive(Debug, Clone)]
pub(crate) struct PlanUsagePolicySnapshot {
    pub(crate) admitted_at_unix_secs: u64,
    subject_id: Arc<str>,
    policy: Arc<EffectivePlanUsagePolicy>,
}

impl PlanUsagePolicySnapshot {
    fn for_admission(
        subject_id: &str,
        policy: EffectivePlanUsagePolicy,
        admitted_at_unix_secs: u64,
    ) -> Option<Self> {
        if policy.cost_rules.is_empty() {
            return None;
        }
        Some(Self {
            admitted_at_unix_secs,
            subject_id: subject_id.to_string().into(),
            policy: Arc::new(policy),
        })
    }

    pub(crate) fn new_reservation_context(&self) -> PlanUsageReservationContext {
        PlanUsageReservationContext {
            policy_snapshot: self.clone(),
            token: uuid::Uuid::new_v4().to_string().into(),
        }
    }

    pub(crate) fn subject_id(&self) -> &str {
        self.subject_id.as_ref()
    }

    pub(crate) fn policy(&self) -> &EffectivePlanUsagePolicy {
        self.policy.as_ref()
    }
}

#[derive(Debug, Clone)]
pub(crate) struct PlanUsageReservationContext {
    policy_snapshot: PlanUsagePolicySnapshot,
    token: Arc<str>,
}

impl PlanUsageReservationContext {
    #[cfg(test)]
    pub(crate) fn for_test(
        subject_id: impl Into<Arc<str>>,
        token: impl Into<Arc<str>>,
        admitted_at_unix_secs: u64,
        policy: EffectivePlanUsagePolicy,
    ) -> Self {
        Self {
            policy_snapshot: PlanUsagePolicySnapshot {
                admitted_at_unix_secs,
                subject_id: subject_id.into(),
                policy: Arc::new(policy),
            },
            token: token.into(),
        }
    }

    pub(crate) fn subject_id(&self) -> &str {
        self.policy_snapshot.subject_id()
    }

    pub(crate) fn token(&self) -> &str {
        self.token.as_ref()
    }

    pub(crate) fn policy(&self) -> &EffectivePlanUsagePolicy {
        self.policy_snapshot.policy()
    }

    pub(crate) const fn admitted_at_unix_secs(&self) -> u64 {
        self.policy_snapshot.admitted_at_unix_secs
    }
}

#[derive(Debug)]
pub(crate) struct HttpPlanUsageAdmission {
    pub(crate) permit: Option<AdmissionPermit>,
    pub(crate) reservation_context: Option<PlanUsageReservationContext>,
}

#[derive(Debug)]
pub(crate) struct PlanUsageAdmission {
    pub(crate) permit: Option<AdmissionPermit>,
    pub(crate) policy_snapshot: Option<PlanUsagePolicySnapshot>,
}

#[derive(Debug, Clone, PartialEq, Default)]
pub(crate) struct EffectivePlanUsagePolicy {
    model: Option<String>,
    request_rules: Vec<EffectiveRequestRule>,
    cost_rules: Vec<EffectiveCostRule>,
    concurrency_limit: Option<u64>,
    valid_until_unix_secs: Option<u64>,
}

#[derive(Debug, Clone, PartialEq)]
struct EffectiveRequestRule {
    model: Option<String>,
    identity: String,
    window: UsagePolicyWindow,
    limit: u64,
    entitlement_period: Option<(u64, u64)>,
}

#[derive(Debug, Clone, PartialEq)]
struct EffectiveCostRule {
    model: Option<String>,
    models: Option<Vec<String>>,
    identity: String,
    window: UsagePolicyWindow,
    limit_cost_units: u64,
    entitlement_period: Option<(u64, u64)>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct RuntimeRequestRule {
    key: String,
    limit: u64,
    window_seconds: u64,
    retention_seconds: u64,
    retry_after: u64,
    window: &'static str,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct DurableRequestRule {
    window: UsagePolicyRequestWindow,
    retry_after: u64,
    label: &'static str,
    influence_ends_at_unix_secs: u64,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct RuntimeCostRule {
    window: UsagePolicyCostWindow,
    retry_after: u64,
    label: &'static str,
    influence_ends_at_unix_secs: u64,
}

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct PlanUsagePolicyRejection {
    pub(crate) used: Option<f64>,
    pub(crate) estimated: Option<f64>,
    pub(crate) metric: &'static str,
    pub(crate) limit: f64,
    pub(crate) retry_after: u64,
    pub(crate) window: &'static str,
}

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum PlanUsageCostReservationOutcome {
    NotRequired,
    Reserved,
    Rejected(PlanUsagePolicyRejection),
}

pub(crate) async fn reserve_admitted_http_plan_usage_policy_cost(
    state: &AppState,
    decision: &GatewayControlDecision,
    plan: &aether_contracts::ExecutionPlan,
    report_context: Option<&serde_json::Value>,
    reservation: Option<&PlanUsageReservationContext>,
) -> Result<PlanUsageCostReservationOutcome, GatewayError> {
    let Some(auth) = plan_usage_auth_context(decision) else {
        return Ok(PlanUsageCostReservationOutcome::NotRequired);
    };
    let Some(reservation) = reservation else {
        return Ok(PlanUsageCostReservationOutcome::NotRequired);
    };
    if reservation.subject_id() != auth.user_id {
        return Err(GatewayError::Internal(
            "plan usage reservation subject does not match the admitted request".to_string(),
        ));
    }
    reserve_plan_usage_policy_cost_with_policy(
        state,
        decision,
        plan,
        report_context,
        reservation.policy(),
        reservation.admitted_at_unix_secs(),
        reservation.token(),
    )
    .await
}

pub(crate) async fn reserve_admitted_plan_usage_policy_cost(
    state: &AppState,
    decision: &GatewayControlDecision,
    plan: &aether_contracts::ExecutionPlan,
    report_context: Option<&serde_json::Value>,
    snapshot: Option<&PlanUsagePolicySnapshot>,
    reservation_token: &str,
) -> Result<PlanUsageCostReservationOutcome, GatewayError> {
    let Some(auth) = plan_usage_auth_context(decision) else {
        return Ok(PlanUsageCostReservationOutcome::NotRequired);
    };
    let Some(snapshot) = snapshot else {
        return Ok(PlanUsageCostReservationOutcome::NotRequired);
    };
    if snapshot.subject_id() != auth.user_id {
        return Err(GatewayError::Internal(
            "plan usage reservation subject does not match the admitted request".to_string(),
        ));
    }
    reserve_plan_usage_policy_cost_with_policy(
        state,
        decision,
        plan,
        report_context,
        snapshot.policy(),
        snapshot.admitted_at_unix_secs,
        reservation_token,
    )
    .await
}

fn plan_usage_auth_context(
    decision: &GatewayControlDecision,
) -> Option<&crate::control::GatewayControlAuthContext> {
    decision.auth_context.as_ref().filter(|auth| {
        decision.route_class.as_deref() == Some("ai_public")
            && (crate::internal_access::enabled()
                || (!auth.admin_bypass_limits && !auth.api_key_is_standalone))
    })
}

#[allow(clippy::too_many_arguments)]
async fn reserve_plan_usage_policy_cost_with_policy(
    state: &AppState,
    decision: &GatewayControlDecision,
    plan: &aether_contracts::ExecutionPlan,
    report_context: Option<&serde_json::Value>,
    policy: &EffectivePlanUsagePolicy,
    admitted_at_unix_secs: u64,
    reservation_token: &str,
) -> Result<PlanUsageCostReservationOutcome, GatewayError> {
    let Some(auth) = plan_usage_auth_context(decision) else {
        return Ok(PlanUsageCostReservationOutcome::NotRequired);
    };
    if policy.cost_rules.is_empty() {
        return Ok(PlanUsageCostReservationOutcome::NotRequired);
    }
    ensure_cost_policy_usage_runtime_enabled(state.usage_runtime.is_enabled())?;
    let estimated_cost_usd =
        crate::control::estimate_execution_plan_cost_upper_bound_usd(state, plan, report_context)
            .await?
            .ok_or_else(|| GatewayError::Client {
                // Defensive fallback only: known media, stored files and hosted tools are all
                // estimable. Remaining cases (server-side conversation state, unknown request
                // shapes) are a client error, never a hidden 500.
                status: http::StatusCode::UNPROCESSABLE_ENTITY,
                message: "a hard plan cost limit is active, but this request's cost cannot be \
                    estimated in advance (e.g. it references server-side conversation state such \
                    as previous_response_id); resend the full input or use a key without a hard \
                    cost limit"
                    .to_string(),
            })?;
    let reserved_cost_units = nonnegative_usd_to_usage_policy_cost_units(estimated_cost_usd)
        .ok_or_else(|| {
            GatewayError::Internal(
                "estimated request cost is outside the supported plan limit range".to_string(),
            )
        })?;
    let runtime_rules = policy
        .cost_rules
        .iter()
        .map(|rule| runtime_cost_rule(rule, admitted_at_unix_secs))
        .collect::<Result<Vec<_>, _>>()?;
    let reservation_expires_at_unix_secs =
        admitted_at_unix_secs.saturating_add(COST_RESERVATION_TTL_SECS);
    let retain_until_unix_secs = runtime_rules
        .iter()
        .map(|rule| rule.influence_ends_at_unix_secs)
        .chain(std::iter::once(reservation_expires_at_unix_secs))
        .chain(std::iter::once(
            admitted_at_unix_secs.saturating_add(COST_RESERVATION_SAFE_HISTORY_SECS),
        ))
        .max()
        .unwrap_or(reservation_expires_at_unix_secs);
    let outcome = state
        .data
        .reserve_usage_policy_cost(ReserveUsagePolicyCostInput {
            model: policy.model.clone(),
            request_id: plan.request_id.clone(),
            reservation_token: reservation_token.to_string(),
            subject_id: auth.user_id.clone(),
            admitted_at_unix_secs,
            reserved_cost_units,
            reservation_expires_at_unix_secs,
            retain_until_unix_secs,
            windows: runtime_rules
                .iter()
                .map(|rule| rule.window.clone())
                .collect(),
        })
        .await
        .map_err(|error| GatewayError::Internal(error.to_string()))?
        .ok_or_else(|| {
            GatewayError::Internal(
                "plan cost limits require a settlement write repository".to_string(),
            )
        })?;

    match outcome {
        ReserveUsagePolicyCostOutcome::Allowed { .. } => {
            Ok(PlanUsageCostReservationOutcome::Reserved)
        }
        ReserveUsagePolicyCostOutcome::Rejected {
            window_index,
            limit_cost_units,
            used_cost_units,
            retry_after_secs,
        } => {
            let rule = runtime_rules.get(window_index).ok_or_else(|| {
                GatewayError::Internal(
                    "usage policy cost reservation returned an invalid window index".to_string(),
                )
            })?;
            warn!(
                event_name = "plan_usage_cost_rejected",
                used_cost_units,
                reserved_cost_units,
                limit_cost_units,
                window = rule.label,
                "request cost reservation exceeds the plan window"
            );
            Ok(PlanUsageCostReservationOutcome::Rejected(
                PlanUsagePolicyRejection {
                    used: Some(used_cost_units as f64 / USAGE_POLICY_COST_UNITS_PER_USD as f64),
                    estimated: Some(estimated_cost_usd),
                    metric: "actual_cost_usd",
                    limit: limit_cost_units as f64 / USAGE_POLICY_COST_UNITS_PER_USD as f64,
                    retry_after: retry_after_secs.unwrap_or(rule.retry_after),
                    window: rule.label,
                },
            ))
        }
        ReserveUsagePolicyCostOutcome::AlreadyTerminal { state } => {
            Err(GatewayError::Internal(format!(
                "plan cost reservation for request {} is already {}",
                plan.request_id,
                state.as_str()
            )))
        }
        ReserveUsagePolicyCostOutcome::Conflict => Err(GatewayError::Internal(
            "plan cost reservation request identity conflict".to_string(),
        )),
    }
}

pub(crate) async fn release_plan_usage_policy_cost(
    state: &AppState,
    decision: &GatewayControlDecision,
    plan: &aether_contracts::ExecutionPlan,
    reservation_token: &str,
    finalized_at_unix_secs: u64,
) -> Result<(), GatewayError> {
    let Some(auth) = decision
        .auth_context
        .as_ref()
        .filter(|_| decision.route_class.as_deref() == Some("ai_public"))
    else {
        return Ok(());
    };
    if !crate::internal_access::enabled()
        && (auth.admin_bypass_limits || auth.api_key_is_standalone)
    {
        return Ok(());
    }

    state
        .data
        .reconcile_usage_policy_cost(build_usage_policy_cost_release_input(
            plan.request_id.as_str(),
            auth.user_id.as_str(),
            reservation_token,
            finalized_at_unix_secs,
        ))
        .await
        .map_err(|error| GatewayError::Internal(error.to_string()))?;
    Ok(())
}

fn build_usage_policy_cost_release_input(
    request_id: &str,
    subject_id: &str,
    reservation_token: &str,
    finalized_at_unix_secs: u64,
) -> ReconcileUsagePolicyCostInput {
    ReconcileUsagePolicyCostInput {
        request_id: request_id.to_string(),
        subject_id: subject_id.to_string(),
        reservation_token: reservation_token.to_string(),
        actual_cost_units: 0,
        terminal_state: UsagePolicyCostReservationState::Released,
        finalized_at_unix_secs,
    }
}

fn ensure_cost_policy_usage_runtime_enabled(enabled: bool) -> Result<(), GatewayError> {
    if enabled {
        Ok(())
    } else {
        Err(GatewayError::Internal(
            "plan cost limits require the usage runtime to be enabled for terminal reconciliation"
                .to_string(),
        ))
    }
}

#[derive(Debug)]
pub(crate) enum PlanUsageAdmissionError {
    Rejected(PlanUsagePolicyRejection),
    Runtime(RuntimeSemaphoreError),
    Gateway(GatewayError),
}

impl From<GatewayError> for PlanUsageAdmissionError {
    fn from(value: GatewayError) -> Self {
        Self::Gateway(value)
    }
}

pub(crate) async fn check_and_acquire_plan_usage_policy_admission(
    state: &AppState,
    decision: Option<&GatewayControlDecision>,
    event_id: &str,
    now_unix_ms: u64,
    model: Option<&str>,
) -> Result<PlanUsageAdmission, PlanUsageAdmissionError> {
    let Some(auth) = decision
        .filter(|decision| decision.route_class.as_deref() == Some("ai_public"))
        .and_then(|decision| decision.auth_context.as_ref())
    else {
        return Ok(PlanUsageAdmission {
            permit: None,
            policy_snapshot: None,
        });
    };
    if !crate::internal_access::enabled()
        && (auth.admin_bypass_limits || auth.api_key_is_standalone)
    {
        return Ok(PlanUsageAdmission {
            permit: None,
            policy_snapshot: None,
        });
    }

    let admitted_at_unix_secs = now_unix_ms / 1_000;
    let policy = load_effective_policy(state, &auth.user_id, admitted_at_unix_secs, model).await?;
    let permit = check_and_acquire_compiled_plan_usage_policy(
        state,
        &auth.user_id,
        &policy,
        event_id,
        now_unix_ms,
        model,
    )
    .await?;
    let policy_snapshot =
        PlanUsagePolicySnapshot::for_admission(&auth.user_id, policy, admitted_at_unix_secs);
    Ok(PlanUsageAdmission {
        permit,
        policy_snapshot,
    })
}

async fn check_and_acquire_compiled_plan_usage_policy(
    state: &AppState,
    subject_id: &str,
    policy: &EffectivePlanUsagePolicy,
    event_id: &str,
    now_unix_ms: u64,
    model: Option<&str>,
) -> Result<Option<AdmissionPermit>, PlanUsageAdmissionError> {
    if policy.request_rules.is_empty() && policy.concurrency_limit.is_none() {
        return Ok(None);
    }

    let now_unix_secs = now_unix_ms / 1_000;

    let plan_permit = if let Some(limit) = policy.concurrency_limit {
        let limit = usize::try_from(limit).map_err(|_| {
            PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                "plan usage concurrency limit exceeds platform capacity".to_string(),
            ))
        })?;
        let gate = state
            .runtime_state
            .keyed_semaphore(
                CONCURRENCY_GATE,
                format!("admission:{CONCURRENCY_GATE}:user:{{{subject_id}}}"),
                limit,
                RuntimeSemaphoreConfig::default(),
            )
            .map_err(PlanUsageAdmissionError::Runtime)?;
        Some(
            gate.try_acquire()
                .await
                .map_err(PlanUsageAdmissionError::Runtime)?,
        )
    } else {
        None
    };

    let (runtime_request_rules, durable_request_rules): (Vec<_>, Vec<_>) = policy
        .request_rules
        .iter()
        .partition(|rule| request_rule_uses_runtime_state(rule));
    let runtime_rules = runtime_request_rules
        .iter()
        .map(|rule| runtime_request_rule(subject_id, rule, now_unix_secs))
        .collect::<Result<Vec<_>, _>>()?;
    let runtime_inputs = runtime_rules
        .iter()
        .map(|rule| UsageLimitRule {
            key: rule.key.as_str(),
            limit: rule.limit,
            window_seconds: rule.window_seconds,
            retention_seconds: rule.retention_seconds,
        })
        .collect::<Vec<_>>();

    // Consume the short-lived runtime windows first. If the durable admission below rejects or
    // errors, remove this event from every short window. This ordering bounds a process-crash
    // compensation gap to the QPS/RPM retention (at most 60 seconds) instead of leaking a
    // calendar/subscription-period admission for weeks or months.
    if !runtime_inputs.is_empty() {
        match state
            .runtime_state
            .check_and_consume_usage_limits(UsageLimitInput {
                rules: &runtime_inputs,
                event_id,
                now_unix_ms,
            })
            .await
            .map_err(|error| {
                PlanUsageAdmissionError::Gateway(GatewayError::Internal(error.to_string()))
            })? {
            UsageLimitCheck::Allowed => {}
            UsageLimitCheck::Rejected {
                rule_index,
                limit,
                retry_after,
            } => {
                let rule = runtime_rules.get(rule_index).ok_or_else(|| {
                    PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                        "usage policy runtime returned an invalid rule index".to_string(),
                    ))
                })?;
                return Err(PlanUsageAdmissionError::Rejected(
                    PlanUsagePolicyRejection {
                        used: None,
                        estimated: None,
                        metric: "request_count",
                        limit: limit as f64,
                        retry_after: retry_after.min(rule.retry_after).max(1),
                        window: rule.window,
                    },
                ));
            }
        }
    }

    let durable_rules = durable_request_rules
        .iter()
        .map(|rule| durable_request_rule(rule, now_unix_secs))
        .collect::<Result<Vec<_>, _>>()?;
    if !durable_rules.is_empty() {
        let retain_until_unix_secs = durable_rules
            .iter()
            .map(|rule| rule.influence_ends_at_unix_secs)
            .chain(std::iter::once(
                now_unix_secs.saturating_add(COST_RESERVATION_SAFE_HISTORY_SECS),
            ))
            .max()
            .unwrap_or(now_unix_secs.saturating_add(COST_RESERVATION_SAFE_HISTORY_SECS));
        let outcome = state
            .data
            .reserve_usage_policy_request(ReserveUsagePolicyRequestInput {
                model: model.map(str::to_string),
                request_id: event_id.to_string(),
                subject_id: subject_id.to_string(),
                event_token: event_id.to_string(),
                admitted_at_unix_secs: now_unix_secs,
                retain_until_unix_secs,
                windows: durable_rules
                    .iter()
                    .map(|rule| rule.window.clone())
                    .collect(),
            })
            .await;
        let outcome = match outcome {
            Ok(Some(outcome)) => outcome,
            Ok(None) => {
                release_runtime_usage_limits_best_effort(
                    state,
                    &runtime_inputs,
                    event_id,
                    "settlement_writer_unavailable",
                )
                .await;
                return Err(PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                    "long-window plan request limits require a settlement write repository"
                        .to_string(),
                )));
            }
            Err(error) => {
                release_runtime_usage_limits_best_effort(
                    state,
                    &runtime_inputs,
                    event_id,
                    "durable_admission_error",
                )
                .await;
                return Err(PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                    error.to_string(),
                )));
            }
        };
        match outcome {
            ReserveUsagePolicyRequestOutcome::Allowed => {}
            ReserveUsagePolicyRequestOutcome::Rejected {
                window_index,
                limit_requests,
                ..
            } => {
                release_runtime_usage_limits_best_effort(
                    state,
                    &runtime_inputs,
                    event_id,
                    "durable_admission_rejected",
                )
                .await;
                let rule = durable_rules.get(window_index).ok_or_else(|| {
                    PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                        "usage policy request ledger returned an invalid rule index".to_string(),
                    ))
                })?;
                return Err(PlanUsageAdmissionError::Rejected(
                    PlanUsagePolicyRejection {
                        used: None,
                        estimated: None,
                        metric: "request_count",
                        limit: limit_requests as f64,
                        retry_after: rule.retry_after,
                        window: rule.label,
                    },
                ));
            }
            ReserveUsagePolicyRequestOutcome::AlreadyReleased => {
                release_runtime_usage_limits_best_effort(
                    state,
                    &runtime_inputs,
                    event_id,
                    "durable_admission_already_released",
                )
                .await;
                return Err(PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                    "plan request admission token is already released".to_string(),
                )));
            }
            ReserveUsagePolicyRequestOutcome::Conflict => {
                release_runtime_usage_limits_best_effort(
                    state,
                    &runtime_inputs,
                    event_id,
                    "durable_admission_conflict",
                )
                .await;
                return Err(PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                    "plan request admission identity conflict".to_string(),
                )));
            }
        }
    }

    Ok(AdmissionPermit::from_parts(None, plan_permit))
}

pub(crate) async fn check_and_acquire_http_plan_usage_policy(
    state: &AppState,
    decision: Option<&GatewayControlDecision>,
    event_id: &str,
    now_unix_ms: u64,
    model: Option<&str>,
) -> Result<HttpPlanUsageAdmission, PlanUsageAdmissionError> {
    let Some(auth) = decision
        .filter(|decision| decision.route_class.as_deref() == Some("ai_public"))
        .and_then(|decision| decision.auth_context.as_ref())
    else {
        return Ok(HttpPlanUsageAdmission {
            permit: None,
            reservation_context: None,
        });
    };
    if !crate::internal_access::enabled()
        && (auth.admin_bypass_limits || auth.api_key_is_standalone)
    {
        return Ok(HttpPlanUsageAdmission {
            permit: None,
            reservation_context: None,
        });
    }

    let admitted_at_unix_secs = now_unix_ms / 1_000;
    let policy = load_effective_policy(state, &auth.user_id, admitted_at_unix_secs, model).await?;
    let permit = check_and_acquire_compiled_plan_usage_policy(
        state,
        &auth.user_id,
        &policy,
        event_id,
        now_unix_ms,
        model,
    )
    .await?;
    let reservation_context =
        PlanUsagePolicySnapshot::for_admission(&auth.user_id, policy, admitted_at_unix_secs)
            .map(|snapshot| snapshot.new_reservation_context());
    Ok(HttpPlanUsageAdmission {
        permit,
        reservation_context,
    })
}

fn request_rule_uses_runtime_state(rule: &&EffectiveRequestRule) -> bool {
    matches!(rule.window, UsagePolicyWindow::Rolling { seconds } if seconds <= 60)
}

async fn release_runtime_usage_limits_best_effort(
    state: &AppState,
    rules: &[UsageLimitRule<'_>],
    event_id: &str,
    reason: &'static str,
) {
    if rules.is_empty() {
        return;
    }
    if let Err(error) = state
        .runtime_state
        .release_usage_limits(UsageLimitReleaseInput { rules, event_id })
        .await
    {
        warn!(
            event_name = "plan_usage_runtime_compensation_failed",
            log_type = "ops",
            event_id,
            reason,
            error = %error,
            "gateway failed to compensate short-window plan usage after durable admission failed"
        );
    }
}

async fn load_effective_policy(
    state: &AppState,
    user_id: &str,
    now_unix_secs: u64,
    model: Option<&str>,
) -> Result<EffectivePlanUsagePolicy, GatewayError> {
    if crate::internal_access::enabled() {
        let (config, allocation) = crate::internal_access::allocation(state, user_id).await?;
        return compile_internal_policy(&config, &allocation, model, now_unix_secs);
    }
    let cache_key = user_id.to_string();
    let policy = state
        .auth_plan_usage_policy_cache
        .get_or_load(cache_key.clone(), POLICY_CACHE_TTL, || async move {
            let _permit = state.acquire_auth_snapshot_load_gate().await?;
            let entitlements = state
                .list_user_plan_entitlements(user_id)
                .await?
                .unwrap_or_default();
            compile_effective_policy(&entitlements, now_unix_secs).map(Some)
        })
        .await
        .map(|policy| policy.unwrap_or_default())?;
    if policy
        .valid_until_unix_secs
        .is_some_and(|valid_until| valid_until <= now_unix_secs)
    {
        state.auth_plan_usage_policy_cache.clear();
        let _permit = state.acquire_auth_snapshot_load_gate().await?;
        let entitlements = state
            .list_user_plan_entitlements(user_id)
            .await?
            .unwrap_or_default();
        let policy = compile_effective_policy(&entitlements, now_unix_secs)?;
        state.auth_plan_usage_policy_cache.insert(
            cache_key,
            Some(policy.clone()),
            POLICY_CACHE_TTL,
        );
        return Ok(policy);
    }
    Ok(policy)
}

fn compile_internal_policy(
    config: &crate::internal_access::InternalAccessConfig,
    allocation: &crate::internal_access::Allocation,
    model: Option<&str>,
    now: u64,
) -> Result<EffectivePlanUsagePolicy, GatewayError> {
    if allocation.unlimited {
        return Ok(EffectivePlanUsagePolicy::default());
    }
    let model = model.ok_or_else(|| GatewayError::Client {
        status: http::StatusCode::BAD_REQUEST,
        message: "a model is required to evaluate internal usage budgets".into(),
    })?;
    let mut result = EffectivePlanUsagePolicy {
        model: Some(model.to_string()),
        ..Default::default()
    };
    let mut cost_rules = BTreeMap::<String, EffectiveCostRule>::new();
    for scoped in config
        .scoped_policies(allocation)
        .map_err(GatewayError::Internal)?
    {
        let scope = scoped.model;
        if scope.as_deref().is_some_and(|m| m != model) {
            continue;
        }
        if scoped
            .models
            .as_ref()
            .is_some_and(|models| !models.iter().any(|m| m == model))
        {
            continue;
        }
        let entitlement = UserPlanEntitlementRecord {
            id: "internal".into(),
            user_id: "internal".into(),
            plan_id: "internal".into(),
            payment_order_id: String::new(),
            status: "active".into(),
            starts_at_unix_secs: 0,
            expires_at_unix_secs: 253_402_300_799,
            entitlements_snapshot: serde_json::json!([scoped.policy]),
            created_at_unix_secs: now,
            updated_at_unix_secs: now,
        };
        let compiled = compile_effective_policy(&[entitlement], now)?;
        for mut rule in compiled.cost_rules {
            if let Some(scope) = &scope {
                use sha2::Digest;
                let hash = format!("{:x}", sha2::Sha256::digest(scope.as_bytes()));
                rule.identity = format!("{}:model:{hash}", rule.identity);
                rule.model = Some(scope.clone());
            }
            if let Some(family) = &scoped.family {
                rule.identity = format!("{}:family:{family}", rule.identity);
                rule.models = scoped.models.clone();
            }
            // A wildcard and total budget can address the same aggregate window.
            cost_rules
                .entry(rule.identity.clone())
                .and_modify(|current| {
                    current.limit_cost_units = current.limit_cost_units.min(rule.limit_cost_units);
                })
                .or_insert(rule);
        }
    }
    result.cost_rules = cost_rules.into_values().collect();
    Ok(result)
}

fn compile_effective_policy(
    entitlements: &[UserPlanEntitlementRecord],
    now_unix_secs: u64,
) -> Result<EffectivePlanUsagePolicy, GatewayError> {
    let mut request_rules = BTreeMap::<String, EffectiveRequestRule>::new();
    let mut cost_rules = BTreeMap::<String, EffectiveCostRule>::new();
    let mut concurrency_limit = None::<u64>;
    let mut valid_until_unix_secs = None::<u64>;

    for entitlement in entitlements.iter().filter(|entitlement| {
        entitlement.status == "active"
            && entitlement.starts_at_unix_secs <= now_unix_secs
            && entitlement.expires_at_unix_secs > now_unix_secs
    }) {
        valid_until_unix_secs = Some(
            valid_until_unix_secs.map_or(entitlement.expires_at_unix_secs, |current| {
                current.min(entitlement.expires_at_unix_secs)
            }),
        );
        let policies = parse_usage_policy_entitlements(&entitlement.entitlements_snapshot)
            .map_err(|error| GatewayError::Internal(error.to_string()))?;
        for policy in policies {
            for rule in policy.rules {
                match rule.metric {
                    UsagePolicyMetric::Concurrency => {
                        let limit = rule.request_limit().ok_or_else(|| {
                            GatewayError::Internal(
                                "validated concurrency usage policy lost its integer limit"
                                    .to_string(),
                            )
                        })?;
                        concurrency_limit =
                            Some(concurrency_limit.map_or(limit, |current| current.min(limit)));
                    }
                    UsagePolicyMetric::RequestCount => {
                        let limit = rule.request_limit().ok_or_else(|| {
                            GatewayError::Internal(
                                "validated request-count usage policy lost its integer limit"
                                    .to_string(),
                            )
                        })?;
                        let (identity, entitlement_period) = match &rule.window {
                            UsagePolicyWindow::SubscriptionPeriod => (
                                format!("subscription:{}", entitlement.id),
                                Some((
                                    entitlement.starts_at_unix_secs,
                                    entitlement.expires_at_unix_secs,
                                )),
                            ),
                            window => (window_identity(window), None),
                        };
                        let candidate = EffectiveRequestRule {
                            model: None,
                            identity: identity.clone(),
                            window: rule.window,
                            limit,
                            entitlement_period,
                        };
                        request_rules
                            .entry(identity)
                            .and_modify(|current| {
                                if candidate.limit < current.limit {
                                    *current = candidate.clone();
                                }
                            })
                            .or_insert(candidate);
                    }
                    UsagePolicyMetric::ActualCostUsd => {
                        let limit_cost_units = rule.cost_limit_units().ok_or_else(|| {
                            GatewayError::Internal(
                                "validated cost usage policy lost its fixed-point limit"
                                    .to_string(),
                            )
                        })?;
                        let (identity, entitlement_period) = match &rule.window {
                            UsagePolicyWindow::SubscriptionPeriod => (
                                format!("subscription:{}", entitlement.id),
                                Some((
                                    entitlement.starts_at_unix_secs,
                                    entitlement.expires_at_unix_secs,
                                )),
                            ),
                            window => (window_identity(window), None),
                        };
                        let candidate = EffectiveCostRule {
                            model: None,
                            models: None,
                            identity: identity.clone(),
                            window: rule.window,
                            limit_cost_units,
                            entitlement_period,
                        };
                        cost_rules
                            .entry(identity)
                            .and_modify(|current| {
                                if candidate.limit_cost_units < current.limit_cost_units {
                                    *current = candidate.clone();
                                }
                            })
                            .or_insert(candidate);
                    }
                }
            }
        }
    }

    Ok(EffectivePlanUsagePolicy {
        model: None,
        request_rules: request_rules.into_values().collect(),
        cost_rules: cost_rules.into_values().collect(),
        concurrency_limit,
        valid_until_unix_secs,
    })
}

fn window_identity(window: &UsagePolicyWindow) -> String {
    match window {
        UsagePolicyWindow::Rolling { seconds } => format!("rolling:{seconds}"),
        UsagePolicyWindow::CalendarDay { timezone } => {
            format!(
                "calendar_day:{}",
                effective_timezone_name(timezone.as_deref())
            )
        }
        UsagePolicyWindow::CalendarWeek {
            timezone,
            week_start,
        } => format!(
            "calendar_week:{}:{week_start}",
            effective_timezone_name(timezone.as_deref())
        ),
        UsagePolicyWindow::CalendarMonth { timezone } => {
            format!(
                "calendar_month:{}",
                effective_timezone_name(timezone.as_deref())
            )
        }
        UsagePolicyWindow::SubscriptionPeriod => "subscription_period".to_string(),
        UsagePolicyWindow::Concurrent => "concurrent".to_string(),
    }
}

fn runtime_request_rule(
    user_id: &str,
    rule: &EffectiveRequestRule,
    now_unix_secs: u64,
) -> Result<RuntimeRequestRule, PlanUsageAdmissionError> {
    let (bucket, window_seconds, retry_after, window) = match &rule.window {
        UsagePolicyWindow::Rolling { seconds } => (
            "sliding".to_string(),
            *seconds,
            *seconds,
            rolling_label(*seconds),
        ),
        UsagePolicyWindow::CalendarDay { timezone } => {
            calendar_bucket(now_unix_secs, timezone.as_deref(), CalendarWindow::Day, 1)?
        }
        UsagePolicyWindow::CalendarWeek {
            timezone,
            week_start,
        } => calendar_bucket(
            now_unix_secs,
            timezone.as_deref(),
            CalendarWindow::Week,
            *week_start,
        )?,
        UsagePolicyWindow::CalendarMonth { timezone } => {
            calendar_bucket(now_unix_secs, timezone.as_deref(), CalendarWindow::Month, 1)?
        }
        UsagePolicyWindow::SubscriptionPeriod => {
            let (starts_at, expires_at) = rule.entitlement_period.ok_or_else(|| {
                PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                    "subscription period usage policy is missing its entitlement period"
                        .to_string(),
                ))
            })?;
            let window_seconds = expires_at.saturating_sub(starts_at).max(1);
            let remaining = expires_at.saturating_sub(now_unix_secs).max(1);
            (
                starts_at.to_string(),
                window_seconds,
                remaining,
                "subscription_period",
            )
        }
        UsagePolicyWindow::Concurrent => {
            return Err(PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                "concurrent window reached request counter compiler".to_string(),
            )));
        }
    };
    Ok(RuntimeRequestRule {
        key: format!("plan-usage:user:{{{user_id}}}:{}:{bucket}", rule.identity),
        limit: rule.limit,
        window_seconds: window_seconds.max(1),
        retention_seconds: retry_after.max(1),
        retry_after: retry_after.max(1),
        window,
    })
}

fn durable_request_rule(
    rule: &EffectiveRequestRule,
    admitted_at_unix_secs: u64,
) -> Result<DurableRequestRule, PlanUsageAdmissionError> {
    let (starts_at, ends_at, retry_after, label) =
        usage_window_bounds(&rule.window, rule.entitlement_period, admitted_at_unix_secs)
            .map_err(PlanUsageAdmissionError::Gateway)?;
    Ok(DurableRequestRule {
        window: UsagePolicyRequestWindow {
            model: rule.model.clone(),
            starts_at_unix_secs: starts_at,
            ends_at_unix_secs: ends_at,
            limit_requests: rule.limit,
        },
        retry_after,
        label,
        influence_ends_at_unix_secs: match &rule.window {
            UsagePolicyWindow::Rolling { seconds } => admitted_at_unix_secs
                .saturating_add(*seconds)
                .saturating_add(1),
            _ => ends_at,
        },
    })
}

fn runtime_cost_rule(
    rule: &EffectiveCostRule,
    admitted_at_unix_secs: u64,
) -> Result<RuntimeCostRule, GatewayError> {
    let (starts_at, ends_at, retry_after, label) =
        usage_window_bounds(&rule.window, rule.entitlement_period, admitted_at_unix_secs)?;
    Ok(RuntimeCostRule {
        window: UsagePolicyCostWindow {
            rolling_seconds: match rule.window {
                UsagePolicyWindow::Rolling { seconds } => Some(seconds),
                _ => None,
            },
            model: rule.model.clone(),
            models: rule.models.clone(),
            window_id: format!("{}:{starts_at}", rule.identity),
            starts_at_unix_secs: starts_at,
            ends_at_unix_secs: ends_at,
            limit_cost_units: rule.limit_cost_units,
        },
        retry_after,
        label,
        influence_ends_at_unix_secs: match &rule.window {
            UsagePolicyWindow::Rolling { seconds } => admitted_at_unix_secs
                .saturating_add(*seconds)
                .saturating_add(1),
            _ => ends_at,
        },
    })
}

pub(crate) fn usage_window_bounds(
    window: &UsagePolicyWindow,
    entitlement_period: Option<(u64, u64)>,
    now_unix_secs: u64,
) -> Result<(u64, u64, u64, &'static str), GatewayError> {
    match window {
        UsagePolicyWindow::Rolling { seconds } => Ok((
            now_unix_secs.saturating_sub(*seconds),
            now_unix_secs.saturating_add(1),
            *seconds,
            rolling_label(*seconds),
        )),
        UsagePolicyWindow::CalendarDay { timezone } => {
            calendar_window_bounds(now_unix_secs, timezone.as_deref(), CalendarWindow::Day, 1)
        }
        UsagePolicyWindow::CalendarWeek {
            timezone,
            week_start,
        } => calendar_window_bounds(
            now_unix_secs,
            timezone.as_deref(),
            CalendarWindow::Week,
            *week_start,
        ),
        UsagePolicyWindow::CalendarMonth { timezone } => {
            calendar_window_bounds(now_unix_secs, timezone.as_deref(), CalendarWindow::Month, 1)
        }
        UsagePolicyWindow::SubscriptionPeriod => {
            let (starts_at, ends_at) = entitlement_period.ok_or_else(|| {
                GatewayError::Internal(
                    "subscription period cost policy is missing its entitlement period".to_string(),
                )
            })?;
            Ok((
                starts_at,
                ends_at,
                ends_at.saturating_sub(now_unix_secs).max(1),
                "subscription_period",
            ))
        }
        UsagePolicyWindow::Concurrent => Err(GatewayError::Internal(
            "concurrent window reached cost policy compiler".to_string(),
        )),
    }
}

fn rolling_label(seconds: u64) -> &'static str {
    match seconds {
        1 => "qps",
        60 => "rpm",
        18_000 => "rolling_5h",
        _ => "rolling",
    }
}

#[derive(Clone, Copy)]
enum CalendarWindow {
    Day,
    Week,
    Month,
}

fn calendar_bucket(
    now_unix_secs: u64,
    timezone: Option<&str>,
    kind: CalendarWindow,
    week_start: u8,
) -> Result<(String, u64, u64, &'static str), PlanUsageAdmissionError> {
    let timezone = effective_timezone_name(timezone)
        .parse::<chrono_tz::Tz>()
        .map_err(|_| {
            PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                "usage policy contains an invalid timezone".to_string(),
            ))
        })?;
    let now = Utc
        .timestamp_opt(i64::try_from(now_unix_secs).unwrap_or(i64::MAX), 0)
        .single()
        .ok_or_else(|| {
            PlanUsageAdmissionError::Gateway(GatewayError::Internal(
                "usage policy timestamp is out of range".to_string(),
            ))
        })?
        .with_timezone(&timezone);
    let current_date = now.date_naive();
    let (start_date, end_date, label) = match kind {
        CalendarWindow::Day => (
            current_date,
            current_date.succ_opt().ok_or_else(date_overflow)?,
            "calendar_day",
        ),
        CalendarWindow::Week => {
            let current = weekday_number(current_date.weekday());
            let elapsed = (current + 7 - u32::from(week_start)) % 7;
            let start = current_date - chrono::Duration::days(i64::from(elapsed));
            (
                start,
                start
                    .checked_add_days(chrono::Days::new(7))
                    .ok_or_else(date_overflow)?,
                "calendar_week",
            )
        }
        CalendarWindow::Month => {
            let start = current_date.with_day(1).ok_or_else(date_overflow)?;
            let (year, month) = if start.month() == 12 {
                (start.year() + 1, 1)
            } else {
                (start.year(), start.month() + 1)
            };
            (
                start,
                chrono::NaiveDate::from_ymd_opt(year, month, 1).ok_or_else(date_overflow)?,
                "calendar_month",
            )
        }
    };
    let start = timezone
        .from_local_datetime(&start_date.and_hms_opt(0, 0, 0).ok_or_else(date_overflow)?)
        .earliest()
        .ok_or_else(date_overflow)?;
    let end = timezone
        .from_local_datetime(&end_date.and_hms_opt(0, 0, 0).ok_or_else(date_overflow)?)
        .latest()
        .ok_or_else(date_overflow)?;
    let end_unix = u64::try_from(end.timestamp()).map_err(|_| date_overflow())?;
    let start_unix = u64::try_from(start.timestamp()).map_err(|_| date_overflow())?;
    let window_seconds = end_unix.saturating_sub(start_unix).max(1);
    let remaining = end_unix.saturating_sub(now_unix_secs).max(1);
    Ok((
        start.timestamp().to_string(),
        window_seconds,
        remaining,
        label,
    ))
}

fn calendar_window_bounds(
    now_unix_secs: u64,
    timezone: Option<&str>,
    kind: CalendarWindow,
    week_start: u8,
) -> Result<(u64, u64, u64, &'static str), GatewayError> {
    let (start, window_seconds, retry_after, label) =
        calendar_bucket(now_unix_secs, timezone, kind, week_start).map_err(
            |error| match error {
                PlanUsageAdmissionError::Gateway(error) => error,
                PlanUsageAdmissionError::Runtime(error) => {
                    GatewayError::Internal(error.to_string())
                }
                PlanUsageAdmissionError::Rejected(_) => GatewayError::Internal(
                    "calendar usage policy compilation unexpectedly rejected a request".to_string(),
                ),
            },
        )?;
    let starts_at_unix_secs = start.parse::<u64>().map_err(|_| {
        GatewayError::Internal("calendar usage policy produced an invalid epoch".to_string())
    })?;
    Ok((
        starts_at_unix_secs,
        starts_at_unix_secs.saturating_add(window_seconds),
        retry_after,
        label,
    ))
}

fn effective_timezone_name(explicit: Option<&str>) -> String {
    explicit
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToOwned::to_owned)
        .or_else(|| {
            std::env::var("APP_TIMEZONE")
                .ok()
                .map(|value| value.trim().to_string())
                .filter(|value| value.parse::<chrono_tz::Tz>().is_ok())
        })
        .unwrap_or_else(|| DEFAULT_CALENDAR_TIMEZONE.to_string())
}

fn weekday_number(weekday: Weekday) -> u32 {
    weekday.num_days_from_monday() + 1
}

fn date_overflow() -> PlanUsageAdmissionError {
    PlanUsageAdmissionError::Gateway(GatewayError::Internal(
        "usage policy calendar window is out of range".to_string(),
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn explicit_zero_money_budget_compiles_to_a_real_zero_capacity_window() {
        let config =
            crate::internal_access::parse(json!({"default":{"monthly_budget_usd":0}})).unwrap();
        let compiled = compile_internal_policy(
            &config,
            config.default.as_ref().unwrap(),
            Some("paid-model"),
            1_760_000_000,
        )
        .unwrap();
        assert_eq!(compiled.cost_rules.len(), 1);
        assert_eq!(compiled.cost_rules[0].limit_cost_units, 0);
        assert_eq!(
            runtime_cost_rule(&compiled.cost_rules[0], 1_760_000_000)
                .unwrap()
                .window
                .limit_cost_units,
            0
        );
    }

    #[test]
    fn internal_families_compile_one_selector_per_matching_pool_without_extra_reservations() {
        let config = crate::internal_access::parse(json!({"model_families":{
            "fabel":{"name":"Fabel","models":["a","b"]},"premium":{"name":"Premium","models":["b","c"]}
        },"default":{"weekly_budget_usd":20,"families":{"fabel":{"weekly_budget_usd":5},"premium":{"weekly_budget_usd":3}}}})).unwrap();
        let allocation = config.default.as_ref().unwrap();
        for (model, expected) in [("a", 2), ("b", 3), ("c", 2), ("other", 1)] {
            let compiled =
                compile_internal_policy(&config, allocation, Some(model), 1_792_152_000).unwrap();
            assert_eq!(compiled.cost_rules.len(), expected);
            assert_eq!(compiled.model.as_deref(), Some(model));
            for rule in &compiled.cost_rules {
                let window = runtime_cost_rule(rule, 1_792_152_000).unwrap().window;
                if let Some(models) = window.models {
                    assert!(models.contains(&model.to_string()));
                }
            }
        }
    }

    #[test]
    fn internal_policy_compiles_total_money_and_only_current_model_money() {
        let config = crate::internal_access::parse(json!({"timezone":"Asia/Shanghai","default":{
            "five_hour_budget_usd":2.5,"weekly_budget_usd":20,"monthly_budget_usd":100,"models":{"glm":{"five_hour_budget_usd":1.25,"daily_budget_usd":5,"weekly_budget_usd":10,"monthly_budget_usd":50},"claude":{"daily_budget_usd":5}}
        }})).unwrap();
        let allocation = config.default.as_ref().unwrap();
        let now = 1_760_000_000;
        let compiled = compile_internal_policy(&config, allocation, Some("glm"), now).unwrap();
        assert!(compiled.request_rules.is_empty());
        assert_eq!(compiled.cost_rules.len(), 7);
        assert_eq!(compiled.model.as_deref(), Some("glm"));
        assert_eq!(
            compiled
                .cost_rules
                .iter()
                .filter(|r| r.model.is_none())
                .count(),
            3
        );
        let model_rules: Vec<_> = compiled
            .cost_rules
            .iter()
            .filter(|r| r.model.is_some())
            .collect();
        assert_eq!(model_rules.len(), 4);
        assert!(compiled
            .cost_rules
            .iter()
            .all(|r| r.model.is_none() || r.model.as_deref() == Some("glm")));
        let window = runtime_cost_rule(model_rules[0], now).unwrap();
        assert_eq!(window.window.model.as_deref(), Some("glm"));
        let ordinary =
            compile_internal_policy(&config, allocation, Some("other-authorized-model"), now)
                .unwrap();
        assert_eq!(ordinary.cost_rules.len(), 3);
        assert!(ordinary.cost_rules.iter().all(|r| r.model.is_none()));
        let mut total_only = allocation.clone();
        total_only.models.clear();
        assert_eq!(
            compile_internal_policy(&config, &total_only, Some("glm"), now)
                .unwrap()
                .cost_rules
                .len(),
            3
        );
        let unlimited = crate::internal_access::Allocation {
            unlimited: true,
            ..Default::default()
        };
        let compiled = compile_internal_policy(&config, &unlimited, Some("glm"), 100).unwrap();
        assert!(compiled.request_rules.is_empty() && compiled.cost_rules.is_empty());
    }

    fn entitlement(id: &str, snapshot: serde_json::Value) -> UserPlanEntitlementRecord {
        UserPlanEntitlementRecord {
            id: id.to_string(),
            user_id: "user-1".to_string(),
            plan_id: "plan-1".to_string(),
            payment_order_id: "order-1".to_string(),
            status: "active".to_string(),
            starts_at_unix_secs: 1_000,
            expires_at_unix_secs: 10_000,
            entitlements_snapshot: snapshot,
            created_at_unix_secs: 1_000,
            updated_at_unix_secs: 1_000,
        }
    }

    #[test]
    fn wildcard_money_and_total_money_use_stricter_shared_window_without_double_charging() {
        let config = crate::internal_access::parse(json!({"default":{
            "five_hour_budget_usd":5,"weekly_budget_usd":20,"models":{
                "*":{"five_hour_budget_usd":3,"weekly_budget_usd":30},
                "glm":{"five_hour_budget_usd":1.25}
            }
        }}))
        .unwrap();
        let compiled = compile_internal_policy(
            &config,
            config.default.as_ref().unwrap(),
            Some("glm"),
            1_760_000_000,
        )
        .unwrap();
        assert_eq!(compiled.cost_rules.len(), 3);
        let global = compiled
            .cost_rules
            .iter()
            .find(|r| r.model.is_none() && matches!(r.window, UsagePolicyWindow::Rolling { .. }))
            .unwrap();
        assert_eq!(global.limit_cost_units, 3 * USAGE_POLICY_COST_UNITS_PER_USD);
        let scoped = compiled
            .cost_rules
            .iter()
            .find(|r| r.model.is_some())
            .unwrap();
        assert_eq!(scoped.limit_cost_units, 125_000_000);
        let windows: Vec<_> = compiled
            .cost_rules
            .iter()
            .map(|r| runtime_cost_rule(r, 1_760_000_000).unwrap().window)
            .collect();
        ReserveUsagePolicyCostInput {
            model: compiled.model.clone(),
            request_id: "r".into(),
            subject_id: "u".into(),
            reservation_token: "t".into(),
            admitted_at_unix_secs: 1_760_000_000,
            reserved_cost_units: 1,
            reservation_expires_at_unix_secs: 1_760_001_000,
            retain_until_unix_secs: 1_800_000_000,
            windows,
        }
        .validate()
        .unwrap();
    }

    #[test]
    fn compiles_week_only_and_combined_rules() {
        let policy = compile_effective_policy(
            &[entitlement(
                "ent-1",
                json!([{
                    "type": "usage_policy",
                    "rules": [
                        {"metric":"request_count","window":{"kind":"rolling","seconds":18000},"limit":500},
                        {"metric":"request_count","window":{"kind":"calendar_week","timezone":"Asia/Shanghai"},"limit":10000},
                        {"metric":"concurrency","window":{"kind":"concurrent"},"limit":4}
                    ]
                }]),
            )],
            2_000,
        )
        .expect("policy");
        assert_eq!(policy.request_rules.len(), 2);
        assert_eq!(policy.concurrency_limit, Some(4));
    }

    #[test]
    fn policy_snapshot_exists_only_for_cost_rules_and_derives_unique_tokens() {
        assert!(PlanUsagePolicySnapshot::for_admission(
            "user-1",
            EffectivePlanUsagePolicy::default(),
            2_000,
        )
        .is_none());

        let policy = compile_effective_policy(
            &[entitlement(
                "ent-cost",
                json!([{"type":"usage_policy","rules":[
                    {"metric":"actual_cost_usd","window":{"kind":"subscription_period"},"limit":10.0}
                ]}]),
            )],
            2_000,
        )
        .expect("cost policy");
        let snapshot = PlanUsagePolicySnapshot::for_admission("user-1", policy.clone(), 2_000)
            .expect("cost rules require a policy snapshot");
        let reservation = snapshot.new_reservation_context();
        let retry_reservation = snapshot.new_reservation_context();

        assert_eq!(reservation.subject_id(), "user-1");
        assert_eq!(reservation.admitted_at_unix_secs(), 2_000);
        assert_eq!(reservation.policy(), &policy);
        assert!(!reservation.token().trim().is_empty());
        assert_ne!(reservation.token(), retry_reservation.token());
        assert_eq!(retry_reservation.admitted_at_unix_secs(), 2_000);
        assert!(std::ptr::eq(
            reservation.policy(),
            retry_reservation.policy()
        ));
    }

    #[test]
    fn admitted_cost_snapshot_keeps_original_window_after_entitlement_expiry() {
        let policy = compile_effective_policy(
            &[entitlement(
                "ent-cost",
                json!([{"type":"usage_policy","rules":[
                    {"metric":"actual_cost_usd","window":{"kind":"subscription_period"},"limit":10.0}
                ]}]),
            )],
            9_999,
        )
        .expect("policy before expiry");
        let snapshot = PlanUsagePolicySnapshot::for_admission("user-1", policy, 9_999)
            .expect("cost policy snapshot");

        let rule = snapshot
            .policy()
            .cost_rules
            .first()
            .expect("snapshotted cost rule");
        let runtime = runtime_cost_rule(rule, snapshot.admitted_at_unix_secs)
            .expect("snapshot remains compilable at its admitted timestamp");
        assert_eq!(runtime.window.starts_at_unix_secs, 1_000);
        assert_eq!(runtime.window.ends_at_unix_secs, 10_000);
        assert_eq!(runtime.retry_after, 1);

        assert!(compile_effective_policy(
            &[entitlement(
                "ent-cost",
                json!([{"type":"usage_policy","rules":[
                    {"metric":"actual_cost_usd","window":{"kind":"subscription_period"},"limit":10.0}
                ]}]),
            )],
            10_000,
        )
        .expect("policy at expiry")
        .cost_rules
        .is_empty());
    }

    #[test]
    fn same_window_uses_strictest_limit_but_subscription_periods_stay_independent() {
        let first = entitlement(
            "ent-a",
            json!([{"type":"usage_policy","rules":[
                {"metric":"request_count","window":{"kind":"rolling","seconds":60},"limit":100},
                {"metric":"request_count","window":{"kind":"subscription_period"},"limit":1000}
            ]}]),
        );
        let second = entitlement(
            "ent-b",
            json!([{"type":"usage_policy","rules":[
                {"metric":"request_count","window":{"kind":"rolling","seconds":60},"limit":40},
                {"metric":"request_count","window":{"kind":"subscription_period"},"limit":2000}
            ]}]),
        );
        let policy = compile_effective_policy(&[first, second], 2_000).expect("policy");
        assert_eq!(policy.request_rules.len(), 3);
        assert_eq!(
            policy
                .request_rules
                .iter()
                .find(|rule| rule.identity == "rolling:60")
                .map(|rule| rule.limit),
            Some(40)
        );
    }

    #[test]
    fn rolling_windows_use_stable_sliding_keys_and_calendar_windows_use_epoch_keys() {
        let rolling = EffectiveRequestRule {
            model: None,
            identity: "rolling:18000".to_string(),
            window: UsagePolicyWindow::Rolling { seconds: 18_000 },
            limit: 10,
            entitlement_period: None,
        };
        let first = runtime_request_rule("user-1", &rolling, 20_000).expect("first rolling");
        let second = runtime_request_rule("user-1", &rolling, 40_000).expect("second rolling");
        assert_eq!(first.key, second.key);
        assert_eq!(first.window_seconds, 18_000);
        assert_eq!(first.retention_seconds, 18_000);

        let weekly = EffectiveRequestRule {
            model: None,
            identity: "calendar_week:Asia/Shanghai:1".to_string(),
            window: UsagePolicyWindow::CalendarWeek {
                timezone: Some("Asia/Shanghai".to_string()),
                week_start: 1,
            },
            limit: 20,
            entitlement_period: None,
        };
        let sunday = chrono::DateTime::parse_from_rfc3339("2026-08-16T12:00:00+08:00")
            .unwrap()
            .timestamp() as u64;
        let monday = chrono::DateTime::parse_from_rfc3339("2026-08-17T12:00:00+08:00")
            .unwrap()
            .timestamp() as u64;
        let first = runtime_request_rule("user-1", &weekly, sunday).expect("first week");
        let second = runtime_request_rule("user-1", &weekly, monday).expect("second week");
        assert_ne!(first.key, second.key);
        assert_eq!(first.window, "calendar_week");
        assert_eq!(first.retention_seconds, first.retry_after);
        assert!(first.retention_seconds < first.window_seconds);
    }

    #[test]
    fn subscription_period_counter_keeps_the_entitlement_epoch_isolated() {
        let rule = EffectiveRequestRule {
            model: None,
            identity: "subscription:ent-1".to_string(),
            window: UsagePolicyWindow::SubscriptionPeriod,
            limit: 5,
            entitlement_period: Some((1_000, 10_000)),
        };
        let runtime = runtime_request_rule("user-1", &rule, 9_500).expect("subscription rule");
        assert!(runtime.key.ends_with(":subscription:ent-1:1000"));
        assert_eq!(runtime.window_seconds, 9_000);
        assert_eq!(runtime.retention_seconds, 500);
        assert_eq!(runtime.retry_after, 500);
    }

    #[test]
    fn calendar_bucket_retention_uses_only_the_short_remaining_period() {
        let daily = EffectiveRequestRule {
            model: None,
            identity: "calendar_day:Asia/Shanghai".to_string(),
            window: UsagePolicyWindow::CalendarDay {
                timezone: Some("Asia/Shanghai".to_string()),
            },
            limit: 5,
            entitlement_period: None,
        };
        let near_midnight = chrono::DateTime::parse_from_rfc3339("2026-08-18T23:59:59+08:00")
            .unwrap()
            .timestamp() as u64;
        let runtime = runtime_request_rule("user-1", &daily, near_midnight).expect("daily rule");

        assert_eq!(runtime.window_seconds, 24 * 60 * 60);
        assert_eq!(runtime.retention_seconds, 1);
        assert_eq!(runtime.retry_after, 1);
    }

    #[test]
    fn cost_policy_fails_closed_when_usage_runtime_is_disabled() {
        assert!(ensure_cost_policy_usage_runtime_enabled(true).is_ok());
        assert!(matches!(
            ensure_cost_policy_usage_runtime_enabled(false),
            Err(GatewayError::Internal(message))
                if message.contains("usage runtime")
                    && message.contains("terminal reconciliation")
        ));
    }

    #[test]
    fn cost_retention_uses_exclusive_rolling_influence_end() {
        let rule = EffectiveCostRule {
            model: None,
            models: None,
            identity: "rolling:18000".to_string(),
            window: UsagePolicyWindow::Rolling { seconds: 18_000 },
            limit_cost_units: 100,
            entitlement_period: None,
        };
        let runtime = runtime_cost_rule(&rule, 20_000).expect("runtime cost rule");
        assert_eq!(runtime.influence_ends_at_unix_secs, 38_001);
    }

    #[test]
    fn explicit_cost_release_uses_server_token_and_zero_actual_cost() {
        let input = build_usage_policy_cost_release_input(
            "request-1",
            "user-1",
            "server-reservation-token",
            12_345,
        );

        assert_eq!(input.request_id, "request-1");
        assert_eq!(input.subject_id, "user-1");
        assert_eq!(input.reservation_token, "server-reservation-token");
        assert_eq!(input.actual_cost_units, 0);
        assert_eq!(
            input.terminal_state,
            UsagePolicyCostReservationState::Released
        );
        assert_eq!(input.finalized_at_unix_secs, 12_345);
        input.validate().expect("release input should validate");
    }
}
