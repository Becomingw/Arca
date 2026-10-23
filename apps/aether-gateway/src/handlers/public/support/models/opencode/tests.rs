use super::*;

fn model(config: Value) -> StoredPublicGlobalModel {
    StoredPublicGlobalModel {
        id: "internal-id".into(),
        name: "public-alias".into(),
        display_name: Some("Public Model".into()),
        is_active: true,
        default_price_per_request: None,
        default_tiered_pricing: Some(json!({"tiers": [{"up_to": null,
            "input_price_per_1m": 2.0, "output_price_per_1m": 8.0,
            "cache_read_price_per_1m": 0.0, "cache_creation_price_per_1m": 3.0}]})),
        supported_capabilities: None,
        config: Some(config),
        usage_count: 0,
    }
}

#[test]
fn opencode_projects_only_known_schema_fields() {
    let model = model(json!({
        "context_limit": 200000, "output_limit": 16000,
        "function_calling": true, "vision": true, "extended_thinking": false,
        "attachment": true, "temperature": false,
        "input_modalities": ["text", "image", "pdf"], "output_modalities": ["text"],
        "family": "public-family", "release_date": "2026-01-01",
        "apiKey": "secret-canary", "provider_model_name": "upstream-canary",
        "options": {"headers": {"Authorization": "secret-canary"}}
    }));
    assert_eq!(
        project_model("public-alias", Some(&model)),
        json!({
            "id": "public-alias", "name": "Public Model", "tool_call": true,
            "reasoning": false, "attachment": true, "temperature": false,
            "family": "public-family", "release_date": "2026-01-01",
            "limit": {"context": 200000, "output": 16000},
            "modalities": {"input": ["text", "image", "pdf"], "output": ["text"]},
            "cost": {"input": 2.0, "output": 8.0, "cache_read": 0.0, "cache_write": 3.0}
        })
    );
}

#[test]
fn opencode_does_not_infer_metadata_from_model_names_or_missing_values() {
    assert_eq!(
        project_model("gpt-vision-thinking-unknown", None),
        json!({
            "id": "gpt-vision-thinking-unknown", "name": "gpt-vision-thinking-unknown"
        })
    );
    let mut model = model(json!({"context_limit": 100000, "function_calling": "true",
        "input_modalities": ["text", "secret-canary"], "output_limit": 0,
        "family": "{env:SECRET}"}));
    model.default_tiered_pricing = None;
    assert_eq!(
        project_model("public-alias", Some(&model)),
        json!({"id": "public-alias", "name": "Public Model"})
    );
    model.config = Some(json!({"context_limit": -1, "output_limit": 10}));
    assert!(project_model("public-alias", Some(&model))
        .get("limit")
        .is_none());
}

#[test]
fn opencode_keeps_unknown_capabilities_absent_and_respects_explicit_false() {
    let mut model = model(json!({"function_calling": false, "vision": false}));
    model.supported_capabilities = Some(json!(["function_calling", "vision", "extended_thinking"]));
    let card = project_model("public-alias", Some(&model));
    assert_eq!(card["tool_call"], false);
    assert_eq!(card["reasoning"], true);
    assert!(card.get("attachment").is_none());
    assert!(card.get("modalities").is_none());
}

#[test]
fn opencode_omits_cost_that_cannot_be_faithfully_expressed() {
    for pricing in [
        json!({"tiers": [{"up_to": null, "input_price_per_1m": 1.0}]}),
        json!({"tiers": [{"up_to": null, "input_price_per_1m": -1.0, "output_price_per_1m": 2.0}]}),
        json!({"tiers": [{"up_to": 1000, "input_price_per_1m": 1.0, "output_price_per_1m": 2.0}]}),
        json!({"tiers": [{"up_to": 1000, "input_price_per_1m": 1.0, "output_price_per_1m": 2.0},
            {"up_to": null, "input_price_per_1m": 2.0, "output_price_per_1m": 4.0}]}),
    ] {
        let mut model = model(json!({}));
        model.default_tiered_pricing = Some(pricing);
        assert!(project_cost(&model).is_none());
    }
    let mut model = model(json!({}));
    model.default_price_per_request = Some(0.01);
    assert!(project_cost(&model).is_none());
}

#[test]
fn opencode_rejects_local_dynamic_references() {
    for value in ["{env:SECRET}", "public-{file:/etc/passwd}", "Model ${HOME}"] {
        assert!(contains_dynamic_reference(value));
    }
    assert!(!contains_dynamic_reference("Public Model"));
    assert_eq!(
        config(Map::new(), "https://gateway.example/")["provider"]["aether"]["options"],
        json!({"baseURL": "https://gateway.example/v1"})
    );
}

#[test]
fn opencode_provider_limits_override_fields_and_inherit_missing_fields() {
    let global = json!({"context_limit": 200000, "output_limit": 16000});
    assert_eq!(
        effective_token_limits(Some(&global), None),
        Some((200000, 16000))
    );
    assert_eq!(
        effective_token_limits(Some(&global), Some(&json!({"context_limit": 64000}))),
        Some((64000, 16000))
    );
    assert_eq!(
        effective_token_limits(Some(&global), Some(&json!({"output_limit": 4000}))),
        Some((200000, 4000))
    );
    assert_eq!(
        effective_token_limits(
            None,
            Some(&json!({"context_limit": 64000, "output_limit": 4000}))
        ),
        Some((64000, 4000))
    );
}

#[test]
fn opencode_provider_limits_take_minimum_without_guessing_unknowns() {
    assert_eq!(
        conservative_token_limits(&[
            Some((200000, 16000)),
            Some((64000, 8000)),
            Some((100000, 4000))
        ]),
        Some((64000, 4000))
    );
    assert_eq!(
        conservative_token_limits(&[Some((64000, 4000)), None]),
        None
    );
    assert_eq!(conservative_token_limits(&[]), None);
}

#[test]
fn opencode_provider_invalid_overrides_do_not_fall_back_to_larger_global_limit() {
    let global = json!({"context_limit": 200000, "output_limit": 16000});
    for invalid in [
        json!(null),
        json!(0),
        json!(-1),
        json!(1.5),
        json!("64000"),
        json!(9_007_199_254_740_992_u64),
    ] {
        assert_eq!(
            effective_token_limits(Some(&global), Some(&json!({"context_limit": invalid}))),
            None
        );
    }
    assert_eq!(
        effective_token_limits(Some(&global), Some(&json!({"context_limit": 1000}))),
        None
    );
    assert_eq!(
        effective_token_limits(Some(&json!({"context_limit": 64000})), None),
        None
    );
}
