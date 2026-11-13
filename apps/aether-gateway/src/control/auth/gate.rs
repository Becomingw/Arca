use axum::body::Bytes;
use axum::http::Uri;

use super::super::GatewayControlDecision;
use super::credentials::{contains_string, extract_requested_model};
use super::GatewayControlAuthContext;
use crate::stage_metrics::observe_gateway_stage_ms;
use crate::{AppState, GatewayError};

const DAILY_QUOTA_EPSILON_USD: f64 = 0.000_000_01;
const AUTH_PRICING_VALIDATION_CACHE_TTL: std::time::Duration = std::time::Duration::from_secs(5);
// Billing mutations clear this cache locally. The bounded stale window leaves
// room for cross-node propagation without synchronously reloading at every
// short TTL boundary.
const AUTH_CAPACITY_CACHE_STALE_TTL: std::time::Duration = std::time::Duration::from_secs(60);

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum GatewayLocalAuthRejection {
    InvalidApiKey,
    LockedApiKey,
    WalletUnavailable,
    BalanceDenied { remaining: Option<f64> },
    ProviderNotAllowed { provider: String },
    ApiFormatNotAllowed { api_format: String },
    ModelNotAllowed { model: String },
    IpNotAllowed { remote_ip: String },
}

pub(crate) fn trusted_auth_local_rejection(
    decision: Option<&GatewayControlDecision>,
    _headers: &http::HeaderMap,
) -> Option<GatewayLocalAuthRejection> {
    let decision = decision?;
    if decision.route_class.as_deref() != Some("ai_public") {
        return None;
    }

    decision
        .local_auth_rejection
        .clone()
        .or_else(|| decision.auth_context.as_ref()?.local_rejection.clone())
}

pub(crate) fn should_buffer_request_for_local_auth(
    decision: Option<&GatewayControlDecision>,
    headers: &http::HeaderMap,
) -> bool {
    let Some(decision) = decision else {
        return false;
    };
    decision.route_class.as_deref() == Some("ai_public")
        && decision.route_kind.as_deref() != Some("files")
        && crate::headers::is_json_request(headers)
}

pub(crate) async fn request_model_local_rejection(
    state: &AppState,
    decision: Option<&GatewayControlDecision>,
    uri: &Uri,
    headers: &http::HeaderMap,
    body: &Bytes,
) -> Result<Option<GatewayLocalAuthRejection>, GatewayError> {
    let Some(decision) = decision else {
        return Ok(None);
    };
    if decision.route_class.as_deref() != Some("ai_public") {
        return Ok(None);
    }
    let Some(auth_context) = decision.auth_context.as_ref() else {
        return Ok(None);
    };
    let requested_model = extract_requested_model(decision, uri, headers, body);
    if let (Some(allowed_models), Some(requested_model)) = (
        auth_context.allowed_models.as_deref(),
        requested_model.as_deref(),
    ) {
        if !contains_string(allowed_models, requested_model)
            && !model_directive_base_model_is_allowed_for_request(
                decision,
                requested_model,
                allowed_models,
            )
            && !request_model_resolves_to_allowed_model(
                state,
                decision,
                requested_model,
                allowed_models,
            )
            .await?
        {
            return Ok(Some(GatewayLocalAuthRejection::ModelNotAllowed {
                model: requested_model.to_string(),
            }));
        }
    }

    Ok(None)
}

pub(crate) async fn execution_plan_balance_capacity_rejection(
    state: &AppState,
    decision: &GatewayControlDecision,
    plan: &aether_contracts::ExecutionPlan,
    report_context: Option<&serde_json::Value>,
) -> Result<Option<GatewayLocalAuthRejection>, GatewayError> {
    let started_at = std::time::Instant::now();
    let result =
        execution_plan_balance_capacity_rejection_inner(state, decision, plan, report_context)
            .await;
    observe_gateway_stage_ms(
        "auth_capacity_total",
        started_at.elapsed().as_millis() as u64,
    );
    result
}

async fn execution_plan_balance_capacity_rejection_inner(
    state: &AppState,
    decision: &GatewayControlDecision,
    plan: &aether_contracts::ExecutionPlan,
    report_context: Option<&serde_json::Value>,
) -> Result<Option<GatewayLocalAuthRejection>, GatewayError> {
    let Some(auth_context) = decision.auth_context.as_ref() else {
        return Ok(None);
    };
    if auth_context.local_rejection.is_some() {
        return Ok(None);
    }
    if auth_context.api_key_is_standalone {
        validate_execution_plan_pricing_configuration_for_plan(state, plan, report_context).await?;
        return Ok(None);
    }
    let Some(available_usd) = available_balance_capacity_usd(state, auth_context).await? else {
        validate_execution_plan_pricing_configuration_for_plan(state, plan, report_context).await?;
        return Ok(None);
    };
    match estimate_execution_plan_cost_upper_bound_usd(state, plan, report_context).await? {
        Some(estimated_cost_usd)
            if estimated_cost_usd <= available_usd + DAILY_QUOTA_EPSILON_USD =>
        {
            Ok(None)
        }
        Some(_) | None if available_usd <= DAILY_QUOTA_EPSILON_USD => {
            Ok(Some(GatewayLocalAuthRejection::BalanceDenied {
                remaining: Some(0.0),
            }))
        }
        Some(_) => Ok(Some(GatewayLocalAuthRejection::BalanceDenied {
            remaining: Some(available_usd),
        })),
        None => Ok(None),
    }
}

async fn validate_execution_plan_pricing_configuration_for_plan(
    state: &AppState,
    plan: &aether_contracts::ExecutionPlan,
    report_context: Option<&serde_json::Value>,
) -> Result<(), GatewayError> {
    let model_id = report_context_string_field(report_context, "model_id");
    let global_model_name = report_context_string_field(report_context, "global_model_name");
    let requested_processing_tier =
        aether_data_contracts::repository::usage::extract_provider_service_tier_from_body(
            plan.body.json_body.as_ref(),
        );
    validate_execution_plan_pricing_for_unavailable_estimate(
        state,
        plan,
        model_id,
        global_model_name,
        requested_processing_tier.as_deref(),
    )
    .await
}

async fn available_balance_capacity_usd(
    state: &AppState,
    auth_context: &GatewayControlAuthContext,
) -> Result<Option<f64>, GatewayError> {
    if crate::internal_access::enabled() {
        return Ok(None);
    }
    let quota_started_at = std::time::Instant::now();
    let quota_result = state
        .find_user_daily_quota_availability_for_auth(&auth_context.user_id)
        .await;
    observe_gateway_stage_ms(
        "auth_capacity_quota",
        quota_started_at.elapsed().as_millis() as u64,
    );
    let quota = quota_result?.filter(|quota| quota.has_active_daily_quota);

    let wallet_started_at = std::time::Instant::now();
    let wallet_result = state
        .read_wallet_snapshot_for_auth(
            &auth_context.user_id,
            &auth_context.api_key_id,
            auth_context.api_key_is_standalone,
        )
        .await;
    observe_gateway_stage_ms(
        "auth_capacity_wallet",
        wallet_started_at.elapsed().as_millis() as u64,
    );
    let wallet = wallet_result?;
    let wallet_available_usd = wallet.as_ref().and_then(wallet_finite_available_usd);
    let wallet_is_unlimited = wallet
        .as_ref()
        .is_some_and(|wallet| wallet.limit_mode.eq_ignore_ascii_case("unlimited"));
    Ok(match quota.as_ref() {
        Some(_) if wallet_is_unlimited => None,
        Some(quota) if !quota.allow_wallet_overage => Some(quota.remaining_usd.max(0.0)),
        Some(quota) => Some(quota.remaining_usd.max(0.0) + wallet_available_usd.unwrap_or(0.0)),
        None if wallet_is_unlimited => None,
        None => wallet_available_usd,
    })
}

fn wallet_finite_available_usd(
    wallet: &aether_data::repository::wallet::StoredWalletSnapshot,
) -> Option<f64> {
    if !wallet.status.eq_ignore_ascii_case("active")
        || wallet.limit_mode.eq_ignore_ascii_case("unlimited")
    {
        return None;
    }
    Some(wallet.balance.max(0.0) + wallet.gift_balance.max(0.0))
}

pub(crate) async fn estimate_execution_plan_cost_upper_bound_usd(
    state: &AppState,
    plan: &aether_contracts::ExecutionPlan,
    report_context: Option<&serde_json::Value>,
) -> Result<Option<f64>, GatewayError> {
    let started_at = std::time::Instant::now();
    let result =
        estimate_execution_plan_cost_upper_bound_usd_inner(state, plan, report_context).await;
    observe_gateway_stage_ms(
        "auth_capacity_cost_estimate",
        started_at.elapsed().as_millis() as u64,
    );
    result
}

async fn estimate_execution_plan_cost_upper_bound_usd_inner(
    state: &AppState,
    plan: &aether_contracts::ExecutionPlan,
    report_context: Option<&serde_json::Value>,
) -> Result<Option<f64>, GatewayError> {
    let api_format = crate::ai_serving::normalize_api_format_alias(&plan.provider_api_format);
    let body_json = plan.body.json_body.as_ref();
    let requested_processing_tier =
        aether_data_contracts::repository::usage::extract_provider_service_tier_from_body(
            body_json,
        );
    let model_id = report_context_string_field(report_context, "model_id");
    let global_model_name = report_context_string_field(report_context, "global_model_name");
    let Some(task_type) = authorization_task_type(&api_format, report_context) else {
        validate_execution_plan_pricing_for_unavailable_estimate(
            state,
            plan,
            model_id,
            global_model_name,
            requested_processing_tier.as_deref(),
        )
        .await?;
        return Ok(None);
    };
    let Some(body_json) = body_json else {
        validate_execution_plan_pricing_for_unavailable_estimate(
            state,
            plan,
            model_id,
            global_model_name,
            requested_processing_tier.as_deref(),
        )
        .await?;
        return Ok(None);
    };
    if !request_input_is_self_contained(&api_format, body_json) {
        validate_execution_plan_pricing_for_unavailable_estimate(
            state,
            plan,
            model_id,
            global_model_name,
            requested_processing_tier.as_deref(),
        )
        .await?;
        return Ok(None);
    }
    let hosted_tools_surcharge_usd = hosted_tools_policy_cost_usd(body_json);
    let input_tokens = request_input_token_count_policy_bound(&api_format, body_json);
    let Ok(input_tokens) = i64::try_from(input_tokens) else {
        validate_execution_plan_pricing_for_unavailable_estimate(
            state,
            plan,
            model_id,
            global_model_name,
            requested_processing_tier.as_deref(),
        )
        .await?;
        return Ok(None);
    };
    let requested_output_tokens = max_output_tokens_from_request(body_json)
        .map(|value| value.saturating_mul(output_choice_count_upper_bound(&api_format, body_json)));
    let output_cap_requested = requested_output_tokens.is_some();
    let max_output_tokens = requested_output_tokens.and_then(|value| i64::try_from(value).ok());
    let cache_ttl_minutes =
        aether_data_contracts::repository::usage::resolve_provider_cache_ttl_minutes(
            Some(&api_format),
            plan.model_name.as_deref(),
            global_model_name,
            Some(body_json),
        );
    if model_id.is_none() && global_model_name.is_none() {
        return Ok(None);
    }
    let cache_key = execution_plan_cost_upper_bound_cache_key(
        plan,
        model_id,
        global_model_name,
        &api_format,
        input_tokens,
        max_output_tokens,
        output_cap_requested,
        requested_processing_tier.as_deref(),
        cache_ttl_minutes,
    );
    let estimate = estimate_execution_plan_token_cost_upper_bound(
        state,
        plan,
        model_id,
        global_model_name,
        &api_format,
        task_type,
        input_tokens,
        max_output_tokens,
        output_cap_requested,
        requested_processing_tier.as_deref(),
        cache_ttl_minutes,
        cache_key,
    )
    .await?;
    Ok(estimate.map(|cost| cost + hosted_tools_surcharge_usd))
}

#[allow(clippy::too_many_arguments)]
async fn estimate_execution_plan_token_cost_upper_bound(
    state: &AppState,
    plan: &aether_contracts::ExecutionPlan,
    model_id: Option<&str>,
    global_model_name: Option<&str>,
    api_format: &str,
    task_type: &str,
    input_tokens: i64,
    max_output_tokens: Option<i64>,
    output_cap_requested: bool,
    requested_processing_tier: Option<&str>,
    cache_ttl_minutes: Option<i64>,
    cache_key: String,
) -> Result<Option<f64>, GatewayError> {
    let ttl = state.frontdoor_runtime_guards.auth_capacity_cache_ttl;
    if ttl.is_zero() {
        let _permit = state.acquire_auth_snapshot_load_gate().await?;
        return calculate_execution_plan_cost_upper_bound(
            state,
            plan,
            model_id,
            global_model_name,
            api_format,
            task_type,
            input_tokens,
            max_output_tokens,
            output_cap_requested,
            requested_processing_tier,
            cache_ttl_minutes,
        )
        .await;
    }
    state
        .auth_request_cost_upper_bound_cache
        .get_or_load(cache_key, ttl, || async {
            let _permit = state.acquire_auth_snapshot_load_gate().await?;
            calculate_execution_plan_cost_upper_bound(
                state,
                plan,
                model_id,
                global_model_name,
                api_format,
                task_type,
                input_tokens,
                max_output_tokens,
                output_cap_requested,
                requested_processing_tier,
                cache_ttl_minutes,
            )
            .await
        })
        .await
}

#[allow(clippy::too_many_arguments)]
async fn calculate_execution_plan_cost_upper_bound(
    state: &AppState,
    plan: &aether_contracts::ExecutionPlan,
    model_id: Option<&str>,
    global_model_name: Option<&str>,
    api_format: &str,
    task_type: &str,
    input_tokens: i64,
    max_output_tokens: Option<i64>,
    output_cap_requested: bool,
    requested_processing_tier: Option<&str>,
    cache_ttl_minutes: Option<i64>,
) -> Result<Option<f64>, GatewayError> {
    let context =
        load_execution_plan_billing_context(state, plan, model_id, global_model_name).await?;
    let Some(context) = context else {
        return Ok(None);
    };
    // Clients such as Codex send no output cap. Reserving the model's full configured output limit
    // (often 128k tokens) makes the reservation dwarf the real cost, so — like new-api's
    // pre-consume — reserve only a small default output budget, capped by the configured limit.
    // Any overrun beyond the default is settled from the actual usage afterwards.
    // An explicit cap that does not fit keeps the previous fallback to the configured limit.
    let max_output_tokens = if output_cap_requested {
        max_output_tokens.or_else(|| configured_output_limit(&context))
    } else {
        Some(default_reserved_output_tokens(
            configured_output_limit(&context),
            usage_policy_default_output_tokens_from_env(),
        ))
    };
    let input_tokens = configured_token_limit(&context, "context_limit")
        .map_or(input_tokens, |limit| input_tokens.min(limit));
    let mut estimate =
        aether_billing::BillingAuthorizationEstimateInput::new(task_type, input_tokens);
    estimate.api_format = Some(api_format.to_string());
    estimate.requested_processing_tier = requested_processing_tier.map(ToOwned::to_owned);
    estimate.cache_ttl_minutes = cache_ttl_minutes;
    estimate.max_output_tokens = max_output_tokens;
    aether_billing::BillingService::new()
        .estimate_authorization_cost_upper_bound(
            &aether_billing::BillingModelPricingSnapshot::from(context),
            &estimate,
        )
        .map_err(|err| GatewayError::Internal(err.to_string()))
}

// Hosted tools are billed per call/session and their results are injected server-side, so the
// call count is unknown up front. Reserve a small fixed amount per distinct hosted tool on top of
// the token estimate; the actual usage is settled afterwards. Basis: new-api's per-call tool prices
// (setting/operation_setting/tools.go: web_search $10 / 1K calls, file_search $2.5 / 1K,
// google_search $14 / 1K, image_generation $150 / 1K). new-api has no price for code
// interpreter, MCP or computer use, so they share a small default like the output default.
const USAGE_POLICY_HOSTED_TOOL_COST_DEFAULTS: [(&str, f64); 3] = [
    // 5 calls at $10 / 1K.
    ("AETHER_GATEWAY_USAGE_POLICY_WEB_SEARCH_COST_USD", 0.05),
    // 1 call at $150 / 1K.
    (
        "AETHER_GATEWAY_USAGE_POLICY_IMAGE_GENERATION_COST_USD",
        0.15,
    ),
    // Any other hosted tool: code_interpreter, mcp, computer_use, file_search, ...
    ("AETHER_GATEWAY_USAGE_POLICY_HOSTED_TOOL_COST_USD", 0.05),
];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum HostedToolKind {
    WebSearch,
    ImageGeneration,
    Other,
}

fn hosted_tool_policy_cost_usd(kind: HostedToolKind) -> f64 {
    static VALUES: std::sync::OnceLock<[f64; 3]> = std::sync::OnceLock::new();
    let values = VALUES.get_or_init(|| {
        USAGE_POLICY_HOSTED_TOOL_COST_DEFAULTS.map(|(env, default)| {
            parse_usage_policy_tool_cost_usd(std::env::var(env).ok().as_deref(), default)
        })
    });
    values[kind as usize]
}

fn parse_usage_policy_tool_cost_usd(raw: Option<&str>, default: f64) -> f64 {
    raw.and_then(|value| value.trim().parse::<f64>().ok())
        .filter(|value| value.is_finite() && *value >= 0.0)
        .unwrap_or(default)
}

fn hosted_tool_kind(name: &str) -> HostedToolKind {
    let name = name.trim().to_ascii_lowercase();
    if name.starts_with("web_search") || name.starts_with("google_search") || name == "googlesearch"
    {
        HostedToolKind::WebSearch
    } else if name.starts_with("image_generation") {
        HostedToolKind::ImageGeneration
    } else {
        HostedToolKind::Other
    }
}

/// Distinct hosted tools in OpenAI (Responses tool types, Chat `web_search_options`),
/// Claude (non-custom tool types, `mcp_servers`) and Gemini (non-function tool keys).
fn request_hosted_tools(body: &serde_json::Value) -> Vec<HostedToolKind> {
    let mut kinds = Vec::new();
    if body.get("web_search_options").is_some() {
        kinds.push(HostedToolKind::WebSearch);
    }
    if body.get("mcp_servers").is_some_and(has_reference_value) {
        kinds.push(HostedToolKind::Other);
    }
    for tool in body
        .get("tools")
        .and_then(serde_json::Value::as_array)
        .into_iter()
        .flatten()
        .filter_map(serde_json::Value::as_object)
    {
        match tool.get("type").and_then(serde_json::Value::as_str) {
            Some("function" | "custom") => {}
            Some(kind) => kinds.push(hosted_tool_kind(kind)),
            // Untyped named tools are Claude custom tools; untyped unnamed ones are Gemini tools.
            None if tool.contains_key("name") => {}
            None => kinds.extend(
                tool.keys()
                    .filter(|key| {
                        !matches!(
                            key.as_str(),
                            "functionDeclarations" | "function_declarations"
                        )
                    })
                    .map(|key| hosted_tool_kind(key)),
            ),
        }
    }
    kinds.sort_by_key(|kind| *kind as usize);
    kinds.dedup();
    kinds
}

fn hosted_tools_policy_cost_usd(body: &serde_json::Value) -> f64 {
    request_hosted_tools(body)
        .into_iter()
        .map(hosted_tool_policy_cost_usd)
        .sum()
}

const USAGE_POLICY_DEFAULT_OUTPUT_TOKENS_ENV: &str =
    "AETHER_GATEWAY_USAGE_POLICY_DEFAULT_OUTPUT_TOKENS";
const DEFAULT_USAGE_POLICY_DEFAULT_OUTPUT_TOKENS: i64 = 4096;

fn usage_policy_default_output_tokens_from_env() -> i64 {
    static VALUE: std::sync::OnceLock<i64> = std::sync::OnceLock::new();
    *VALUE.get_or_init(|| {
        parse_usage_policy_default_output_tokens(
            std::env::var(USAGE_POLICY_DEFAULT_OUTPUT_TOKENS_ENV)
                .ok()
                .as_deref(),
        )
    })
}

fn parse_usage_policy_default_output_tokens(raw: Option<&str>) -> i64 {
    raw.and_then(|value| value.trim().parse::<i64>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(DEFAULT_USAGE_POLICY_DEFAULT_OUTPUT_TOKENS)
}

/// Output tokens to reserve when the client did not send an explicit output cap.
fn default_reserved_output_tokens(
    configured_output_limit: Option<i64>,
    default_tokens: i64,
) -> i64 {
    match configured_output_limit {
        Some(limit) if limit > 0 => default_tokens.min(limit),
        _ => default_tokens,
    }
}

fn configured_output_limit(
    context: &aether_data_contracts::repository::billing::StoredBillingModelContext,
) -> Option<i64> {
    configured_token_limit(context, "output_limit")
}

fn configured_token_limit(
    context: &aether_data_contracts::repository::billing::StoredBillingModelContext,
    field: &str,
) -> Option<i64> {
    [
        context.model_config.as_ref(),
        context.global_model_config.as_ref(),
    ]
    .into_iter()
    .flatten()
    .filter_map(|config| config.get(field))
    .filter_map(serde_json::Value::as_i64)
    .find(|limit| *limit > 0)
}

async fn validate_execution_plan_pricing_for_unavailable_estimate(
    state: &AppState,
    plan: &aether_contracts::ExecutionPlan,
    model_id: Option<&str>,
    global_model_name: Option<&str>,
    requested_processing_tier: Option<&str>,
) -> Result<(), GatewayError> {
    let started_at = std::time::Instant::now();
    let result = validate_execution_plan_pricing_for_unavailable_estimate_inner(
        state,
        plan,
        model_id,
        global_model_name,
        requested_processing_tier,
    )
    .await;
    observe_gateway_stage_ms(
        "auth_capacity_pricing_validation",
        started_at.elapsed().as_millis() as u64,
    );
    result
}

async fn validate_execution_plan_pricing_for_unavailable_estimate_inner(
    state: &AppState,
    plan: &aether_contracts::ExecutionPlan,
    model_id: Option<&str>,
    global_model_name: Option<&str>,
    requested_processing_tier: Option<&str>,
) -> Result<(), GatewayError> {
    if model_id.is_none() && global_model_name.is_none() {
        return Ok(());
    }

    let capacity_ttl = state.frontdoor_runtime_guards.auth_capacity_cache_ttl;
    if capacity_ttl.is_zero() {
        return validate_execution_plan_pricing_uncached(
            state,
            plan,
            model_id,
            global_model_name,
            requested_processing_tier,
        )
        .await;
    }
    let ttl = capacity_ttl.max(AUTH_PRICING_VALIDATION_CACHE_TTL);

    let cache_key = execution_plan_pricing_validation_cache_key(
        plan,
        model_id,
        global_model_name,
        requested_processing_tier,
    );
    let cache = state.auth_request_cost_upper_bound_cache.clone();
    cache
        .get_or_load_once_stale_while_revalidating(
            cache_key,
            ttl,
            AUTH_CAPACITY_CACHE_STALE_TTL,
            || async {
                validate_execution_plan_pricing_uncached(
                    state,
                    plan,
                    model_id,
                    global_model_name,
                    requested_processing_tier,
                )
                .await?;
                Ok::<Option<f64>, GatewayError>(Some(0.0))
            },
            || {
                let state = state.clone();
                let plan = plan.clone();
                let model_id = model_id.map(ToOwned::to_owned);
                let global_model_name = global_model_name.map(ToOwned::to_owned);
                let requested_processing_tier = requested_processing_tier.map(ToOwned::to_owned);
                async move {
                    validate_execution_plan_pricing_uncached(
                        &state,
                        &plan,
                        model_id.as_deref(),
                        global_model_name.as_deref(),
                        requested_processing_tier.as_deref(),
                    )
                    .await?;
                    Ok::<Option<f64>, GatewayError>(Some(0.0))
                }
            },
            crate::cache::CacheLoadObserver::default(),
        )
        .await?;
    Ok(())
}

async fn validate_execution_plan_pricing_uncached(
    state: &AppState,
    plan: &aether_contracts::ExecutionPlan,
    model_id: Option<&str>,
    global_model_name: Option<&str>,
    requested_processing_tier: Option<&str>,
) -> Result<(), GatewayError> {
    let _permit = state.acquire_auth_snapshot_load_gate().await?;
    let Some(context) =
        load_execution_plan_billing_context(state, plan, model_id, global_model_name).await?
    else {
        return Ok(());
    };
    aether_billing::BillingModelPricingSnapshot::from(context)
        .validate_authorization_pricing_configuration(requested_processing_tier)
        .map_err(|err| GatewayError::Internal(err.to_string()))
}

async fn load_execution_plan_billing_context(
    state: &AppState,
    plan: &aether_contracts::ExecutionPlan,
    model_id: Option<&str>,
    global_model_name: Option<&str>,
) -> Result<
    Option<aether_data_contracts::repository::billing::StoredBillingModelContext>,
    GatewayError,
> {
    let context = match model_id {
        Some(model_id) => state
            .data
            .find_billing_model_context_by_model_id(&plan.provider_id, Some(&plan.key_id), model_id)
            .await
            .map_err(|err| GatewayError::Internal(err.to_string()))?,
        None => state
            .data
            .find_billing_model_context(
                &plan.provider_id,
                Some(&plan.key_id),
                global_model_name.expect("global model name should exist"),
            )
            .await
            .map_err(|err| GatewayError::Internal(err.to_string()))?,
    };
    Ok(context)
}

fn execution_plan_cost_upper_bound_cache_key(
    plan: &aether_contracts::ExecutionPlan,
    model_id: Option<&str>,
    global_model_name: Option<&str>,
    api_format: &str,
    input_tokens: i64,
    max_output_tokens: Option<i64>,
    output_cap_requested: bool,
    requested_processing_tier: Option<&str>,
    cache_ttl_minutes: Option<i64>,
) -> String {
    format!(
        "{}\x1f{}\x1f{}\x1f{}\x1f{}\x1f{}\x1f{}\x1f{}\x1f{}",
        plan.provider_id,
        plan.key_id,
        model_id.unwrap_or(""),
        global_model_name.unwrap_or(""),
        api_format,
        input_tokens,
        match (max_output_tokens, output_cap_requested) {
            (Some(value), _) => value.to_string(),
            (None, true) => "unrepresentable".to_string(),
            (None, false) => "none".to_string(),
        },
        requested_processing_tier.unwrap_or("standard"),
        cache_ttl_minutes
            .map(|value| value.to_string())
            .unwrap_or_else(|| "none".to_string()),
    )
}

fn execution_plan_pricing_validation_cache_key(
    plan: &aether_contracts::ExecutionPlan,
    model_id: Option<&str>,
    global_model_name: Option<&str>,
    requested_processing_tier: Option<&str>,
) -> String {
    format!(
        "pricing-validation\x1f{}\x1f{}\x1f{}\x1f{}\x1f{}",
        plan.provider_id,
        plan.key_id,
        model_id.unwrap_or(""),
        global_model_name.unwrap_or(""),
        requested_processing_tier.unwrap_or("standard"),
    )
}

fn authorization_task_type<'a>(
    api_format: &str,
    report_context: Option<&'a serde_json::Value>,
) -> Option<&'a str> {
    if report_context
        .and_then(|context| context.get("image_request"))
        .is_some()
        || api_format == "openai:image"
    {
        return None;
    }
    if api_format.ends_with(":embedding") {
        return Some("embedding");
    }
    if api_format.ends_with(":rerank") {
        return Some("rerank");
    }
    Some("chat")
}

fn report_context_string_field<'a>(
    report_context: Option<&'a serde_json::Value>,
    key: &str,
) -> Option<&'a str> {
    report_context
        .and_then(|context| context.get(key))
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
}

fn max_output_tokens_from_request(value: &serde_json::Value) -> Option<u64> {
    ["max_tokens", "max_completion_tokens", "max_output_tokens"]
        .iter()
        .filter_map(|field| value.get(*field).and_then(serde_json::Value::as_u64))
        .chain(
            ["generationConfig", "generation_config"]
                .iter()
                .filter_map(|field| value.get(*field))
                .flat_map(|config| {
                    ["maxOutputTokens", "max_output_tokens"]
                        .iter()
                        .filter_map(move |field| {
                            config.get(*field).and_then(serde_json::Value::as_u64)
                        })
                }),
        )
        .filter(|value| *value > 0)
        .max()
}

fn output_choice_count_upper_bound(api_format: &str, value: &serde_json::Value) -> u64 {
    match api_format {
        "openai:chat" => value
            .get("n")
            .and_then(serde_json::Value::as_u64)
            .filter(|value| *value > 0)
            .unwrap_or(1),
        "gemini:generate_content" => ["generationConfig", "generation_config"]
            .iter()
            .filter_map(|field| value.get(*field))
            .flat_map(|config| {
                ["candidateCount", "candidate_count"]
                    .iter()
                    .filter_map(move |field| config.get(*field).and_then(serde_json::Value::as_u64))
            })
            .filter(|value| *value > 0)
            .max()
            .unwrap_or(1),
        _ => 1,
    }
}

fn request_input_is_self_contained(api_format: &str, value: &serde_json::Value) -> bool {
    match api_format {
        "claude:messages" => claude_request_input_is_self_contained(value),
        "gemini:generate_content" => gemini_request_input_is_self_contained(value),
        _ => openai_request_input_is_self_contained(api_format, value),
    }
}

fn only_fields(object: &serde_json::Map<String, serde_json::Value>, fields: &[&str]) -> bool {
    object.keys().all(|key| fields.contains(&key.as_str()))
}

fn nonempty_string(value: Option<&serde_json::Value>) -> Option<&str> {
    value
        .and_then(serde_json::Value::as_str)
        .filter(|value| !value.trim().is_empty())
}

fn claude_cache_control_is_bounded(value: Option<&serde_json::Value>) -> bool {
    let Some(value) = value else { return true };
    value.as_object().is_some_and(|object| {
        only_fields(object, &["type", "ttl"])
            && object.get("type").and_then(serde_json::Value::as_str) == Some("ephemeral")
            && object
                .get("ttl")
                .is_none_or(|ttl| matches!(ttl.as_str(), Some("5m" | "1h")))
    })
}

fn claude_text_content_is_self_contained(value: &serde_json::Value) -> bool {
    value.is_string()
        || value
            .as_array()
            .is_some_and(|blocks| blocks.iter().all(claude_text_block_is_self_contained))
}

fn claude_text_block_is_self_contained(value: &serde_json::Value) -> bool {
    value.as_object().is_some_and(|object| {
        only_fields(object, &["type", "text", "cache_control"])
            && object.get("type").and_then(serde_json::Value::as_str) == Some("text")
            && object.get("text").is_some_and(serde_json::Value::is_string)
            && claude_cache_control_is_bounded(object.get("cache_control"))
    })
}

/// Extended thinking is billed as output and output is capped by `max_tokens`, so every
/// documented mode keeps the request bounded; unknown shapes still fail closed.
fn claude_thinking_config_is_bounded(value: &serde_json::Value) -> bool {
    let Some(thinking) = value.as_object() else {
        return false;
    };
    if !only_fields(thinking, &["type", "budget_tokens", "display"])
        || thinking.get("display").is_some_and(|v| !v.is_string())
    {
        return false;
    }
    match thinking.get("type").and_then(serde_json::Value::as_str) {
        Some("disabled" | "adaptive") => !thinking.contains_key("budget_tokens"),
        Some("enabled") => thinking
            .get("budget_tokens")
            .is_none_or(serde_json::Value::is_u64),
        _ => false,
    }
}

/// Text, function calls and inline data only: every part is literal and already inside the byte
/// bound. Hosted tools (search, URL context, code execution) and file references fail closed.
fn gemini_request_input_is_self_contained(value: &serde_json::Value) -> bool {
    let Some(body) = value.as_object() else {
        return false;
    };
    if !only_fields(
        body,
        &[
            "model",
            "contents",
            "systemInstruction",
            "system_instruction",
            "tools",
            "toolConfig",
            "tool_config",
            "generationConfig",
            "generation_config",
            "safetySettings",
            "safety_settings",
            "stream",
        ],
    ) {
        return false;
    }
    let parts_ok = |parts: &serde_json::Value| {
        parts.as_array().is_some_and(|parts| {
            parts.iter().all(|part| {
                part.as_object().is_some_and(|part| {
                    only_fields(
                        part,
                        &[
                            "text",
                            "thought",
                            "thoughtSignature",
                            "thought_signature",
                            "functionCall",
                            "function_call",
                            "functionResponse",
                            "function_response",
                            "inlineData",
                            "inline_data",
                            "fileData",
                            "file_data",
                            "executableCode",
                            "executable_code",
                            "codeExecutionResult",
                            "code_execution_result",
                        ],
                    )
                })
            })
        })
    };
    let contents_ok = body
        .get("contents")
        .and_then(serde_json::Value::as_array)
        .is_some_and(|contents| {
            !contents.is_empty()
                && contents.iter().all(|content| {
                    content.get("parts").is_some_and(parts_ok)
                        && content
                            .as_object()
                            .is_some_and(|o| only_fields(o, &["role", "parts"]))
                })
        });
    let system_ok = ["systemInstruction", "system_instruction"]
        .iter()
        .filter_map(|key| body.get(*key))
        .all(|system| system.get("parts").is_some_and(parts_ok));
    let tools_ok = body.get("tools").is_none_or(|tools| {
        tools.as_array().is_some_and(|tools| {
            tools.iter().all(|tool| {
                tool.as_object().is_some_and(|tool| {
                    // Hosted tools are reserved as a fixed surcharge (`hosted_tools_policy_cost_usd`).
                    !tool.is_empty()
                })
            })
        })
    });
    contents_ok && system_ok && tools_ok
}

fn claude_request_input_is_self_contained(value: &serde_json::Value) -> bool {
    let Some(body) = value.as_object() else {
        return false;
    };
    // Opt in only to local text/function semantics; opaque state, server tools and
    // future extensions must not turn a hard cost limit into an unbounded request.
    if !only_fields(
        body,
        &[
            "model",
            "messages",
            "system",
            "max_tokens",
            "stream",
            "temperature",
            "top_p",
            "top_k",
            "stop_sequences",
            "tools",
            "tool_choice",
            "metadata",
            "service_tier",
            "cache_control",
            "thinking",
            "output_config",
            "context_management",
            "mcp_servers",
            "container",
        ],
    ) || nonempty_string(body.get("model")).is_none()
        || body
            .get("max_tokens")
            .and_then(serde_json::Value::as_u64)
            .is_none_or(|n| n == 0)
        || !claude_cache_control_is_bounded(body.get("cache_control"))
        || body
            .get("thinking")
            .is_some_and(|v| !claude_thinking_config_is_bounded(v))
        || body.get("output_config").is_some_and(|v| {
            !v.as_object().is_some_and(|config| {
                only_fields(config, &["effort"])
                    && config
                        .get("effort")
                        .is_none_or(serde_json::Value::is_string)
            })
        })
        || body.get("context_management").is_some_and(|v| {
            // Edits only ever remove history, so they cannot raise the upper bound.
            !v.as_object().is_some_and(|config| {
                only_fields(config, &["edits"])
                    && config
                        .get("edits")
                        .and_then(serde_json::Value::as_array)
                        .is_some_and(|edits| {
                            edits.iter().all(|edit| {
                                edit.as_object()
                                    .is_some_and(|edit| nonempty_string(edit.get("type")).is_some())
                            })
                        })
            })
        })
        || body
            .get("system")
            .is_some_and(|v| !claude_text_content_is_self_contained(v))
        || body.get("metadata").is_some_and(|v| {
            !v.as_object().is_some_and(|metadata| {
                only_fields(metadata, &["user_id"])
                    && metadata.values().all(serde_json::Value::is_string)
            })
        })
    {
        return false;
    }
    if let Some(value) = body.get("tools") {
        let Some(definitions) = value.as_array() else {
            return false;
        };
        for definition in definitions {
            let Some(tool) = definition.as_object() else {
                return false;
            };
            if tool
                .get("type")
                .and_then(serde_json::Value::as_str)
                .is_some_and(|kind| kind != "custom")
            {
                // Server/hosted tools are reserved as a fixed surcharge
                // (see `hosted_tools_policy_cost_usd`); their config is literal input.
                if nonempty_string(tool.get("name")).is_none() {
                    return false;
                }
                continue;
            }
            if !only_fields(
                tool,
                &[
                    "name",
                    "description",
                    "input_schema",
                    "type",
                    "cache_control",
                    "strict",
                ],
            ) || nonempty_string(tool.get("name")).is_none()
                || tool
                    .get("type")
                    .is_some_and(|v| v.as_str() != Some("custom"))
                || !tool
                    .get("input_schema")
                    .is_some_and(serde_json::Value::is_object)
                || tool.get("description").is_some_and(|v| !v.is_string())
                || tool.get("strict").is_some_and(|v| !v.is_boolean())
                || !claude_cache_control_is_bounded(tool.get("cache_control"))
            {
                return false;
            }
        }
    }
    if let Some(value) = body.get("tool_choice") {
        let Some(choice) = value.as_object() else {
            return false;
        };
        let kind = choice.get("type").and_then(serde_json::Value::as_str);
        if !only_fields(choice, &["type", "name", "disable_parallel_tool_use"])
            || !matches!(kind, Some("auto" | "any" | "none" | "tool"))
            || choice
                .get("disable_parallel_tool_use")
                .is_some_and(|v| !v.is_boolean())
            || (kind == Some("tool") && nonempty_string(choice.get("name")).is_none())
            || (kind != Some("tool") && choice.contains_key("name"))
        {
            return false;
        }
    }
    let Some(messages) = body.get("messages").and_then(serde_json::Value::as_array) else {
        return false;
    };
    if messages.is_empty() {
        return false;
    }
    for value in messages {
        let Some(message) = value.as_object() else {
            return false;
        };
        let role = message.get("role").and_then(serde_json::Value::as_str);
        if !only_fields(message, &["role", "content"])
            || !matches!(role, Some("user" | "assistant" | "system"))
        {
            return false;
        }
        let Some(content) = message.get("content") else {
            return false;
        };
        if content.is_string() {
            continue;
        }
        let Some(blocks) = content.as_array() else {
            return false;
        };
        for value in blocks {
            let Some(block) = value.as_object() else {
                return false;
            };
            if !claude_cache_control_is_bounded(block.get("cache_control")) {
                return false;
            }
            match block.get("type").and_then(serde_json::Value::as_str) {
                Some("text") if claude_text_block_is_self_contained(value) => {}
                // Replayed reasoning is literal text, already inside the byte bound.
                Some("thinking") if role == Some("assistant") => {
                    if !only_fields(block, &["type", "thinking", "signature", "cache_control"])
                        || !block
                            .get("thinking")
                            .is_some_and(serde_json::Value::is_string)
                        || block.get("signature").is_some_and(|v| !v.is_string())
                    {
                        return false;
                    }
                }
                Some("redacted_thinking") if role == Some("assistant") => {
                    if !only_fields(block, &["type", "data", "cache_control"])
                        || !block.get("data").is_some_and(serde_json::Value::is_string)
                    {
                        return false;
                    }
                }
                Some("tool_use") if role == Some("assistant") => {
                    // Historical names/IDs and arbitrary argument objects are literal
                    // tokens, independent of the currently available tool definitions.
                    if !only_fields(block, &["type", "id", "name", "input", "cache_control"])
                        || nonempty_string(block.get("id")).is_none()
                        || nonempty_string(block.get("name")).is_none()
                        || !block.get("input").is_some_and(serde_json::Value::is_object)
                    {
                        return false;
                    }
                }
                // Media is reserved at a fixed per-part bound (see `media_part_policy_tokens`).
                Some("image") if role == Some("user") => {
                    if !only_fields(block, &["type", "source", "cache_control"])
                        || !claude_media_source_is_bounded(block.get("source"))
                    {
                        return false;
                    }
                }
                Some("document") if role == Some("user") => {
                    if !only_fields(
                        block,
                        &[
                            "type",
                            "source",
                            "title",
                            "context",
                            "citations",
                            "cache_control",
                        ],
                    ) || !claude_media_source_is_bounded(block.get("source"))
                    {
                        return false;
                    }
                }
                // Replayed hosted tool history is literal content.
                Some(kind)
                    if role == Some("assistant")
                        && (matches!(kind, "server_tool_use" | "mcp_tool_use")
                            || kind.ends_with("_tool_result")) => {}
                Some("tool_result") if role == Some("user") => {
                    if !only_fields(
                        block,
                        &[
                            "type",
                            "tool_use_id",
                            "content",
                            "is_error",
                            "cache_control",
                        ],
                    ) || nonempty_string(block.get("tool_use_id")).is_none()
                        || block
                            .get("content")
                            .is_some_and(|v| !claude_text_content_is_self_contained(v))
                        || block.get("is_error").is_some_and(|v| !v.is_boolean())
                    {
                        return false;
                    }
                }
                _ => return false,
            }
        }
    }
    true
}

fn claude_media_source_is_bounded(source: Option<&serde_json::Value>) -> bool {
    let Some(source) = source.and_then(serde_json::Value::as_object) else {
        return false;
    };
    match source.get("type").and_then(serde_json::Value::as_str) {
        Some("base64") => {
            only_fields(source, &["type", "media_type", "data"])
                && source.get("data").is_some_and(serde_json::Value::is_string)
        }
        Some("url") => {
            only_fields(source, &["type", "url"]) && nonempty_string(source.get("url")).is_some()
        }
        Some("text") => {
            only_fields(source, &["type", "media_type", "data"])
                && source.get("data").is_some_and(serde_json::Value::is_string)
        }
        Some("file") => {
            only_fields(source, &["type", "file_id"])
                && nonempty_string(source.get("file_id")).is_some()
        }
        _ => false,
    }
}

fn openai_request_input_is_self_contained(api_format: &str, value: &serde_json::Value) -> bool {
    if !api_format.starts_with("openai:") {
        return false;
    }
    let Some(object) = value.as_object() else {
        return true;
    };
    if ["previous_response_id", "conversation"]
        .iter()
        .any(|key| object.get(*key).is_some_and(has_reference_value))
    {
        return false;
    }
    if object
        .get("prompt")
        .and_then(serde_json::Value::as_object)
        .and_then(|prompt| prompt.get("id"))
        .is_some_and(has_reference_value)
    {
        return false;
    }
    !contains_indirect_request_input(value)
}

fn contains_indirect_request_input(value: &serde_json::Value) -> bool {
    match value {
        serde_json::Value::Array(items) => items.iter().any(contains_indirect_request_input),
        serde_json::Value::Object(object) => {
            let item_type = object
                .get("type")
                .and_then(serde_json::Value::as_str)
                .map(|value| value.trim().to_ascii_lowercase());
            if item_type
                .as_deref()
                .is_some_and(|item_type| matches!(item_type, "url" | "item_reference"))
            {
                return true;
            }
            // Media parts and stored-file references (`file_id`, `file_uri`) are reserved at a
            // fixed per-part bound; hosted tools (incl. `vector_store_ids`) at a fixed surcharge.
            if media_part_kind(object).is_some() {
                return false;
            }
            object.values().any(contains_indirect_request_input)
        }
        _ => false,
    }
}

fn has_reference_value(value: &serde_json::Value) -> bool {
    match value {
        serde_json::Value::Null => false,
        serde_json::Value::String(value) => !value.trim().is_empty(),
        serde_json::Value::Array(values) => !values.is_empty(),
        serde_json::Value::Object(values) => !values.is_empty(),
        _ => true,
    }
}

fn json_input_token_estimate(value: &serde_json::Value) -> u64 {
    // This is an admission estimate, not a tokenizer or a billing measurement.
    // ASCII prose/code averages several bytes per token. Reserve one token per
    // four ASCII bytes and one per two UTF-8 bytes for multilingual content.
    // Counting every JSON byte as a token spuriously crossed long-context tiers.
    // Actual upstream usage continues to determine settlement.
    serde_json::to_vec(value)
        .map(|bytes| {
            let ascii = bytes.iter().filter(|byte| byte.is_ascii()).count() as u64;
            let non_ascii = bytes.len() as u64 - ascii;
            ascii.div_ceil(4).saturating_add(non_ascii.div_ceil(2))
        })
        .unwrap_or(u64::MAX)
}

fn request_input_token_count_policy_bound(api_format: &str, value: &serde_json::Value) -> u64 {
    request_input_token_count_policy_bound_with(api_format, value, media_part_policy_tokens)
}

fn request_input_token_count_policy_bound_with(
    api_format: &str,
    value: &serde_json::Value,
    media_tokens: impl Fn(MediaPartKind) -> u64,
) -> u64 {
    // Media payloads (base64 data URIs, URLs) are replaced by a fixed per-part bound instead
    // of being counted byte-for-byte, so a single image neither costs millions of tokens nor
    // is dropped from the estimate.
    let mut media_estimate = 0_u64;
    let mut media_reserved = 0_u64;
    let mut pending = vec![value];
    while let Some(value) = pending.pop() {
        match value {
            serde_json::Value::Object(object) => {
                if let Some(kind) = media_part_kind(object) {
                    media_estimate =
                        media_estimate.saturating_add(json_input_token_estimate(value));
                    media_reserved = media_reserved.saturating_add(media_tokens(kind));
                    continue;
                }
                for (key, child) in object {
                    if matches!(
                        key.as_str(),
                        "inlineData" | "inline_data" | "fileData" | "file_data"
                    ) {
                        if let Some(inline) = child.as_object() {
                            media_estimate =
                                media_estimate.saturating_add(json_input_token_estimate(child));
                            media_reserved = media_reserved
                                .saturating_add(media_tokens(gemini_inline_media_kind(inline)));
                            continue;
                        }
                    }
                    pending.push(child);
                }
            }
            serde_json::Value::Array(items) => pending.extend(items),
            _ => {}
        }
    }
    let tokens = json_input_token_estimate(value)
        .saturating_sub(media_estimate)
        .saturating_add(media_reserved);
    if api_format != "claude:messages" {
        return tokens;
    }
    let mut nodes = 0_u64;
    let mut pending = vec![value];
    while let Some(value) = pending.pop() {
        nodes = nodes.saturating_add(1);
        match value {
            serde_json::Value::Object(object) => pending.extend(object.values()),
            serde_json::Value::Array(items) => pending.extend(items),
            _ => {}
        }
    }
    let tools = value
        .get("tools")
        .and_then(serde_json::Value::as_array)
        .map_or(0, |tools| u64::try_from(tools.len()).unwrap_or(u64::MAX));
    claude_framed_input_policy_bound(tokens, nodes, tools)
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum MediaPartKind {
    Image,
    Audio,
    Video,
    File,
}

/// OpenAI Chat/Responses and Claude media parts. Gemini `inlineData` is handled by the caller.
fn media_part_kind(object: &serde_json::Map<String, serde_json::Value>) -> Option<MediaPartKind> {
    match object.get("type").and_then(serde_json::Value::as_str)? {
        "image_url" | "input_image" | "image" => Some(MediaPartKind::Image),
        "input_audio" => Some(MediaPartKind::Audio),
        "input_file" | "file" | "document" => Some(MediaPartKind::File),
        _ => None,
    }
}

fn gemini_inline_media_kind(object: &serde_json::Map<String, serde_json::Value>) -> MediaPartKind {
    let mime = ["mimeType", "mime_type"]
        .iter()
        .find_map(|key| object.get(*key).and_then(serde_json::Value::as_str))
        .unwrap_or_default()
        .trim()
        .to_ascii_lowercase();
    if mime.starts_with("image/") {
        MediaPartKind::Image
    } else if mime.starts_with("audio/") {
        MediaPartKind::Audio
    } else if mime.starts_with("video/") {
        MediaPartKind::Video
    } else {
        MediaPartKind::File
    }
}

// Per-part input token reservations, modelled on new-api's pre-consume
// (service/token_counter.go CountRequestToken, tokenkit/image.go ImageTokens).
// Images: new-api's largest fixed per-image results are Gemini's 12+1 tiles (3354) and
// Claude's 1568px tier (3140); 4096 covers both and the classic OpenAI 85+170/tile rule.
// Audio 256, video 8192 and file 4096 are new-api's fixed per-file values.
const USAGE_POLICY_MEDIA_TOKEN_DEFAULTS: [(&str, MediaPartKind, u64); 4] = [
    (
        "AETHER_GATEWAY_USAGE_POLICY_IMAGE_TOKENS",
        MediaPartKind::Image,
        4096,
    ),
    (
        "AETHER_GATEWAY_USAGE_POLICY_AUDIO_TOKENS",
        MediaPartKind::Audio,
        256,
    ),
    (
        "AETHER_GATEWAY_USAGE_POLICY_VIDEO_TOKENS",
        MediaPartKind::Video,
        8192,
    ),
    (
        "AETHER_GATEWAY_USAGE_POLICY_FILE_TOKENS",
        MediaPartKind::File,
        4096,
    ),
];

fn media_part_policy_tokens(kind: MediaPartKind) -> u64 {
    static VALUES: std::sync::OnceLock<[u64; 4]> = std::sync::OnceLock::new();
    let values = VALUES.get_or_init(|| {
        USAGE_POLICY_MEDIA_TOKEN_DEFAULTS.map(|(env, _, default)| {
            parse_usage_policy_media_tokens(std::env::var(env).ok().as_deref(), default)
        })
    });
    USAGE_POLICY_MEDIA_TOKEN_DEFAULTS
        .iter()
        .position(|(_, k, _)| *k == kind)
        .map_or(0, |index| values[index])
}

fn parse_usage_policy_media_tokens(raw: Option<&str>, default: u64) -> u64 {
    raw.and_then(|value| value.trim().parse::<u64>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(default)
}

fn claude_framed_input_policy_bound(bytes: u64, nodes: u64, tools: u64) -> u64 {
    // Claude tool definitions generate an additional system prompt. The published
    // overhead table includes 804 tokens, beyond the literal names/schema/history:
    // https://platform.claude.com/docs/en/about-claude/pricing
    // https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview
    // Reserve policy margins, not a claimed exact vendor count: 4096 framing,
    // 1024 per tool and 128 per JSON node (including messages, blocks and schemas).
    // This assumes a protocol-conforming provider, not an unobservable reseller
    // injecting arbitrary context. Unknown/nonlocal semantics fail closed above.
    bytes
        .saturating_add(4096)
        .saturating_add(tools.saturating_mul(1024))
        .saturating_add(nodes.saturating_mul(128))
}

fn model_directive_base_model_is_allowed_for_request(
    decision: &GatewayControlDecision,
    requested_model: &str,
    allowed_models: &[String],
) -> bool {
    let Some(client_api_format) = decision
        .auth_endpoint_signature
        .as_deref()
        .map(crate::ai_serving::normalize_api_format_alias)
        .filter(|value| !value.trim().is_empty())
    else {
        return false;
    };
    for api_format in candidate_api_formats_for_model_resolution(&client_api_format) {
        let resolution = decision
            .model_directive_policy
            .resolve_reasoning(&api_format, Some(requested_model));
        if resolution
            .base_model()
            .is_some_and(|base_model| contains_string(allowed_models, base_model))
        {
            return true;
        }
    }
    false
}

async fn request_model_resolves_to_allowed_model(
    state: &AppState,
    decision: &GatewayControlDecision,
    requested_model: &str,
    allowed_models: &[String],
) -> Result<bool, GatewayError> {
    let Some(client_api_format) = decision
        .auth_endpoint_signature
        .as_deref()
        .map(crate::ai_serving::normalize_api_format_alias)
        .filter(|value| !value.trim().is_empty())
    else {
        return Ok(false);
    };

    for api_format in candidate_api_formats_for_model_resolution(&client_api_format) {
        let resolution = decision
            .model_directive_policy
            .resolve_reasoning(&api_format, Some(requested_model));
        let routing_model = resolution.base_model().unwrap_or(requested_model);
        let rows = {
            // Model alias authorization runs before candidate planning, so its database read must
            // participate in the same foreground DB admission budget as the rest of auth. Keep
            // the permit scoped to this one read; callers do not hold this gate, and releasing it
            // here avoids carrying a DB permit through pure filtering or subsequent formats.
            let _permit = state.acquire_auth_snapshot_load_gate().await?;
            state
                .list_minimal_candidate_selection_rows_for_api_format(&api_format)
                .await?
        };
        let matching_rows = rows
            .into_iter()
            .filter(|row| {
                aether_scheduler_core::row_supports_requested_model_with_model_directives(
                    row,
                    routing_model,
                    &api_format,
                    false,
                )
            })
            .collect::<Vec<_>>();
        let Some(resolved_global_model) =
            aether_scheduler_core::resolve_requested_global_model_name_with_model_directives(
                &matching_rows,
                routing_model,
                &api_format,
                false,
            )
        else {
            continue;
        };
        if contains_string(allowed_models, &resolved_global_model) {
            return Ok(true);
        }
    }

    Ok(false)
}

fn candidate_api_formats_for_model_resolution(client_api_format: &str) -> Vec<String> {
    let mut api_formats = Vec::new();
    push_unique_api_format(&mut api_formats, client_api_format);
    for api_format in crate::ai_serving::request_candidate_api_formats(client_api_format, false) {
        push_unique_api_format(&mut api_formats, api_format);
    }
    api_formats
}

fn push_unique_api_format(api_formats: &mut Vec<String>, api_format: &str) {
    let api_format = crate::ai_serving::normalize_api_format_alias(api_format);
    if api_format.is_empty() || api_formats.iter().any(|value| value == &api_format) {
        return;
    }
    api_formats.push(api_format);
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::Arc;
    use std::time::Duration;

    use aether_contracts::{ExecutionPlan, RequestBody};
    use aether_data::repository::candidate_selection::InMemoryMinimalCandidateSelectionReadRepository;
    use aether_data::repository::wallet::StoredWalletSnapshot;
    use aether_data_contracts::repository::billing::{
        BillingReadRepository, StoredBillingModelContext, UserDailyQuotaAvailabilityRecord,
        UserPlanEntitlementRecord,
    };
    use aether_data_contracts::repository::candidate_selection::{
        StoredMinimalCandidateSelectionRow, StoredProviderModelMapping,
    };
    use aether_data_contracts::DataLayerError;
    use aether_runtime::ConcurrencyGate;
    use async_trait::async_trait;
    use axum::body::Bytes;
    use axum::http::{HeaderMap, Uri};
    use serde_json::json;

    use super::{
        available_balance_capacity_usd, default_reserved_output_tokens,
        execution_plan_balance_capacity_rejection, execution_plan_cost_upper_bound_cache_key,
        max_output_tokens_from_request, openai_request_input_is_self_contained,
        output_choice_count_upper_bound, parse_usage_policy_default_output_tokens,
        request_input_is_self_contained, request_model_local_rejection, GatewayLocalAuthRejection,
    };
    use crate::control::{GatewayControlAuthContext, GatewayControlDecision};
    use crate::data::GatewayDataState;
    use crate::AppState;

    fn sample_row() -> StoredMinimalCandidateSelectionRow {
        StoredMinimalCandidateSelectionRow {
            provider_id: "provider-1".to_string(),
            provider_name: "Provider 1".to_string(),
            provider_type: "openai".to_string(),
            provider_priority: 0,
            provider_is_active: true,
            endpoint_id: "endpoint-1".to_string(),
            endpoint_api_format: "openai:chat".to_string(),
            endpoint_api_family: Some("openai".to_string()),
            endpoint_kind: Some("chat".to_string()),
            endpoint_is_active: true,
            key_id: "key-1".to_string(),
            key_name: "key".to_string(),
            key_auth_type: "api_key".to_string(),
            key_is_active: true,
            key_api_formats: Some(vec!["openai:chat".to_string()]),
            key_allowed_models: None,
            key_capabilities: None,
            key_internal_priority: 0,
            key_global_priority_by_format: None,
            model_id: "model-1".to_string(),
            global_model_id: "global-model-1".to_string(),
            global_model_name: "gpt-5".to_string(),
            global_model_mappings: Some(vec!["gpt-5(?:\\.\\d+)?".to_string()]),
            global_model_supports_streaming: Some(true),
            model_provider_model_name: "gpt-5-upstream".to_string(),
            model_provider_model_mappings: Some(vec![StoredProviderModelMapping {
                name: "gpt-5-upstream".to_string(),
                priority: 1,
                api_formats: Some(vec!["openai:chat".to_string()]),
                endpoint_ids: None,
                operations: None,
            }]),
            model_supports_streaming: Some(true),
            model_is_active: true,
            model_is_available: true,
        }
    }

    fn sample_row_for_api_format(api_format: &str) -> StoredMinimalCandidateSelectionRow {
        let mut row = sample_row();
        let api_family = api_format
            .split_once(':')
            .map(|(family, _)| family)
            .unwrap_or(api_format);
        row.provider_id = format!("provider-{api_family}");
        row.provider_name = format!("Provider {api_family}");
        row.provider_type = api_family.to_string();
        row.endpoint_id = format!("endpoint-{api_family}");
        row.endpoint_api_format = api_format.to_string();
        row.endpoint_api_family = Some(api_family.to_string());
        row.key_id = format!("key-{api_family}");
        row.key_api_formats = Some(vec![api_format.to_string()]);
        if let Some(mappings) = row.model_provider_model_mappings.as_mut() {
            for mapping in mappings {
                mapping.api_formats = Some(vec![api_format.to_string()]);
            }
        }
        row
    }

    fn decision_with_allowed_models(allowed_models: Vec<String>) -> GatewayControlDecision {
        let mut decision = GatewayControlDecision::synthetic(
            "/v1/chat/completions",
            Some("ai_public".to_string()),
            Some("openai".to_string()),
            Some("chat".to_string()),
            Some("openai:chat".to_string()),
        );
        decision.auth_context = Some(GatewayControlAuthContext {
            user_id: "user-1".to_string(),
            api_key_id: "api-key-1".to_string(),
            username: None,
            api_key_name: None,
            balance_remaining: None,
            access_allowed: true,
            user_rate_limit: None,
            api_key_rate_limit: None,
            api_key_is_standalone: false,
            admin_bypass_limits: false,
            local_rejection: None,
            allowed_models: Some(allowed_models),
            ip_rules: None,
            verified_api_key_hash: None,
        });
        decision
    }

    fn state_with_rows(rows: Vec<StoredMinimalCandidateSelectionRow>) -> AppState {
        let repository = Arc::new(InMemoryMinimalCandidateSelectionReadRepository::seed(rows));
        let data = GatewayDataState::with_minimal_candidate_selection_reader_for_tests(repository);
        AppState::new()
            .expect("state should build")
            .with_data_state_for_tests(data)
    }

    fn state_with_quota_and_wallet(
        quota: UserDailyQuotaAvailabilityRecord,
        context: StoredBillingModelContext,
    ) -> AppState {
        state_with_quota_context_and_wallet(quota, context, sample_wallet("user-1", 30.0))
    }

    fn state_with_quota_context_and_wallet(
        quota: UserDailyQuotaAvailabilityRecord,
        context: StoredBillingModelContext,
        wallet: StoredWalletSnapshot,
    ) -> AppState {
        let candidate_repository =
            Arc::new(InMemoryMinimalCandidateSelectionReadRepository::seed(vec![
                sample_row(),
            ]));
        let billing_repository = Arc::new(FixedBillingReadRepository::new(quota, context));
        let data = GatewayDataState::with_minimal_candidate_selection_and_billing_for_tests(
            candidate_repository,
            billing_repository,
        );
        AppState::new()
            .expect("state should build")
            .with_data_state_for_tests(data)
            .with_auth_wallets_for_tests(vec![wallet])
    }

    fn state_with_model_mapping() -> AppState {
        state_with_rows(vec![sample_row()])
    }

    fn execution_plan(body: serde_json::Value, api_format: &str) -> ExecutionPlan {
        ExecutionPlan {
            request_id: "request-1".to_string(),
            candidate_id: Some("candidate-1".to_string()),
            provider_name: Some("OpenAI".to_string()),
            provider_id: "provider-1".to_string(),
            endpoint_id: "endpoint-1".to_string(),
            key_id: "key-1".to_string(),
            method: "POST".to_string(),
            url: "https://api.openai.com/v1/responses".to_string(),
            headers: BTreeMap::new(),
            content_type: Some("application/json".to_string()),
            content_encoding: None,
            body: RequestBody::from_json(body),
            stream: false,
            client_api_format: api_format.to_string(),
            provider_api_format: api_format.to_string(),
            model_name: Some("gpt-5".to_string()),
            proxy: None,
            transport_profile: None,
            timeouts: None,
        }
    }

    fn billing_report_context() -> serde_json::Value {
        json!({
            "model_id": "model-1",
            "global_model_name": "gpt-5"
        })
    }

    fn estimate_from_billing_context(
        context: &StoredBillingModelContext,
        api_format: &str,
        input_tokens: i64,
        max_output_tokens: Option<i64>,
    ) -> Option<f64> {
        let mut estimate =
            aether_billing::BillingAuthorizationEstimateInput::new("chat", input_tokens);
        estimate.api_format = Some(api_format.to_string());
        estimate.max_output_tokens = max_output_tokens;
        aether_billing::BillingService::new()
            .estimate_authorization_cost_upper_bound(
                &aether_billing::BillingModelPricingSnapshot::from(context),
                &estimate,
            )
            .expect("estimate should calculate")
    }

    fn json_headers() -> HeaderMap {
        let mut headers = HeaderMap::new();
        headers.insert(
            axum::http::header::CONTENT_TYPE,
            "application/json"
                .parse()
                .expect("content type should parse"),
        );
        headers
    }

    fn billing_context_with_pricing(
        default_tiered_pricing: Option<serde_json::Value>,
        model_tiered_pricing: Option<serde_json::Value>,
        rate_multipliers: Option<serde_json::Value>,
        billing_type: Option<&str>,
    ) -> StoredBillingModelContext {
        StoredBillingModelContext::new(
            "provider-1".to_string(),
            billing_type.map(ToOwned::to_owned),
            Some("key-1".to_string()),
            rate_multipliers,
            Some(60),
            "global-model-1".to_string(),
            "gpt-5".to_string(),
            None,
            None,
            default_tiered_pricing,
            Some("model-1".to_string()),
            Some("gpt-5-upstream".to_string()),
            None,
            None,
            model_tiered_pricing,
        )
        .expect("billing context should build")
    }

    fn sample_wallet(user_id: &str, balance: f64) -> StoredWalletSnapshot {
        StoredWalletSnapshot::new(
            format!("wallet-{user_id}"),
            Some(user_id.to_string()),
            None,
            balance,
            0.0,
            "finite".to_string(),
            "USD".to_string(),
            "active".to_string(),
            balance,
            0.0,
            0.0,
            0.0,
            100,
        )
        .expect("wallet should build")
    }

    fn quota_availability(
        remaining_usd: f64,
        allow_wallet_overage: bool,
    ) -> UserDailyQuotaAvailabilityRecord {
        UserDailyQuotaAvailabilityRecord {
            has_active_daily_quota: true,
            total_quota_usd: remaining_usd,
            used_usd: 0.0,
            remaining_usd,
            allow_wallet_overage,
        }
    }

    #[derive(Debug)]
    struct FixedBillingReadRepository {
        quota: UserDailyQuotaAvailabilityRecord,
        context: StoredBillingModelContext,
        entitlements: Vec<UserPlanEntitlementRecord>,
        quota_calls: Arc<AtomicUsize>,
        model_context_by_model_id_calls: Arc<AtomicUsize>,
    }

    impl FixedBillingReadRepository {
        fn new(
            quota: UserDailyQuotaAvailabilityRecord,
            context: StoredBillingModelContext,
        ) -> Self {
            Self {
                quota,
                context,
                entitlements: Vec::new(),
                quota_calls: Arc::new(AtomicUsize::new(0)),
                model_context_by_model_id_calls: Arc::new(AtomicUsize::new(0)),
            }
        }

        fn with_counters(
            quota: UserDailyQuotaAvailabilityRecord,
            context: StoredBillingModelContext,
            quota_calls: Arc<AtomicUsize>,
            model_context_by_model_id_calls: Arc<AtomicUsize>,
        ) -> Self {
            Self {
                quota,
                context,
                entitlements: Vec::new(),
                quota_calls,
                model_context_by_model_id_calls,
            }
        }
    }

    #[async_trait]
    impl BillingReadRepository for FixedBillingReadRepository {
        async fn list_user_plan_entitlements(
            &self,
            user_id: &str,
        ) -> Result<Option<Vec<UserPlanEntitlementRecord>>, DataLayerError> {
            Ok(Some(
                self.entitlements
                    .iter()
                    .filter(|record| record.user_id == user_id)
                    .cloned()
                    .collect(),
            ))
        }

        async fn find_model_context(
            &self,
            _provider_id: &str,
            _provider_api_key_id: Option<&str>,
            _global_model_name: &str,
        ) -> Result<Option<StoredBillingModelContext>, DataLayerError> {
            Ok(Some(self.context.clone()))
        }

        async fn find_model_context_by_model_id(
            &self,
            _provider_id: &str,
            _provider_api_key_id: Option<&str>,
            _model_id: &str,
        ) -> Result<Option<StoredBillingModelContext>, DataLayerError> {
            self.model_context_by_model_id_calls
                .fetch_add(1, Ordering::AcqRel);
            Ok(Some(self.context.clone()))
        }

        async fn find_user_daily_quota_availability(
            &self,
            _user_id: &str,
        ) -> Result<Option<UserDailyQuotaAvailabilityRecord>, DataLayerError> {
            self.quota_calls.fetch_add(1, Ordering::AcqRel);
            Ok(Some(self.quota.clone()))
        }
    }

    #[tokio::test]
    async fn model_rejection_allows_requested_model_that_resolves_to_allowed_global_model() {
        let state = state_with_model_mapping();
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let uri: Uri = "/v1/chat/completions".parse().expect("uri should parse");
        let body = Bytes::from_static(br#"{"model":"gpt-5.2","messages":[]}"#);

        let rejection =
            request_model_local_rejection(&state, Some(&decision), &uri, &json_headers(), &body)
                .await
                .expect("model rejection should resolve");

        assert_eq!(rejection, None);
    }

    #[tokio::test]
    async fn model_alias_resolution_waits_for_auth_database_gate() {
        let mut state = state_with_model_mapping();
        state.auth_snapshot_load_gate = Some(Arc::new(ConcurrencyGate::new(
            "test_auth_model_resolution",
            1,
        )));
        let held = state
            .acquire_auth_snapshot_load_gate()
            .await
            .expect("auth gate acquisition should succeed")
            .expect("auth gate should be configured");
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let uri: Uri = "/v1/chat/completions".parse().expect("uri should parse");
        let headers = json_headers();
        let body = Bytes::from_static(br#"{"model":"gpt-5.2","messages":[]}"#);

        let blocked = tokio::time::timeout(
            Duration::from_millis(25),
            request_model_local_rejection(&state, Some(&decision), &uri, &headers, &body),
        )
        .await;
        assert!(
            blocked.is_err(),
            "model alias candidate reads must wait for the auth DB gate"
        );

        drop(held);
        let rejection = tokio::time::timeout(
            Duration::from_secs(1),
            request_model_local_rejection(&state, Some(&decision), &uri, &headers, &body),
        )
        .await
        .expect("model alias resolution should resume after releasing the auth gate")
        .expect("model rejection should resolve");
        assert_eq!(rejection, None);
    }

    #[tokio::test]
    async fn model_rejection_allows_cross_format_provider_mapping_to_allowed_global_model() {
        let mut row = sample_row_for_api_format("gemini:generate_content");
        row.model_provider_model_name = "gemini-2.5-pro-upstream".to_string();
        row.model_provider_model_mappings = Some(vec![StoredProviderModelMapping {
            name: "gemini-2.5-pro-alias".to_string(),
            priority: 1,
            api_formats: Some(vec!["gemini:generate_content".to_string()]),
            endpoint_ids: None,
            operations: None,
        }]);
        let state = state_with_rows(vec![row]);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let uri: Uri = "/v1/chat/completions".parse().expect("uri should parse");
        let body = Bytes::from_static(br#"{"model":"gemini-2.5-pro-alias","messages":[]}"#);

        let rejection =
            request_model_local_rejection(&state, Some(&decision), &uri, &json_headers(), &body)
                .await
                .expect("model rejection should resolve");

        assert_eq!(rejection, None);
    }

    #[tokio::test]
    async fn model_rejection_allows_cross_format_regex_mapping_to_allowed_global_model() {
        let state = state_with_rows(vec![sample_row_for_api_format("claude:messages")]);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let uri: Uri = "/v1/chat/completions".parse().expect("uri should parse");
        let body = Bytes::from_static(br#"{"model":"gpt-5.2","messages":[]}"#);

        let rejection =
            request_model_local_rejection(&state, Some(&decision), &uri, &json_headers(), &body)
                .await
                .expect("model rejection should resolve");

        assert_eq!(rejection, None);
    }

    #[tokio::test]
    async fn model_rejection_denies_requested_model_outside_allowed_global_models() {
        let state = state_with_model_mapping();
        let decision = decision_with_allowed_models(vec!["gpt-4.1".to_string()]);
        let uri: Uri = "/v1/chat/completions".parse().expect("uri should parse");
        let body = Bytes::from_static(br#"{"model":"gpt-5.2","messages":[]}"#);

        let rejection =
            request_model_local_rejection(&state, Some(&decision), &uri, &json_headers(), &body)
                .await
                .expect("model rejection should resolve");

        assert_eq!(
            rejection,
            Some(GatewayLocalAuthRejection::ModelNotAllowed {
                model: "gpt-5.2".to_string(),
            })
        );
    }

    #[tokio::test]
    async fn model_rejection_reuses_request_policy_snapshot_for_directive_base_model() {
        let state = state_with_rows(Vec::new());
        let mut decision = decision_with_allowed_models(vec!["gpt-5.6-sol".to_string()]);
        decision.model_directive_policy =
            crate::system_features::ModelDirectivePolicySnapshot::from_config_values(
                Some(&json!(true)),
                None,
            );
        let uri: Uri = "/v1/chat/completions".parse().expect("uri should parse");
        let body = Bytes::from_static(br#"{"model":"gpt-5.6-sol-high","messages":[]}"#);

        let rejection =
            request_model_local_rejection(&state, Some(&decision), &uri, &json_headers(), &body)
                .await
                .expect("model rejection should resolve");

        assert_eq!(rejection, None);
    }

    #[tokio::test]
    async fn model_rejection_uses_custom_policy_suffix_for_base_model_authorization() {
        let state = state_with_rows(Vec::new());
        let mut decision = decision_with_allowed_models(vec!["deployment-alias".to_string()]);
        decision.model_directive_policy =
            crate::system_features::ModelDirectivePolicySnapshot::from_config_values(
                Some(&json!(true)),
                Some(&json!({
                    "reasoning_effort": {
                        "api_formats": {
                            "openai:chat": {
                                "suffixes": ["VendorFuture"],
                                "mappings": {
                                    "VendorFuture": { "reasoning_effort": "high" }
                                }
                            }
                        }
                    }
                })),
            );
        let uri: Uri = "/v1/chat/completions".parse().expect("uri should parse");
        let body =
            Bytes::from_static(br#"{"model":"deployment-alias-VendorFuture","messages":[]}"#);

        let rejection =
            request_model_local_rejection(&state, Some(&decision), &uri, &json_headers(), &body)
                .await
                .expect("model rejection should resolve");

        assert_eq!(rejection, None);
    }

    #[tokio::test]
    async fn positive_balance_allows_unbounded_output_request_without_cost_estimate() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 1.0,
                    "output_price_per_1m": 2.0
                }]
            })),
            None,
            None,
            None,
        );
        for allow_wallet_overage in [false, true] {
            let state = state_with_quota_and_wallet(
                quota_availability(50.0, allow_wallet_overage),
                context.clone(),
            );
            let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
            let plan = execution_plan(
                json!({
                    "model": "gpt-5",
                    "messages": [{"role": "user", "content": "hi"}],
                    "stream": true
                }),
                "openai:chat",
            );

            let rejection = execution_plan_balance_capacity_rejection(
                &state,
                &decision,
                &plan,
                Some(&billing_report_context()),
            )
            .await
            .expect("quota rejection should resolve");

            assert_eq!(rejection, None);
        }
    }

    #[tokio::test]
    async fn unlimited_wallet_capacity_ignores_exhausted_non_overage_quota() {
        let context = billing_context_with_pricing(None, None, None, None);
        let mut wallet = sample_wallet("user-1", 0.0);
        wallet.limit_mode = "unlimited".to_string();
        let state =
            state_with_quota_context_and_wallet(quota_availability(0.0, false), context, wallet);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let auth_context = decision
            .auth_context
            .as_ref()
            .expect("decision should include auth context");

        let capacity = available_balance_capacity_usd(&state, auth_context)
            .await
            .expect("capacity should resolve");

        assert_eq!(capacity, None);
    }

    #[tokio::test]
    async fn positive_balance_does_not_allow_historical_invalid_processing_pricing() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 1.0,
                    "output_price_per_1m": 2.0
                }],
                "processing_tiers": {
                    "priority": {
                        "tiers": [{}],
                        "price_multiplier": -1.0
                    }
                }
            })),
            None,
            None,
            None,
        );
        let state = state_with_quota_and_wallet(quota_availability(50.0, true), context);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}],
                "service_tier": "priority",
                "max_completion_tokens": 1
            }),
            "openai:chat",
        );

        let error = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&billing_report_context()),
        )
        .await
        .expect_err("invalid configured processing pricing must stop authorization");

        assert!(error
            .into_message()
            .contains("explicit catalog contains malformed or unrecognized prices"));
    }

    #[tokio::test]
    async fn standalone_key_does_not_bypass_invalid_processing_pricing_validation() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{"up_to": null, "input_price_per_1m": 1.0}],
                "processing_tiers": {
                    "priority": {"tiers": [{}], "price_multiplier": 2.0}
                }
            })),
            None,
            None,
            None,
        );
        let state = state_with_quota_and_wallet(quota_availability(50.0, true), context);
        let mut decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        decision
            .auth_context
            .as_mut()
            .expect("auth context should exist")
            .api_key_is_standalone = true;
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}],
                "service_tier": "priority",
                "max_completion_tokens": 1
            }),
            "openai:chat",
        );

        let error = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&billing_report_context()),
        )
        .await
        .expect_err("standalone keys must still validate configured pricing");
        assert!(error
            .into_message()
            .contains("explicit catalog contains malformed or unrecognized prices"));
    }

    #[tokio::test]
    async fn unlimited_wallet_does_not_bypass_invalid_processing_pricing_validation() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{"up_to": null, "input_price_per_1m": 1.0}],
                "processing_tiers": {
                    "priority": {"tiers": [{}], "price_multiplier": 2.0}
                }
            })),
            None,
            None,
            None,
        );
        let state = state_with_quota_and_wallet(quota_availability(0.0, true), context);
        {
            let store = state
                .auth_wallet_store
                .as_ref()
                .expect("test wallet store should exist");
            let mut wallets = store.lock().expect("wallet store should lock");
            wallets
                .get_mut("wallet-user-1")
                .expect("test wallet should exist")
                .limit_mode = "unlimited".to_string();
        }
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}],
                "service_tier": "priority",
                "max_completion_tokens": 1
            }),
            "openai:chat",
        );

        let error = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&billing_report_context()),
        )
        .await
        .expect_err("unlimited wallets must still validate configured pricing");
        assert!(error
            .into_message()
            .contains("explicit catalog contains malformed or unrecognized prices"));
    }

    #[tokio::test]
    async fn standalone_and_unlimited_paths_keep_allowing_valid_pricing() {
        let valid_context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{"up_to": null, "input_price_per_1m": 1.0}]
            })),
            None,
            None,
            None,
        );
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}],
                "max_completion_tokens": 1
            }),
            "openai:chat",
        );
        let report_context = billing_report_context();

        let standalone_state =
            state_with_quota_and_wallet(quota_availability(50.0, true), valid_context.clone());
        let mut standalone_decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        standalone_decision
            .auth_context
            .as_mut()
            .expect("auth context should exist")
            .api_key_is_standalone = true;
        assert_eq!(
            execution_plan_balance_capacity_rejection(
                &standalone_state,
                &standalone_decision,
                &plan,
                Some(&report_context),
            )
            .await
            .expect("valid standalone pricing should resolve"),
            None
        );

        let unlimited_state =
            state_with_quota_and_wallet(quota_availability(0.0, true), valid_context);
        {
            let store = unlimited_state
                .auth_wallet_store
                .as_ref()
                .expect("test wallet store should exist");
            let mut wallets = store.lock().expect("wallet store should lock");
            wallets
                .get_mut("wallet-user-1")
                .expect("test wallet should exist")
                .limit_mode = "unlimited".to_string();
        }
        assert_eq!(
            execution_plan_balance_capacity_rejection(
                &unlimited_state,
                &decision_with_allowed_models(vec!["gpt-5".to_string()]),
                &plan,
                Some(&report_context),
            )
            .await
            .expect("valid unlimited-wallet pricing should resolve"),
            None
        );
    }

    #[tokio::test]
    async fn auth_capacity_reuses_quota_wallet_and_cost_estimate_within_ttl() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 0.0,
                    "output_price_per_1m": 60.0
                }]
            })),
            None,
            None,
            None,
        );
        let quota_calls = Arc::new(AtomicUsize::new(0));
        let model_context_calls = Arc::new(AtomicUsize::new(0));
        let candidate_repository =
            Arc::new(InMemoryMinimalCandidateSelectionReadRepository::seed(vec![
                sample_row(),
            ]));
        let billing_repository = Arc::new(FixedBillingReadRepository::with_counters(
            quota_availability(1.0, true),
            context,
            Arc::clone(&quota_calls),
            Arc::clone(&model_context_calls),
        ));
        let data = GatewayDataState::with_minimal_candidate_selection_and_billing_for_tests(
            candidate_repository,
            billing_repository,
        );
        let state = AppState::new()
            .expect("state should build")
            .with_data_state_for_tests(data)
            .with_auth_wallets_for_tests(vec![sample_wallet("user-1", 30.0)]);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}],
                "max_tokens": 100_000
            }),
            "openai:chat",
        );
        let report_context = billing_report_context();

        let first = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&report_context),
        )
        .await
        .expect("first auth capacity check should resolve");

        assert_eq!(first, None);
        assert_eq!(quota_calls.load(Ordering::Acquire), 1);
        assert_eq!(model_context_calls.load(Ordering::Acquire), 1);

        let store = state
            .auth_wallet_store
            .as_ref()
            .expect("test wallet store should exist");
        {
            let mut wallets = store.lock().expect("wallet store should lock");
            let wallet = wallets
                .get_mut("wallet-user-1")
                .expect("test wallet should exist");
            wallet.balance = 0.0;
            wallet.gift_balance = 0.0;
        }

        let cached = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&report_context),
        )
        .await
        .expect("cached auth capacity check should resolve");

        assert_eq!(cached, None);
        assert_eq!(quota_calls.load(Ordering::Acquire), 1);
        assert_eq!(model_context_calls.load(Ordering::Acquire), 1);

        state.invalidate_auth_context_cache();

        let refreshed = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&report_context),
        )
        .await
        .expect("refreshed auth capacity check should resolve");

        assert_eq!(
            refreshed,
            Some(GatewayLocalAuthRejection::BalanceDenied {
                remaining: Some(1.0),
            })
        );
        assert_eq!(quota_calls.load(Ordering::Acquire), 2);
    }

    #[tokio::test]
    async fn standalone_auth_capacity_reuses_pricing_validation_within_ttl() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 1.0,
                    "output_price_per_1m": 2.0
                }]
            })),
            None,
            None,
            None,
        );
        let quota_calls = Arc::new(AtomicUsize::new(0));
        let model_context_calls = Arc::new(AtomicUsize::new(0));
        let candidate_repository =
            Arc::new(InMemoryMinimalCandidateSelectionReadRepository::seed(vec![
                sample_row(),
            ]));
        let billing_repository = Arc::new(FixedBillingReadRepository::with_counters(
            quota_availability(1.0, true),
            context,
            Arc::clone(&quota_calls),
            Arc::clone(&model_context_calls),
        ));
        let data = GatewayDataState::with_minimal_candidate_selection_and_billing_for_tests(
            candidate_repository,
            billing_repository,
        );
        let state = AppState::new()
            .expect("state should build")
            .with_data_state_for_tests(data);
        let mut decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        decision
            .auth_context
            .as_mut()
            .expect("auth context should exist")
            .api_key_is_standalone = true;
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}]
            }),
            "openai:chat",
        );
        let report_context = billing_report_context();

        for _ in 0..2 {
            assert_eq!(
                execution_plan_balance_capacity_rejection(
                    &state,
                    &decision,
                    &plan,
                    Some(&report_context),
                )
                .await
                .expect("standalone pricing validation should resolve"),
                None
            );
        }
        assert_eq!(quota_calls.load(Ordering::Acquire), 0);
        assert_eq!(model_context_calls.load(Ordering::Acquire), 1);

        let cache_key = super::execution_plan_pricing_validation_cache_key(
            &plan,
            Some("model-1"),
            Some("gpt-5"),
            None,
        );
        assert_eq!(
            state.auth_request_cost_upper_bound_cache.get(
                &cache_key,
                state.frontdoor_runtime_guards.auth_capacity_cache_ttl,
            ),
            Some(Some(0.0))
        );
        state.invalidate_provider_routing_caches();
        assert_eq!(
            state.auth_request_cost_upper_bound_cache.get(
                &cache_key,
                state.frontdoor_runtime_guards.auth_capacity_cache_ttl,
            ),
            None
        );
    }

    #[tokio::test]
    async fn admin_bypass_limits_does_not_skip_unbounded_zero_balance_capacity() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 1.0,
                    "output_price_per_1m": 2.0
                }]
            })),
            None,
            None,
            None,
        );
        let state = state_with_quota_and_wallet(quota_availability(0.0, false), context);
        let mut decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        if let Some(auth_context) = decision.auth_context.as_mut() {
            auth_context.admin_bypass_limits = true;
        }
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": true
            }),
            "openai:chat",
        );
        let report_context = billing_report_context();

        let rejection = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&report_context),
        )
        .await
        .expect("quota rejection should resolve");

        assert_eq!(
            rejection,
            Some(GatewayLocalAuthRejection::BalanceDenied {
                remaining: Some(0.0),
            })
        );
    }

    #[tokio::test]
    async fn zero_balance_allows_a_proven_free_tier_execution_plan() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 100.0,
                    "output_price_per_1m": 100.0
                }]
            })),
            None,
            None,
            Some("free_tier"),
        );
        let state = state_with_quota_and_wallet(quota_availability(0.0, false), context);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}],
                "max_completion_tokens": 1_000_000
            }),
            "openai:chat",
        );
        let report_context = billing_report_context();

        let rejection = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&report_context),
        )
        .await
        .expect("free tier capacity check should resolve");

        assert_eq!(rejection, None);
    }

    #[tokio::test]
    async fn finalized_chat_output_fields_and_choice_count_bound_capacity() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 0.0,
                    "output_price_per_1m": 20.0
                }]
            })),
            None,
            None,
            None,
        );
        let state = state_with_quota_and_wallet(quota_availability(50.0, false), context);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}],
                "max_tokens": 1,
                "max_completion_tokens": 1_000_000,
                "n": 3
            }),
            "openai:chat",
        );

        let rejection = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&billing_report_context()),
        )
        .await
        .expect("quota rejection should resolve");

        assert_eq!(
            rejection,
            Some(GatewayLocalAuthRejection::BalanceDenied {
                remaining: Some(50.0),
            })
        );
    }

    #[tokio::test]
    async fn stateful_responses_request_skips_unprovable_capacity_rejection() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 100.0,
                    "output_price_per_1m": 100.0
                }]
            })),
            None,
            None,
            None,
        );
        let state = state_with_quota_and_wallet(quota_availability(0.01, false), context);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "input": "continue",
                "previous_response_id": "resp_123",
                "max_output_tokens": 1_000_000
            }),
            "openai:responses",
        );
        let report_context = billing_report_context();

        let rejection = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&report_context),
        )
        .await
        .expect("stateful request capacity check should resolve");

        assert_eq!(rejection, None);
    }

    #[tokio::test]
    async fn stateful_unavailable_estimate_still_rejects_invalid_processing_pricing() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 1.0,
                    "output_price_per_1m": 2.0
                }],
                "processing_tiers": {
                    "priority": {
                        "tiers": [{}],
                        "price_multiplier": 2.0
                    }
                }
            })),
            None,
            None,
            None,
        );
        let state = state_with_quota_and_wallet(quota_availability(1.0, false), context);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "input": "continue",
                "previous_response_id": "resp_123",
                "service_tier": "priority",
                "max_output_tokens": 1
            }),
            "openai:responses",
        );

        let error = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&billing_report_context()),
        )
        .await
        .expect_err("unavailable estimates must still validate configured processing pricing");

        assert!(error
            .into_message()
            .contains("explicit catalog contains malformed or unrecognized prices"));
    }

    #[tokio::test]
    async fn wallet_overage_policy_extends_known_cost_capacity_when_enabled() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 0.0,
                    "output_price_per_1m": 70.0
                }]
            })),
            None,
            None,
            None,
        );
        let state = state_with_quota_and_wallet(quota_availability(50.0, true), context);
        let decision = decision_with_allowed_models(vec!["gpt-5".to_string()]);
        let plan = execution_plan(
            json!({
                "model": "gpt-5",
                "messages": [{"role": "user", "content": "hi"}],
                "max_tokens": 1_000_000
            }),
            "openai:chat",
        );

        let rejection = execution_plan_balance_capacity_rejection(
            &state,
            &decision,
            &plan,
            Some(&billing_report_context()),
        )
        .await
        .expect("quota rejection should resolve");

        assert_eq!(rejection, None);
    }

    #[test]
    fn gemini_function_conversations_have_a_bounded_cost_estimate() {
        let body = json!({"model": "m", "contents": [
            {"role": "user", "parts": [{"text": "go"}]},
            {"role": "model", "parts": [{"text": "plan", "thought": true}, {"functionCall": {"name": "Write", "args": {}}}]},
            {"role": "user", "parts": [{"functionResponse": {"name": "Write", "response": {"result": "ok"}}}]}],
            "systemInstruction": {"parts": [{"text": "sys"}]},
            "tools": [{"functionDeclarations": [{"name": "Write", "parametersJsonSchema": {"type": "object"}}]}],
            "generationConfig": {"maxOutputTokens": 100}});
        assert!(super::request_input_is_self_contained(
            "gemini:generate_content",
            &body
        ));
        let mut hosted = body.clone();
        hosted["tools"]
            .as_array_mut()
            .unwrap()
            .push(json!({"googleSearch": {}}));
        // Hosted tools and stored files are estimable at fixed bounds.
        assert!(super::request_input_is_self_contained(
            "gemini:generate_content",
            &hosted
        ));
        let mut file = body;
        file["contents"][0]["parts"][0] = json!({"fileData": {"fileUri": "gs://x"}});
        assert!(super::request_input_is_self_contained(
            "gemini:generate_content",
            &file
        ));
        let mut unknown = file;
        unknown["contents"][0]["parts"][0] = json!({"futurePart": {}});
        assert!(!super::request_input_is_self_contained(
            "gemini:generate_content",
            &unknown
        ));
    }

    #[test]
    fn real_claude_code_requests_have_a_bounded_cost_estimate() {
        // Captured from the real CLI: adaptive thinking, output_config, context_management and a
        // mid-conversation system message. A hard cost limit must still be able to bound it.
        let mut body: serde_json::Value = serde_json::from_str(include_str!(
            "../../../../../crates/aether-ai/formats/tests/fixtures/real_clients/claude_messages.json"
        ))
        .expect("fixture parses");
        assert!(super::request_input_is_self_contained(
            "claude:messages",
            &body
        ));
        // Replayed reasoning is still literal text.
        body["messages"]
            .as_array_mut()
            .unwrap()
            .push(json!({"role": "assistant", "content": [
            {"type": "thinking", "thinking": "plan", "signature": "sig"},
            {"type": "redacted_thinking", "data": "opaque"},
            {"type": "tool_use", "id": "toolu_1", "name": "Write", "input": {"file_path": "a"}}]}));
        assert!(super::request_input_is_self_contained(
            "claude:messages",
            &body
        ));
        // Server tools are estimable with a fixed surcharge; unknown extensions fail closed.
        let mut server_tool = body.clone();
        server_tool["tools"]
            .as_array_mut()
            .unwrap()
            .push(json!({"type": "web_search_20250305", "name": "web_search"}));
        assert!(super::request_input_is_self_contained(
            "claude:messages",
            &server_tool
        ));
        assert_eq!(
            super::request_hosted_tools(&server_tool),
            vec![super::HostedToolKind::WebSearch]
        );
        let mut unknown = body.clone();
        unknown["future_extension"] = json!("abc");
        assert!(!super::request_input_is_self_contained(
            "claude:messages",
            &unknown
        ));
        let mut bad_thinking = body;
        bad_thinking["thinking"] = json!({"type": "enabled", "budget_tokens": "lots"});
        assert!(!super::request_input_is_self_contained(
            "claude:messages",
            &bad_thinking
        ));
    }

    #[test]
    fn daily_quota_estimate_falls_back_to_default_tiers_when_model_tiers_empty() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 3.0,
                    "output_price_per_1m": 15.0
                }]
            })),
            Some(json!({})),
            None,
            None,
        );

        let estimate =
            estimate_from_billing_context(&context, "openai:chat", 1_000_000, Some(1_000_000))
                .expect("estimate should be bounded");

        assert_eq!(estimate, 18.75);
    }

    #[test]
    fn daily_quota_estimate_applies_provider_key_rate_multiplier() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 1.0,
                    "output_price_per_1m": 2.0
                }]
            })),
            None,
            Some(json!({ "openai:chat": 2.0 })),
            None,
        );

        let estimate =
            estimate_from_billing_context(&context, "openai:chat", 1_000_000, Some(1_000_000))
                .expect("estimate should be bounded");

        assert_eq!(estimate, 6.5);
    }

    #[test]
    fn daily_quota_estimate_treats_free_tier_as_zero_cost() {
        let context = billing_context_with_pricing(
            Some(json!({
                "tiers": [{
                    "up_to": null,
                    "input_price_per_1m": 3.0,
                    "output_price_per_1m": 15.0
                }]
            })),
            None,
            Some(json!({ "openai:chat": 10.0 })),
            Some("free_tier"),
        );

        let estimate =
            estimate_from_billing_context(&context, "openai:chat", 1_000_000, Some(1_000_000))
                .expect("estimate should be bounded");

        assert_eq!(estimate, 0.0);
    }

    #[test]
    fn output_bound_uses_largest_supported_field_and_chat_choice_count() {
        let body = json!({
            "max_tokens": 1,
            "max_completion_tokens": 100_000,
            "max_output_tokens": 50_000,
            "n": 3
        });

        assert_eq!(max_output_tokens_from_request(&body), Some(100_000));
        assert_eq!(output_choice_count_upper_bound("openai:chat", &body), 3);
        assert_eq!(
            output_choice_count_upper_bound("openai:responses", &body),
            1
        );
    }

    #[test]
    fn authorization_cache_key_includes_effective_cache_ttl() {
        let plan = execution_plan(json!({"model": "gpt-5.6-sol"}), "openai:responses");
        let without_ttl = execution_plan_cost_upper_bound_cache_key(
            &plan,
            Some("model-1"),
            Some("gpt-5.6-sol"),
            "openai:responses",
            100,
            Some(10),
            true,
            Some("priority"),
            None,
        );
        let with_ttl = execution_plan_cost_upper_bound_cache_key(
            &plan,
            Some("model-1"),
            Some("gpt-5.6-sol"),
            "openai:responses",
            100,
            Some(10),
            true,
            Some("priority"),
            Some(30),
        );

        assert_ne!(without_ttl, with_ttl);
    }

    #[test]
    fn indirect_request_inputs_are_not_treated_as_body_bounded() {
        let self_contained = json!({
            "input": [{
                "role": "user",
                "content": [{"type": "input_text", "text": "hello"}]
            }],
            "tools": [{"type": "function", "name": "lookup", "parameters": {}}]
        });
        assert!(openai_request_input_is_self_contained(
            "openai:responses",
            &self_contained
        ));

        for indirect in [
            json!({"input": "continue", "previous_response_id": "resp_123"}),
            json!({"input": "hello", "conversation": "conv_123"}),
            json!({"prompt": {"id": "pmpt_123", "variables": {}}}),
            json!({"input": [{"type": "item_reference", "id": "item_123"}]}),
            json!({"input": [{"type": "url", "url": "https://example.test/document"}]}),
        ] {
            assert!(!openai_request_input_is_self_contained(
                "openai:responses",
                &indirect
            ));
        }
        assert!(!openai_request_input_is_self_contained(
            "claude:messages",
            &self_contained
        ));
    }

    #[test]
    fn multimodal_parts_get_a_fixed_bounded_cost_estimate() {
        let tokens = |kind: super::MediaPartKind| match kind {
            super::MediaPartKind::Image => 1000,
            super::MediaPartKind::Audio => 200,
            super::MediaPartKind::Video => 3000,
            super::MediaPartKind::File => 500,
        };
        let big = format!("data:image/png;base64,{}", "A".repeat(200_000));
        let text_only =
            json!({"messages": [{"role": "user", "content": [{"type": "text", "text": "hi"}]}]});
        let base =
            super::request_input_token_count_policy_bound_with("openai:chat", &text_only, tokens);

        // OpenAI Chat Completions: data URI and URL images, inline audio, inline file.
        let chat = json!({"messages": [{"role": "user", "content": [
            {"type": "text", "text": "hi"},
            {"type": "image_url", "image_url": {"url": big, "detail": "high"}},
            {"type": "image_url", "image_url": {"url": "https://example.test/a.png"}},
            {"type": "input_audio", "input_audio": {"data": "UklGRg==", "format": "wav"}},
            {"type": "file", "file": {"filename": "a.pdf", "file_data": "data:application/pdf;base64,JVBERi0="}}]}]});
        assert!(request_input_is_self_contained("openai:chat", &chat));
        let bound =
            super::request_input_token_count_policy_bound_with("openai:chat", &chat, tokens);
        assert!(bound >= base + 1000 * 2 + 200 + 500, "{bound}");
        assert!(bound < 20_000, "base64 bytes must not be counted: {bound}");

        // OpenAI Responses: input_image by URL, data URI or stored image id; inline file.
        let responses = json!({"input": [{"role": "user", "content": [
            {"type": "input_text", "text": "hi"},
            {"type": "input_image", "image_url": big},
            {"type": "input_image", "file_id": "file_img"},
            {"type": "input_file", "filename": "a.pdf", "file_data": "JVBERi0="},
            {"type": "input_file", "file_url": "https://example.test/a.pdf"}]}]});
        assert!(request_input_is_self_contained(
            "openai:responses",
            &responses
        ));
        assert!(
            super::request_input_token_count_policy_bound_with(
                "openai:responses",
                &responses,
                tokens
            ) < 20_000
        );
        // Claude: base64/url/file images and base64/url/text documents in user turns.
        let mut claude = claude_text_request();
        claude["messages"][0]["content"] = json!([
            {"type": "text", "text": "hi"},
            {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "A".repeat(200_000)}},
            {"type": "image", "source": {"type": "url", "url": "https://example.test/a.png"}},
            {"type": "image", "source": {"type": "file", "file_id": "file_1"}},
            {"type": "document", "source": {"type": "base64", "media_type": "application/pdf", "data": "JVBERi0="}, "title": "a"}]);
        assert!(request_input_is_self_contained("claude:messages", &claude));
        assert!(
            super::request_input_token_count_policy_bound_with("claude:messages", &claude, tokens)
                < 30_000
        );
        let mut system_image = claude_text_request();
        system_image["system"] = json!([{"type": "image", "source": {"type": "url", "url": "https://example.test/a.png"}}]);
        assert!(!request_input_is_self_contained(
            "claude:messages",
            &system_image
        ));

        // Gemini inlineData is bounded by mime type, not by base64 length.
        let gemini = json!({"contents": [{"role": "user", "parts": [
            {"text": "hi"},
            {"inlineData": {"mimeType": "image/png", "data": "A".repeat(200_000)}},
            {"inline_data": {"mime_type": "audio/wav", "data": "UklGRg=="}},
            {"inlineData": {"mimeType": "video/mp4", "data": "AAAA"}}]}]});
        assert!(request_input_is_self_contained(
            "gemini:generate_content",
            &gemini
        ));
        let bound = super::request_input_token_count_policy_bound_with(
            "gemini:generate_content",
            &gemini,
            tokens,
        );
        assert!((1000 + 200 + 3000..20_000).contains(&bound), "{bound}");
    }

    #[tokio::test]
    async fn long_context_reservation_does_not_price_json_bytes_as_tokens() {
        let mut context = billing_context_with_pricing(
            Some(json!({"tiers": [
                {"up_to": 272000, "input_price_per_1m": 2.5, "output_price_per_1m": 10.0,
                 "cache_creation_price_per_1m": 3.125, "cache_read_price_per_1m": 0.25},
                {"up_to": null, "input_price_per_1m": 4.0, "output_price_per_1m": 15.0,
                 "cache_creation_price_per_1m": 5.0, "cache_read_price_per_1m": 0.4}
            ]})),
            None,
            None,
            None,
        );
        context.global_model_config = Some(json!({"context_limit": 1000000}));
        context.model_config = Some(json!({"context_limit": 200000}));
        let state = state_with_quota_and_wallet(quota_availability(7.0, false), context);
        for (format, body) in [
            (
                "openai:responses",
                json!({"input": "word ".repeat(180000), "max_output_tokens": 4096}),
            ),
            (
                "openai:chat",
                json!({"messages": [{"role":"user", "content":"word ".repeat(180000)}], "max_tokens": 4096}),
            ),
        ] {
            let tokens = super::request_input_token_count_policy_bound(format, &body);
            assert!((225000..226000).contains(&tokens));
            let cost = super::estimate_execution_plan_cost_upper_bound_usd_inner(
                &state,
                &execution_plan(body, format),
                Some(&billing_report_context()),
            )
            .await
            .unwrap()
            .unwrap();
            assert!((cost - 0.54096).abs() < 1e-8, "{cost}");
            assert!(2.64 + cost < 7.0, "the incident must remain admissible");
        }
    }

    #[test]
    fn input_estimate_keeps_multilingual_content_and_tool_schemas() {
        let plain = json!({"input": "hello world ".repeat(1000)});
        let chinese = json!({"input": "中文测试".repeat(1000)});
        let tools = json!({"input": plain["input"], "tools": [{"type":"function",
            "name":"lookup", "description":"large schema ".repeat(1000), "parameters":{"type":"object"}}]});
        let estimate =
            |body| super::request_input_token_count_policy_bound("openai:responses", body);
        assert!((3000..3100).contains(&estimate(&plain)));
        assert!((6000..6100).contains(&estimate(&chinese)));
        assert!(estimate(&tools) > estimate(&plain) + 3000);
    }

    #[test]
    fn hosted_tools_and_stored_files_get_a_fixed_cost_estimate() {
        use super::HostedToolKind::{ImageGeneration, Other, WebSearch};
        for (tool, kind) in [
            (json!({"type": "web_search"}), WebSearch),
            (json!({"type": "web_search_preview"}), WebSearch),
            (json!({"type": "image_generation"}), ImageGeneration),
            (
                json!({"type": "code_interpreter", "container": {"type": "auto"}}),
                Other,
            ),
            (
                json!({"type": "mcp", "server_label": "x", "server_url": "https://example.test"}),
                Other,
            ),
            (json!({"type": "computer_use_preview"}), Other),
            (
                json!({"type": "file_search", "vector_store_ids": ["vs_1"]}),
                Other,
            ),
        ] {
            let body = json!({"input": "x", "tools": [tool, {"type": "function", "name": "f"}]});
            assert!(request_input_is_self_contained("openai:responses", &body));
            assert_eq!(super::request_hosted_tools(&body), vec![kind]);
        }
        let chat = json!({"messages": [{"role": "user", "content": [
            {"type": "text", "text": "news"},
            {"type": "file", "file": {"file_id": "file_1"}}]}], "web_search_options": {}});
        assert!(request_input_is_self_contained("openai:chat", &chat));
        assert_eq!(super::request_hosted_tools(&chat), vec![WebSearch]);
        let stored = json!({"input": [{"role": "user", "content": [
            {"type": "input_file", "file_id": "file_123"}]}]});
        assert!(request_input_is_self_contained("openai:responses", &stored));
        assert!(super::request_hosted_tools(&stored).is_empty());
        // Conversation state stays unestimable (422 fallback).
        for body in [
            json!({"input": "continue", "previous_response_id": "resp_1"}),
            json!({"input": [{"type": "item_reference", "id": "item_1"}]}),
        ] {
            assert!(!request_input_is_self_contained("openai:responses", &body));
        }

        let gemini = json!({"contents": [{"role": "user", "parts": [
            {"text": "news"},
            {"fileData": {"mimeType": "video/mp4", "fileUri": "gs://x"}}]}],
            "tools": [{"googleSearch": {}}, {"codeExecution": {}}, {"functionDeclarations": []}]});
        assert!(request_input_is_self_contained(
            "gemini:generate_content",
            &gemini
        ));
        assert_eq!(super::request_hosted_tools(&gemini), vec![WebSearch, Other]);

        let mut claude = claude_text_request();
        claude["tools"] = json!([
            {"type": "web_search_20250305", "name": "web_search", "max_uses": 3},
            {"type": "code_execution_20250522", "name": "code_execution"}]);
        claude["mcp_servers"] =
            json!([{"type": "url", "url": "https://example.test", "name": "m"}]);
        claude["messages"] = json!([
            {"role": "user", "content": [
                {"type": "text", "text": "news"},
                {"type": "document", "source": {"type": "file", "file_id": "file_1"}}]},
            {"role": "assistant", "content": [
                {"type": "server_tool_use", "id": "srvtoolu_1", "name": "web_search", "input": {"query": "news"}},
                {"type": "web_search_tool_result", "tool_use_id": "srvtoolu_1", "content": []}]},
            {"role": "user", "content": "more"}]);
        assert!(request_input_is_self_contained("claude:messages", &claude));
        assert_eq!(super::request_hosted_tools(&claude), vec![WebSearch, Other]);
        assert!(super::request_hosted_tools(&claude_text_request()).is_empty());

        assert_eq!(super::parse_usage_policy_tool_cost_usd(None, 0.05), 0.05);
        assert_eq!(
            super::parse_usage_policy_tool_cost_usd(Some(" 0.2 "), 0.05),
            0.2
        );
        assert_eq!(
            super::parse_usage_policy_tool_cost_usd(Some("0"), 0.05),
            0.0
        );
        assert_eq!(
            super::parse_usage_policy_tool_cost_usd(Some("-1"), 0.05),
            0.05
        );
        assert_eq!(
            super::parse_usage_policy_tool_cost_usd(Some("NaN"), 0.15),
            0.15
        );
    }

    #[test]
    fn usage_policy_media_tokens_parse_with_safe_fallback() {
        assert_eq!(super::parse_usage_policy_media_tokens(None, 4096), 4096);
        assert_eq!(
            super::parse_usage_policy_media_tokens(Some(" 1500 "), 4096),
            1500
        );
        assert_eq!(super::parse_usage_policy_media_tokens(Some("0"), 256), 256);
        assert_eq!(super::parse_usage_policy_media_tokens(Some("-1"), 256), 256);
        assert_eq!(super::parse_usage_policy_media_tokens(Some("x"), 256), 256);
    }

    fn claude_text_request() -> serde_json::Value {
        json!({
            "model": "gpt-5", "max_tokens": 256,
            "system": [{"type": "text", "text": "Synthetic fixture only.", "cache_control": {"type": "ephemeral", "ttl": "1h"}}],
            "messages": [{"role": "user", "content": "hello"}]
        })
    }

    fn claude_tool_request() -> serde_json::Value {
        let mut body = claude_text_request();
        body["tools"] = json!([{
            "name": "literal_lookup", "description": "Returns literal data, not remote context.",
            "input_schema": {"type": "object", "properties": {
                "file_id": {"type": "string"}, "vector_store_ids": {"type": "array"},
                "source": {"type": "object", "properties": {"type": {"const": "url"}}}
            }}
        }]);
        body["tool_choice"] = json!({"type": "auto"});
        body["messages"] = json!([
            {"role": "user", "content": [{"type": "text", "text": "Use the literal argument."}]},
            {"role": "assistant", "content": [{"type": "tool_use", "id": "call_1", "name": "literal_lookup",
                "input": {"file_id": "literal", "source": {"type": "url", "url": "https://example.invalid"}}}]},
            {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "call_1",
                "content": [{"type": "text", "text": "literal result"}]}]},
            {"role": "assistant", "content": [{"type": "tool_use", "id": "call_2", "name": "literal_lookup", "input": {}}]},
            {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "call_2", "content": "done", "is_error": false}]}
        ]);
        body
    }

    #[test]
    fn claude_bounded_text_and_ordinary_tool_history_are_supported() {
        for body in [claude_text_request(), claude_tool_request()] {
            assert!(request_input_is_self_contained("claude:messages", &body));
            assert_eq!(max_output_tokens_from_request(&body), Some(256));
        }
        let mut history = claude_tool_request();
        history["tools"] = json!([]);
        history["thinking"] = json!({"type": "disabled"});
        assert!(request_input_is_self_contained("claude:messages", &history));
        history.as_object_mut().unwrap().remove("tools");
        assert!(request_input_is_self_contained("claude:messages", &history));
        for (pointer, value) in [
            ("/messages/2/content/0/tool_use_id", json!("historical_id")),
            ("/messages/3/content/0/id", json!("call_1")),
            ("/messages/1/content/0/name", json!("historical_name")),
        ] {
            let mut body = claude_tool_request();
            *body.pointer_mut(pointer).unwrap() = value;
            assert!(request_input_is_self_contained("claude:messages", &body));
        }
    }

    #[test]
    fn claude_input_policy_includes_framing_tools_and_structural_margins() {
        let plain = claude_text_request();
        let tools = claude_tool_request();
        let plain_bytes = super::json_input_token_estimate(&plain);
        let tool_bytes = super::json_input_token_estimate(&tools);
        let plain_bound = super::request_input_token_count_policy_bound("claude:messages", &plain);
        let tool_bound = super::request_input_token_count_policy_bound("claude:messages", &tools);
        assert!(plain_bound > plain_bytes + 4096);
        assert!(tool_bound > tool_bytes + 4096 + 1024);
        assert!(tool_bound - tool_bytes > plain_bound - plain_bytes);
        assert_eq!(
            super::request_input_token_count_policy_bound("openai:chat", &plain),
            plain_bytes
        );
        for (bytes, nodes, tools) in [(u64::MAX, 0, 0), (0, u64::MAX, 0), (0, 0, u64::MAX)] {
            let bound = super::claude_framed_input_policy_bound(bytes, nodes, tools);
            assert_eq!(bound, u64::MAX);
            assert!(
                i64::try_from(bound).is_err(),
                "overflow cannot reach paid authorization"
            );
        }
    }

    #[test]
    fn claude_media_references_and_opaque_content_fail_closed() {
        for block in [
            json!({"type": "file", "file_id": "file_1"}),
            json!({"type": "url", "url": "https://example.invalid"}),
            json!({"type": "thinking", "thinking": "opaque", "signature": "signed"}),
            json!({"type": "redacted_thinking", "data": "opaque"}),
            json!({"type": "server_tool_use", "id": "server_1", "name": "web_search", "input": {}}),
            json!({"type": "text", "text": "hello", "source": {"file_id": "file_1"}}),
            json!({"type": "future_block", "text": "opaque"}),
        ] {
            let mut body = claude_text_request();
            body["messages"][0]["content"] = json!([block.clone()]);
            assert!(!request_input_is_self_contained("claude:messages", &body));
            body["system"] = json!([block]);
            body["messages"][0]["content"] = json!("hello");
            assert!(!request_input_is_self_contained("claude:messages", &body));
        }
        for field in [
            "context_management",
            "previous_response_id",
            "conversation",
            "cached_content",
            "thinking",
            "unknown",
        ] {
            let mut body = claude_text_request();
            body[field] = json!({"id": "external"});
            assert!(
                !request_input_is_self_contained("claude:messages", &body),
                "{field}"
            );
        }
    }

    #[test]
    fn claude_server_tools_are_surcharged_and_nontext_tool_results_fail_closed() {
        for kind in [
            "web_search_20250305",
            "computer_20250124",
            "bash_20250124",
            "code_execution_20250522",
            "mcp",
            "future",
        ] {
            let mut body = claude_tool_request();
            body["tools"][0]["type"] = json!(kind);
            assert!(
                request_input_is_self_contained("claude:messages", &body),
                "{kind}"
            );
            assert_eq!(super::request_hosted_tools(&body).len(), 1, "{kind}");
        }
        for content in [
            json!([{"type": "image", "source": {"type": "url", "url": "https://example.invalid"}}]),
            json!([{"type": "document", "source": {"file_id": "file_1"}}]),
            json!({"file_id": "file_1"}),
        ] {
            let mut body = claude_tool_request();
            body["messages"][2]["content"][0]["content"] = content;
            assert!(!request_input_is_self_contained("claude:messages", &body));
        }
    }

    #[test]
    fn claude_malformed_bounds_tools_and_cache_hints_fail_closed() {
        for bound in [json!(0), json!(-1), json!(1.5), json!("256"), json!(null)] {
            let mut body = claude_text_request();
            body["max_tokens"] = bound;
            assert!(!request_input_is_self_contained("claude:messages", &body));
        }
        for hint in [
            json!({"type": "reference"}),
            json!({"type": "ephemeral", "ttl": "24h"}),
            json!({"type": "ephemeral", "id": "remote"}),
        ] {
            let mut body = claude_text_request();
            body["system"][0]["cache_control"] = hint;
            assert!(!request_input_is_self_contained("claude:messages", &body));
        }
        for (pointer, value) in [
            ("/messages/2/content/0/tool_use_id", json!(null)),
            ("/messages/3/content/0/id", json!(null)),
            ("/messages/1/role", json!("user")),
            ("/messages/2/role", json!("assistant")),
            ("/messages/1/content/0/name", json!(null)),
            ("/tools/0/input_schema", json!("opaque")),
            ("/tool_choice", json!({"type": "tool", "name": null})),
        ] {
            let mut body = claude_tool_request();
            *body.pointer_mut(pointer).expect("fixture pointer") = value;
            assert!(
                !request_input_is_self_contained("claude:messages", &body),
                "{pointer}"
            );
        }
    }

    #[test]
    fn gemini_output_aliases_count_conservatively_but_semantics_remain_unsupported() {
        for config in ["generationConfig", "generation_config"] {
            for tokens in ["maxOutputTokens", "max_output_tokens"] {
                for candidates in ["candidateCount", "candidate_count"] {
                    let body = json!({config: {tokens: 256, candidates: 3}});
                    assert_eq!(max_output_tokens_from_request(&body), Some(256));
                    assert_eq!(
                        output_choice_count_upper_bound("gemini:generate_content", &body),
                        3
                    );
                    assert!(!request_input_is_self_contained(
                        "gemini:generate_content",
                        &body
                    ));
                }
            }
        }
        let body = json!({"max_tokens": 1, "generationConfig": {"maxOutputTokens": 256, "candidateCount": 2},
            "generation_config": {"max_output_tokens": 512, "candidate_count": 3}});
        assert_eq!(max_output_tokens_from_request(&body), Some(512));
        assert_eq!(
            output_choice_count_upper_bound("gemini:generate_content", &body),
            3
        );
        let body = json!({"generationConfig": {"maxOutputTokens": u64::MAX, "candidateCount": 2}});
        let bound = max_output_tokens_from_request(&body)
            .unwrap()
            .saturating_mul(output_choice_count_upper_bound(
                "gemini:generate_content",
                &body,
            ));
        assert!(i64::try_from(bound).is_err());
        assert!(!request_input_is_self_contained(
            "future:messages",
            &claude_text_request()
        ));
    }

    #[tokio::test]
    async fn claude_estimator_bounds_text_tools_and_rejects_external_context() {
        let context = billing_context_with_pricing(
            Some(json!({"tiers": [{"up_to": null,
            "input_price_per_1m": 1.0, "output_price_per_1m": 2.0,
            "cache_creation_price_per_1m": 1.25, "cache_read_price_per_1m": 0.1}]})),
            None,
            None,
            None,
        );
        let state = state_with_quota_and_wallet(quota_availability(0.1, false), context);
        for body in [claude_text_request(), claude_tool_request()] {
            let estimate = super::estimate_execution_plan_cost_upper_bound_usd_inner(
                &state,
                &execution_plan(body, "claude:messages"),
                Some(&billing_report_context()),
            )
            .await
            .expect("pricing should validate")
            .expect("self-contained request must have a bound");
            assert!(estimate.is_finite() && estimate > 0.0 && estimate < 0.1);
        }
        let mut body = claude_text_request();
        body["messages"][0]["content"] =
            json!([{"type": "document", "source": {"file_id": "external"}}]);
        assert_eq!(
            super::estimate_execution_plan_cost_upper_bound_usd_inner(
                &state,
                &execution_plan(body, "claude:messages"),
                Some(&billing_report_context())
            )
            .await
            .expect("pricing should validate"),
            None
        );
        let mut body = claude_text_request();
        body["max_tokens"] = json!(u64::MAX);
        assert_eq!(
            super::estimate_execution_plan_cost_upper_bound_usd_inner(
                &state,
                &execution_plan(body, "claude:messages"),
                Some(&billing_report_context())
            )
            .await
            .expect("overflow must fail closed"),
            None
        );
    }

    #[derive(Default)]
    struct ObservedCostReservations {
        inner: aether_data::repository::settlement::InMemorySettlementRepository,
        allowed: Arc<AtomicUsize>,
        rejected: Arc<AtomicUsize>,
        reserved_units: Arc<std::sync::atomic::AtomicU64>,
    }

    use aether_data_contracts::repository::settlement;
    use settlement::SettlementWriteRepository;

    #[async_trait]
    impl SettlementWriteRepository for ObservedCostReservations {
        async fn reserve_usage_policy_request(
            &self,
            input: settlement::ReserveUsagePolicyRequestInput,
        ) -> Result<settlement::ReserveUsagePolicyRequestOutcome, DataLayerError> {
            self.inner.reserve_usage_policy_request(input).await
        }

        async fn release_usage_policy_request_admission(
            &self,
            input: settlement::ReleaseUsagePolicyRequestAdmissionInput,
        ) -> Result<Option<settlement::StoredUsagePolicyRequestAdmission>, DataLayerError> {
            self.inner
                .release_usage_policy_request_admission(input)
                .await
        }

        async fn reserve_usage_policy_cost(
            &self,
            input: settlement::ReserveUsagePolicyCostInput,
        ) -> Result<settlement::ReserveUsagePolicyCostOutcome, DataLayerError> {
            assert!(
                input.reserved_cost_units > 0,
                "paid text must reserve a nonzero bound"
            );
            let units = input.reserved_cost_units;
            let outcome = self.inner.reserve_usage_policy_cost(input).await?;
            match &outcome {
                settlement::ReserveUsagePolicyCostOutcome::Allowed { .. } => {
                    self.reserved_units.store(units, Ordering::SeqCst);
                    self.allowed.fetch_add(1, Ordering::SeqCst);
                }
                settlement::ReserveUsagePolicyCostOutcome::Rejected { .. } => {
                    self.rejected.fetch_add(1, Ordering::SeqCst);
                }
                _ => {}
            }
            Ok(outcome)
        }

        async fn reconcile_usage_policy_cost(
            &self,
            input: settlement::ReconcileUsagePolicyCostInput,
        ) -> Result<Option<settlement::StoredUsagePolicyCostReservation>, DataLayerError> {
            self.inner.reconcile_usage_policy_cost(input).await
        }

        async fn settle_usage(
            &self,
            input: settlement::UsageSettlementInput,
        ) -> Result<Option<settlement::StoredUsageSettlement>, DataLayerError> {
            self.inner.settle_usage(input).await
        }
    }

    #[tokio::test]
    async fn claude_messages_executor_reserves_finite_plan_budget_before_upstream() {
        // Real admission, estimator, candidate loop and repository; only transport
        // is replaced. This cannot spend money or imply a real provider passed.
        for (limit, expected_status) in [(0.1, 200), (0.00000001, 429), (0.0, 429)] {
            let now = u64::try_from(chrono::Utc::now().timestamp()).expect("current epoch");
            let context = billing_context_with_pricing(
                Some(json!({"tiers": [{"up_to": null,
                "input_price_per_1m": 1.0, "output_price_per_1m": 2.0}]})),
                None,
                None,
                None,
            );
            let mut fixture_usage = aether_billing::BillingUsageInput::new("chat");
            fixture_usage.api_format = Some("claude:messages".to_string());
            fixture_usage.input_tokens = 1000;
            fixture_usage.output_tokens = 32;
            let fixture_cost = aether_billing::BillingService::new()
                .calculate(
                    &aether_billing::BillingModelPricingSnapshot::from(&context),
                    &fixture_usage,
                )
                .expect("fixture pricing")
                .actual_total_cost;
            let fixture_cost_units = aether_data_contracts::repository::billing::nonnegative_usd_to_usage_policy_cost_units(fixture_cost)
                .expect("finite fixture cost");
            let mut billing =
                FixedBillingReadRepository::new(quota_availability(1.0, true), context);
            billing.entitlements.push(UserPlanEntitlementRecord {
                id: "claude-entitlement".to_string(), user_id: "user-1".to_string(),
                plan_id: "claude-plan".to_string(), payment_order_id: "claude-order".to_string(),
                status: "active".to_string(), starts_at_unix_secs: now - 60, expires_at_unix_secs: now + 3600,
                entitlements_snapshot: json!([{"type": "usage_policy", "rules": [{
                    "metric": "actual_cost_usd", "window": {"kind": "rolling", "seconds": 3600}, "limit": limit
                }]}]), created_at_unix_secs: now - 60, updated_at_unix_secs: now - 60,
            });
            let reservations = Arc::new(ObservedCostReservations::default());
            let upstream_calls = Arc::new(AtomicUsize::new(0));
            let calls = upstream_calls.clone();
            let observed = reservations.allowed.clone();
            let reserved_units = reservations.reserved_units.clone();
            let data = GatewayDataState::with_billing_reader_for_tests(Arc::new(billing))
                .with_settlement_writer_for_tests(reservations.clone());
            let state = AppState::new().expect("state should build")
                .with_data_state_for_tests(data)
                .with_auth_wallets_for_tests(vec![sample_wallet("user-1", 1.0)])
                .with_usage_runtime_for_tests(crate::usage::UsageRuntimeConfig { enabled: true, ..Default::default() })
                .with_execution_runtime_sync_override_for_tests(move |plan| {
                    assert!(observed.load(Ordering::SeqCst) > 0, "reservation must precede transport");
                    assert!(reserved_units.load(Ordering::SeqCst) >= fixture_cost_units, "reservation must cover the fixture's priced usage");
                    calls.fetch_add(1, Ordering::SeqCst);
                    Ok(aether_contracts::ExecutionResult {
                        request_id: plan.request_id.clone(), candidate_id: plan.candidate_id.clone(),
                        status_code: 200, headers: Default::default(), response_observation: None,
                        body: Some(aether_contracts::ResponseBody { json_body: Some(json!({
                            "id": "msg_fixture", "type": "message", "role": "assistant", "model": "gpt-5",
                            "content": [{"type": "text", "text": "ok"}], "stop_reason": "end_turn",
                            "usage": {"input_tokens": 1000, "output_tokens": 32}
                        })), body_bytes_b64: None }), telemetry: None, error: None,
                    })
                });
            let mut decision = GatewayControlDecision::synthetic(
                "/v1/messages",
                Some("ai_public".to_string()),
                Some("claude".to_string()),
                Some("messages".to_string()),
                Some("claude:messages".to_string()),
            )
            .with_execution_runtime_candidate(true);
            decision.auth_context =
                decision_with_allowed_models(vec!["gpt-5".to_string()]).auth_context;
            let admission = crate::plan_usage_policy::check_and_acquire_http_plan_usage_policy(
                &state,
                Some(&decision),
                "request-1",
                now * 1000,
                Some("gpt-5"),
            )
            .await
            .expect("HTTP admission must preserve the finite cost policy");
            let mut request = http::Request::builder()
                .uri("/v1/messages")
                .body(())
                .expect("request");
            request.extensions_mut().insert(
                admission
                    .reservation_context
                    .expect("finite cost reservation context"),
            );
            let (parts, _) = request.into_parts();
            let mut plan = execution_plan(claude_tool_request(), "claude:messages");
            plan.url = "https://example.invalid/v1/messages".to_string();
            let outcome = crate::executor::execute_sync_plan_and_reports(
                &state,
                &parts,
                "claude-budget-fixture",
                &decision,
                "claude_chat_sync",
                vec![crate::ai_serving::AiSyncAttempt {
                    plan,
                    report_kind: None,
                    report_context: Some(billing_report_context()),
                }],
            )
            .await
            .expect("bounded Claude execution should reach an admission outcome");
            let crate::executor::LocalExecutionRequestOutcome::Responded(response) = outcome else {
                panic!("expected HTTP outcome")
            };
            assert_eq!(response.status().as_u16(), expected_status, "limit {limit}");
            assert_eq!(
                upstream_calls.load(Ordering::SeqCst),
                usize::from(expected_status == 200)
            );
            assert_eq!(
                reservations.allowed.load(Ordering::SeqCst),
                usize::from(expected_status == 200)
            );
            assert_eq!(
                reservations.rejected.load(Ordering::SeqCst),
                usize::from(expected_status == 429)
            );
        }
    }

    #[test]
    fn omitted_max_tokens_reserves_small_default_capped_by_output_limit() {
        assert_eq!(default_reserved_output_tokens(Some(128_000), 4096), 4096);
        assert_eq!(default_reserved_output_tokens(Some(1024), 4096), 1024);
        assert_eq!(default_reserved_output_tokens(None, 4096), 4096);
        assert_eq!(default_reserved_output_tokens(Some(0), 4096), 4096);
    }

    #[test]
    fn usage_policy_default_output_tokens_parses_env_with_fallback() {
        assert_eq!(parse_usage_policy_default_output_tokens(None), 4096);
        assert_eq!(
            parse_usage_policy_default_output_tokens(Some(" 8192 ")),
            8192
        );
        assert_eq!(parse_usage_policy_default_output_tokens(Some("0")), 4096);
        assert_eq!(parse_usage_policy_default_output_tokens(Some("-5")), 4096);
        assert_eq!(parse_usage_policy_default_output_tokens(Some("abc")), 4096);
    }
}
