use axum::body::Body;
use axum::http::{Response, StatusCode};
use axum::response::IntoResponse;
use axum::Json;
use serde_json::json;
use sha2::{Digest, Sha256};
use tracing::warn;

use crate::ai_serving::AiSurfaceFinalizeError;
use crate::constants::*;
use crate::insert_header_if_missing;

#[derive(Debug, Clone)]
pub(crate) enum GatewayError {
    UpstreamUnavailable {
        trace_id: String,
        message: String,
    },
    ControlUnavailable {
        trace_id: String,
        message: String,
    },
    LocalExecutionPlanningTimeout {
        trace_id: String,
        phase: &'static str,
        timeout_ms: u64,
    },
    AdmissionTimeout {
        trace_id: String,
        gate: &'static str,
        queue_budget_ms: u64,
    },
    Client {
        status: StatusCode,
        message: String,
    },
    PlanUsageLimited(crate::plan_usage_policy::PlanUsagePolicyRejection),
    LastActiveAdminUpdateDenied,
    LastActiveAdminDeleteDenied,
    Internal(String),
}

impl GatewayError {
    pub(crate) fn into_message(self) -> String {
        match self {
            Self::UpstreamUnavailable { message, .. }
            | Self::ControlUnavailable { message, .. }
            | Self::Client { message, .. }
            | Self::Internal(message) => message,
            Self::PlanUsageLimited(rejection) => format!(
                "subscription plan {} limit {} reached for {} window; retry after {} seconds",
                rejection.metric, rejection.limit, rejection.window, rejection.retry_after
            ),
            Self::LocalExecutionPlanningTimeout {
                phase, timeout_ms, ..
            } => {
                format!("local execution planning timed out in {phase} after {timeout_ms}ms")
            }
            Self::AdmissionTimeout {
                gate,
                queue_budget_ms,
                ..
            } => {
                format!("gateway admission gate {gate} timed out after {queue_budget_ms}ms")
            }
            Self::LastActiveAdminUpdateDenied => "不能降级或停用最后一个管理员账户".to_string(),
            Self::LastActiveAdminDeleteDenied => "不能删除最后一个管理员账户".to_string(),
        }
    }
}

impl IntoResponse for GatewayError {
    fn into_response(self) -> Response<Body> {
        match self {
            Self::UpstreamUnavailable { trace_id, message } => {
                let error_fingerprint = gateway_error_fingerprint(&message);
                warn!(
                    trace_id = %trace_id,
                    error_fingerprint,
                    error_length = message.len(),
                    "gateway proxy unavailable"
                );
                let body = Json(json!({
                    "error": {
                        "message": "gateway proxy unavailable",
                        "trace_id": trace_id,
                    }
                }));
                let mut response = (StatusCode::BAD_GATEWAY, body).into_response();
                let _ =
                    insert_header_if_missing(response.headers_mut(), TRACE_ID_HEADER, &trace_id);
                let _ = insert_header_if_missing(
                    response.headers_mut(),
                    GATEWAY_HEADER,
                    "rust-phase3b",
                );
                response
            }
            Self::ControlUnavailable { trace_id, message } => {
                let error_fingerprint = gateway_error_fingerprint(&message);
                warn!(
                    trace_id = %trace_id,
                    error_fingerprint,
                    error_length = message.len(),
                    "gateway control unavailable"
                );
                let body = Json(json!({
                    "error": {
                        "message": "gateway control unavailable",
                        "trace_id": trace_id,
                    }
                }));
                let mut response = (StatusCode::BAD_GATEWAY, body).into_response();
                let _ =
                    insert_header_if_missing(response.headers_mut(), TRACE_ID_HEADER, &trace_id);
                let _ = insert_header_if_missing(
                    response.headers_mut(),
                    GATEWAY_HEADER,
                    "rust-phase3b",
                );
                response
            }
            Self::LocalExecutionPlanningTimeout {
                trace_id,
                phase,
                timeout_ms,
            } => {
                warn!(
                    trace_id = %trace_id,
                    phase,
                    timeout_ms,
                    "gateway local execution planning timed out"
                );
                let body = Json(json!({
                    "error": {
                        "message": "gateway local execution planning timed out",
                        "trace_id": trace_id,
                    }
                }));
                let mut response = (StatusCode::GATEWAY_TIMEOUT, body).into_response();
                let _ =
                    insert_header_if_missing(response.headers_mut(), TRACE_ID_HEADER, &trace_id);
                let _ = insert_header_if_missing(
                    response.headers_mut(),
                    GATEWAY_HEADER,
                    "rust-phase3b",
                );
                response
            }
            Self::AdmissionTimeout {
                trace_id,
                gate,
                queue_budget_ms,
            } => {
                tracing::debug!(
                    trace_id = %trace_id,
                    gate,
                    queue_budget_ms,
                    "gateway admission gate timed out"
                );
                let body = Json(json!({
                    "error": {
                        "message": "gateway admission queue timed out",
                        "trace_id": trace_id,
                    }
                }));
                let mut response = (StatusCode::TOO_MANY_REQUESTS, body).into_response();
                let _ =
                    insert_header_if_missing(response.headers_mut(), TRACE_ID_HEADER, &trace_id);
                let _ = insert_header_if_missing(
                    response.headers_mut(),
                    GATEWAY_HEADER,
                    "rust-phase3b",
                );
                let _ = insert_header_if_missing(response.headers_mut(), "Retry-After", "1");
                response
            }
            Self::Client { status, message } => (
                status,
                Json(json!({
                    "error": {
                        "message": message,
                    }
                })),
            )
                .into_response(),
            Self::PlanUsageLimited(rejection) => (
                StatusCode::TOO_MANY_REQUESTS,
                Json(json!({
                    "error": {
                        "type": "plan_usage_limit_exceeded",
                        "message": "使用额度已达到上限，请稍后重试",
                        "details": {
                            "metric": rejection.metric,
                "used": rejection.used,
                "estimated": rejection.estimated,
                            "window": rejection.window,
                            "limit": rejection.limit,
                            "retry_after": rejection.retry_after,
                        }
                    }
                })),
            )
                .into_response(),
            Self::LastActiveAdminUpdateDenied => (
                StatusCode::BAD_REQUEST,
                Json(json!({ "detail": "不能降级或停用最后一个管理员账户" })),
            )
                .into_response(),
            Self::LastActiveAdminDeleteDenied => (
                StatusCode::BAD_REQUEST,
                Json(json!({ "detail": "不能删除最后一个管理员账户" })),
            )
                .into_response(),
            Self::Internal(message) => {
                let error_fingerprint = gateway_error_fingerprint(&message);
                tracing::error!(
                    event_name = "gateway_internal_error",
                    error_fingerprint,
                    error_length = message.len(),
                    "internal gateway error hidden from client"
                );
                (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    Json(json!({
                        "error": {
                            "message": "internal server error",
                        }
                    })),
                )
                    .into_response()
            }
        }
    }
}

fn gateway_error_fingerprint(message: &str) -> String {
    let digest = Sha256::digest(message.as_bytes());
    format!("{:x}", digest)[..16].to_string()
}

impl From<AiSurfaceFinalizeError> for GatewayError {
    fn from(error: AiSurfaceFinalizeError) -> Self {
        GatewayError::Internal(error.0)
    }
}

#[cfg(test)]
mod tests {
    use axum::body::to_bytes;
    use axum::http::{header::RETRY_AFTER, StatusCode};
    use axum::response::IntoResponse;

    use crate::constants::TRACE_ID_HEADER;

    use super::{gateway_error_fingerprint, GatewayError};

    #[tokio::test]
    async fn internal_errors_do_not_expose_internal_details() {
        let response = GatewayError::Internal(
            "database connection failed: password=internal-secret".to_string(),
        )
        .into_response();

        assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
        let body = to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("internal error response body should read");
        let payload: serde_json::Value =
            serde_json::from_slice(&body).expect("internal error response should be JSON");
        assert_eq!(payload["error"]["message"], "internal server error");
        assert!(!String::from_utf8_lossy(&body).contains("internal-secret"));
    }

    #[test]
    fn error_fingerprints_are_stable_without_containing_source_text() {
        let secret_error = "postgresql://admin:database-secret@db.internal/aether";
        let fingerprint = gateway_error_fingerprint(secret_error);

        assert_eq!(fingerprint, gateway_error_fingerprint(secret_error));
        assert_eq!(fingerprint.len(), 16);
        assert!(fingerprint.bytes().all(|byte| byte.is_ascii_hexdigit()));
        assert!(!fingerprint.contains("database-secret"));
    }

    #[test]
    fn admission_timeout_returns_429_with_retry_after_without_panicking() {
        let trace_id = "trace-admission-timeout".to_string();

        let response = GatewayError::AdmissionTimeout {
            trace_id: trace_id.clone(),
            gate: "gateway_upstream_execution",
            queue_budget_ms: 250,
        }
        .into_response();

        assert_eq!(response.status(), StatusCode::TOO_MANY_REQUESTS);
        assert_eq!(
            response
                .headers()
                .get(RETRY_AFTER)
                .and_then(|v| v.to_str().ok()),
            Some("1")
        );
        assert_eq!(
            response
                .headers()
                .get(TRACE_ID_HEADER)
                .and_then(|v| v.to_str().ok()),
            Some(trace_id.as_str())
        );
    }
}
