use std::collections::BTreeMap;

use aether_data_contracts::repository::global_models::StoredPublicGlobalModel;
use axum::{
    body::Body,
    http::StatusCode,
    response::{IntoResponse, Response},
    Json,
};
use serde_json::{json, Map, Value};

use super::{
    models_route::{await_models_route_read, list_provider_model_rows_for_query_formats},
    models_shared::models_query_api_formats,
    AppState,
};

const API_FORMAT: &str = "openai:chat";
const MAX_MODELS: usize = 512;
const MAX_PROVIDER_MODEL_MAPPINGS: usize = 4096;
const MAX_RESPONSE_BYTES: usize = 8 * 1024 * 1024;

pub(super) fn unavailable_response() -> Response<Body> {
    (StatusCode::SERVICE_UNAVAILABLE, Json(json!({
        "error": {"code": "catalog_unavailable", "message": "Model catalog is temporarily unavailable"}
    }))).into_response()
}

fn config(models: Map<String, Value>, base_url: &str) -> Value {
    json!({
        "$schema": "https://opencode.ai/config.json",
        "provider": {
            "aether": {
                "npm": "@ai-sdk/openai-compatible",
                "name": "Aether",
                "options": {"baseURL": format!("{}/v1", base_url.trim_end_matches('/'))},
                "models": models
            }
        }
    })
}

pub(super) async fn build_response(
    state: &AppState,
    auth: &crate::data::auth::GatewayAuthApiKeySnapshot,
    base_url: &str,
) -> Response<Body> {
    // Authorize the client's chat protocol, not the upstream protocol used by
    // the gateway's existing conversation-format conversion path.
    if auth.effective_allowed_api_formats().is_some_and(|allowed| {
        !allowed
            .iter()
            .any(|format| crate::ai_serving::api_format_permission_covers(format, API_FORMAT))
    }) {
        return Json(config(Map::new(), base_url)).into_response();
    }

    let payload = await_models_route_read("opencode_catalog", async {
        let query_formats = models_query_api_formats(API_FORMAT)
            .iter()
            .copied()
            .filter(|format| {
                matches!(
                    *format,
                    "openai:chat"
                        | "openai:responses"
                        | "claude:messages"
                        | "gemini:generate_content"
                )
            })
            .collect::<Vec<_>>();
        let Some(rows) =
            list_provider_model_rows_for_query_formats(state, &query_formats, Some(auth)).await
        else {
            return Ok(None);
        };
        if rows.len() > MAX_PROVIDER_MODEL_MAPPINGS {
            return Ok(None);
        }
        let mut grouped = BTreeMap::<String, Vec<_>>::new();
        for row in rows {
            grouped
                .entry(row.global_model_name.clone())
                .or_default()
                .push(row);
        }
        if grouped.len() > MAX_MODELS {
            return Ok(None);
        }
        let mut models = Map::new();
        for (alias, rows) in grouped {
            if contains_dynamic_reference(&alias) {
                continue;
            }
            let model = state.data.get_public_global_model_by_name(&alias).await?;
            // An installed public reader returns only active models. A missing
            // record can mean disable/delete since the candidate snapshot.
            if state.has_global_model_data_reader() && model.is_none() {
                continue;
            }
            if model.as_ref().is_some_and(|model| {
                !model.is_active
                    || model
                        .display_name
                        .as_deref()
                        .is_some_and(contains_dynamic_reference)
            }) {
                continue;
            }
            let Some(global_model_id) = model
                .as_ref()
                .map(|model| model.id.as_str())
                .or_else(|| rows.first().map(|row| row.global_model_id.as_str()))
            else {
                continue;
            };
            // One read per alias, rather than a serial query for every provider/key.
            // Only authorized candidate mappings below contribute to public limits.
            let mappings = state
                .data
                .list_admin_provider_models_by_global_model_id(global_model_id)
                .await?
                .into_iter()
                .filter(|mapping| mapping.global_model_id == global_model_id)
                .map(|mapping| ((mapping.provider_id.clone(), mapping.id.clone()), mapping))
                .collect::<BTreeMap<_, _>>();
            let mut limits = Vec::new();
            for row in &rows {
                if row.global_model_id != global_model_id {
                    continue;
                }
                let mapping = mappings.get(&(row.provider_id.clone(), row.model_id.clone()));
                if mapping.is_some_and(|mapping| !mapping.is_active || !mapping.is_available) {
                    continue;
                }
                // A missing mapping is not evidence that there is no override.
                // Keep the alias, but omit advertised limits instead of overpromising.
                limits.push(mapping.and_then(|mapping| {
                    effective_token_limits(
                        model.as_ref().and_then(|model| model.config.as_ref()),
                        mapping.config.as_ref(),
                    )
                }));
            }
            if limits.is_empty() {
                continue;
            }
            let mut card = project_model(&alias, model.as_ref());
            card.as_object_mut()
                .expect("model card is an object")
                .remove("limit");
            if let Some((context, output)) = conservative_token_limits(&limits) {
                card["limit"] = json!({"context": context, "output": output});
            }
            models.insert(alias, card);
        }
        let payload = config(models, base_url);
        Ok::<_, aether_data_contracts::DataLayerError>(
            serde_json::to_vec(&payload)
                .ok()
                .filter(|bytes| bytes.len() <= MAX_RESPONSE_BYTES)
                .map(|_| payload),
        )
    })
    .await
    .flatten();
    match payload {
        Some(payload) => Json(payload).into_response(),
        None => unavailable_response(),
    }
}

fn nonnegative_number(value: &Value) -> Option<Value> {
    value
        .as_f64()
        .filter(|number| number.is_finite() && *number >= 0.0)
        .map(|_| value.clone())
}

fn positive_token_limit(value: &Value) -> Option<u64> {
    value
        .as_u64()
        .filter(|number| *number > 0 && *number <= 9_007_199_254_740_991)
}

// Only absent fields inherit. An explicit invalid override is unknown, not a
// reason to advertise the potentially larger global value.
fn effective_token_limits(global: Option<&Value>, mapping: Option<&Value>) -> Option<(u64, u64)> {
    let field = |name: &str| {
        mapping
            .and_then(|value| value.get(name))
            .or_else(|| global.and_then(|value| value.get(name)))
            .and_then(positive_token_limit)
    };
    let context = field("context_limit")?;
    let output = field("output_limit")?;
    (output <= context).then_some((context, output))
}

fn conservative_token_limits(limits: &[Option<(u64, u64)>]) -> Option<(u64, u64)> {
    let mut result: Option<(u64, u64)> = None;
    for limit in limits {
        let (context, output) = (*limit)?;
        result = Some(match result {
            Some((prior_context, prior_output)) => {
                (context.min(prior_context), output.min(prior_output))
            }
            None => (context, output),
        });
    }
    result
}

fn contains_dynamic_reference(value: &str) -> bool {
    ["{env:", "{file:", "${"]
        .iter()
        .any(|marker| value.contains(marker))
}

fn modalities(value: &Value) -> Option<Value> {
    let values = value.as_array()?;
    // A partial list would incorrectly describe the model's complete modality set.
    if values.iter().any(|value| {
        !matches!(
            value.as_str(),
            Some("text" | "image" | "audio" | "video" | "pdf")
        )
    }) {
        return None;
    }
    (!values.is_empty()).then(|| value.clone())
}

fn project_model(alias: &str, model: Option<&StoredPublicGlobalModel>) -> Value {
    let mut card = json!({"id": alias, "name": alias});
    let Some(model) = model else { return card };
    if let Some(name) = model
        .display_name
        .as_deref()
        .filter(|name| !name.trim().is_empty())
    {
        card["name"] = json!(name);
    }
    let config = model.config.as_ref().and_then(Value::as_object);
    let has_capability = |name: &str| {
        model
            .supported_capabilities
            .as_ref()
            .and_then(Value::as_array)
            .is_some_and(|values| values.iter().any(|value| value.as_str() == Some(name)))
    };
    for (source, target) in [
        ("function_calling", "tool_call"),
        ("extended_thinking", "reasoning"),
        ("attachment", "attachment"),
        ("temperature", "temperature"),
    ] {
        if let Some(value) = config
            .and_then(|config| config.get(source))
            .and_then(Value::as_bool)
            .or_else(|| has_capability(source).then_some(true))
        {
            card[target] = json!(value);
        }
    }
    if let Some(config) = config {
        for field in ["family", "release_date"] {
            if let Some(value) = config
                .get(field)
                .and_then(Value::as_str)
                .filter(|value| !value.is_empty() && !contains_dynamic_reference(value))
            {
                card[field] = json!(value);
            }
        }
        if let (Some(context), Some(output)) = (
            config.get("context_limit").and_then(positive_token_limit),
            config.get("output_limit").and_then(positive_token_limit),
        ) {
            card["limit"] = json!({"context": context, "output": output});
        }
        for (source, target) in [
            ("input_modalities", "input"),
            ("output_modalities", "output"),
        ] {
            if let Some(value) = config.get(source).and_then(modalities) {
                card["modalities"][target] = value;
            }
        }
    }
    // Vision establishes image input for this text-chat provider, not general
    // attachment support. Prefer the explicitly configured modality list.
    if card
        .get("modalities")
        .and_then(|value| value.get("input"))
        .is_none()
        && config
            .and_then(|config| config.get("vision"))
            .and_then(Value::as_bool)
            .or_else(|| has_capability("vision").then_some(true))
            == Some(true)
    {
        card["modalities"]["input"] = json!(["text", "image"]);
    }
    if let Some(cost) = project_cost(model) {
        card["cost"] = cost;
    }
    card
}

fn project_cost(model: &StoredPublicGlobalModel) -> Option<Value> {
    if model
        .default_price_per_request
        .is_some_and(|price| price != 0.0)
    {
        return None;
    }
    let tiers = model
        .default_tiered_pricing
        .as_ref()?
        .get("tiers")?
        .as_array()?;
    // A single unbounded token tier has the same USD / million tokens units as
    // OpenCode. Do not flatten variable tiers or substitute missing prices.
    if tiers.len() != 1 || tiers[0].get("up_to") != Some(&Value::Null) {
        return None;
    }
    let tier = &tiers[0];
    let mut cost = json!({
        "input": nonnegative_number(tier.get("input_price_per_1m")?)?,
        "output": nonnegative_number(tier.get("output_price_per_1m")?)?,
    });
    for (source, target) in [
        ("cache_read_price_per_1m", "cache_read"),
        ("cache_creation_price_per_1m", "cache_write"),
    ] {
        if let Some(value) = tier.get(source).and_then(nonnegative_number) {
            cost[target] = value;
        }
    }
    Some(cost)
}

#[cfg(test)]
#[path = "opencode/tests.rs"]
mod tests;
