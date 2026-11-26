package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type arcaRequestDiagnosticStage struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Code     string   `json:"code"`
	Evidence []string `json:"evidence"`
}

type arcaRequestDiagnostic struct {
	SchemaVersion      int                          `json:"schema_version"`
	RequestID          string                       `json:"request_id"`
	State              string                       `json:"state"`
	FinalOutcome       string                       `json:"final_outcome"`
	HasAttemptFailures bool                         `json:"has_attempt_failures"`
	Stages             []arcaRequestDiagnosticStage `json:"stages"`
	Summary            string                       `json:"summary"`
}

func GetArcaRequestDiagnostics(c *gin.Context) {
	requestID := c.Param("request_id")
	if c.Request.URL.RawQuery != "" || !validArcaDiagnosticRequestID(requestID) {
		arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid parameters", "ARCA_DIAGNOSTIC_INVALID")
		return
	}
	evidence, err := model.ReadArcaRequestDiagnosticEvidence(c.Request.Context(), c.GetInt("id"), requestID)
	if err != nil {
		arcaFeedbackHTTPError(c, http.StatusServiceUnavailable, "Database error, please contact the administrator", "ARCA_DIAGNOSTIC_UNAVAILABLE")
		return
	}
	common.ApiSuccess(c, buildArcaRequestDiagnostic(requestID, evidence))
}

func validArcaDiagnosticRequestID(requestID string) bool {
	if len(requestID) == 0 || len(requestID) > 64 || requestID == "." || requestID == ".." {
		return false
	}
	for i := 0; i < len(requestID); i++ {
		ch := requestID[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || strings.ContainsRune("-_.", rune(ch)) {
			continue
		}
		return false
	}
	return true
}

func buildArcaRequestDiagnostic(requestID string, evidence model.ArcaRequestDiagnosticEvidence) arcaRequestDiagnostic {
	stages := make([]arcaRequestDiagnosticStage, 0, 8)
	for _, id := range []string{"identity", "model_acl", "quota", "candidate_filter", "conversion", "tunnel", "upstream", "stream"} {
		stage := arcaRequestDiagnosticStage{ID: id, Status: "unknown", Code: "not_recorded", Evidence: []string{}}
		switch {
		case id == "identity" && (evidence.HasUsageLog || evidence.HasErrorLog):
			stage.Status, stage.Code, stage.Evidence = "known", "owner_recorded", []string{"logs.user_id"}
		case id == "candidate_filter" && evidence.HasChannelAttempt:
			stage.Status, stage.Code, stage.Evidence = "known", "channel_attempt_recorded", []string{"logs.channel_id"}
		case id == "stream" && evidence.IsStream:
			stage.Status, stage.Code, stage.Evidence = "known", "stream_recorded", []string{"logs.is_stream"}
		}
		stages = append(stages, stage)
	}

	// A consume log may be written for an interrupted stream, and an error log
	// may describe an attempt followed by a retry. Neither proves a final outcome.
	var summary strings.Builder
	fmt.Fprintf(&summary, "RequestID: %s\nDiagnostic v1: unknown\nUsage log observed: %t\nError log observed: %t\nChannel attempt errors observed: %t",
		requestID, evidence.HasUsageLog, evidence.HasErrorLog, evidence.HasAttemptFailures)
	for _, stage := range stages {
		fmt.Fprintf(&summary, "\n%s: %s (%s)", stage.ID, stage.Status, stage.Code)
	}
	return arcaRequestDiagnostic{
		SchemaVersion:      1,
		RequestID:          requestID,
		State:              "unknown",
		FinalOutcome:       "unknown",
		HasAttemptFailures: evidence.HasAttemptFailures,
		Stages:             stages,
		Summary:            summary.String(),
	}
}
