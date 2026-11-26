package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	arcaFeedbackWriteLimit = 64 * 1024
	arcaFeedbackTextLimit  = 20_000
)

type arcaFeedbackCreateInput struct {
	Title              string  `json:"title"`
	Description        string  `json:"description"`
	Category           string  `json:"category"`
	Model              *string `json:"model"`
	Visibility         string  `json:"visibility"`
	ClientRequestID    string  `json:"client_request_id"`
	IncludeDiagnostics bool    `json:"include_diagnostics"`
	RequestID          *string `json:"request_id,omitempty"`
}

type arcaFeedbackCommandInput struct {
	Action              string  `json:"action"`
	ExpectedVersion     int64   `json:"expected_version"`
	ClientRequestID     string  `json:"client_request_id"`
	Visibility          string  `json:"visibility"`
	Content             *string `json:"content"`
	Status              *string `json:"status"`
	Comment             *string `json:"comment"`
	Pinned              *bool   `json:"pinned,omitempty"`
	Title               *string `json:"title,omitempty"`
	Description         *string `json:"description,omitempty"`
	Category            *string `json:"category,omitempty"`
	Seq                 *int64  `json:"seq,omitempty"`
	DiagnosticRequestID *string `json:"diagnostic_request_id,omitempty"`
}

func ListArcaFeedback(c *gin.Context) {
	filter := model.ArcaFeedbackFilter{
		Category: c.Query("category"),
		Status:   c.Query("status"),
		Search:   strings.TrimSpace(c.Query("search")),
	}
	limit, limitOK := arcaFeedbackQueryInt(c, "limit", 20, 1, 100)
	skip, skipOK := arcaFeedbackQueryInt(c, "skip", 0, 0, 1_000_000)
	if !limitOK || !skipOK || c.Query("owner_id") != "" || c.Query("after") != "" ||
		(filter.Category != "" && !isArcaFeedbackCategory(filter.Category)) ||
		(filter.Status != "" && !isArcaFeedbackStatus(filter.Status)) || (utf8.RuneCountInString(filter.Search) > 200 || strings.ContainsRune(filter.Search, 0)) {
		arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid parameters", "ARCA_FEEDBACK_INVALID")
		return
	}
	filter.Limit, filter.Skip = int(limit), int(skip)
	page, err := model.ListArcaFeedback(c.Request.Context(), filter)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, page)
}

func GetArcaFeedback(c *gin.Context) {
	limit, limitOK := arcaFeedbackQueryInt(c, "limit", 20, 1, 100)
	after, afterOK := arcaFeedbackQueryInt(c, "after_event", 0, 0, 1<<63-1)
	if !limitOK || !afterOK {
		arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid parameters", "ARCA_FEEDBACK_INVALID")
		return
	}
	id, err := arcaFeedbackIssueID(c.Request.Context(), c.Param("id"))
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	detail, err := model.GetArcaFeedbackDetail(c.Request.Context(), id, after, int(limit))
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func CreateArcaFeedback(c *gin.Context) {
	var input arcaFeedbackCreateInput
	if err := decodeArcaFeedbackInput(c, &input, "title", "description", "category", "model", "visibility", "client_request_id", "include_diagnostics", "request_id"); err != nil {
		arcaFeedbackInputError(c, err)
		return
	}
	mutation, err := createArcaFeedback(c.Request.Context(), c.GetInt("id"), c.GetInt("role"), input, false)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	if !mutation.Replayed {
		model.RecordAuditLog(c, model.AuditLog{UserId: c.GetInt("id"), ActorRole: c.GetInt("role"),
			Category: model.AuditCategoryOperation, Action: "arca_feedback.create", Success: true,
			Other: model.AuditOther{Op: &model.AuditOperation{Action: "arca_feedback.create", Params: model.AuditFields{"issue_id": mutation.Issue.ID}}},
		})
	}
	common.ApiSuccess(c, mutation)
}

func createArcaFeedback(ctx context.Context, actorID, actorRole int, input arcaFeedbackCreateInput, adminDiagnostics bool) (model.ArcaFeedbackMutation, error) {
	key, validKey := arcaFeedbackUUID(input.ClientRequestID)
	input.Title = strings.TrimSpace(input.Title)
	if !validKey || !isArcaFeedbackTitle(input.Title) || !isArcaFeedbackText(input.Description) ||
		!isArcaFeedbackCategory(input.Category) ||
		(input.Model != nil && (utf8.RuneCountInString(*input.Model) > 200 || strings.ContainsRune(*input.Model, 0))) ||
		(input.Visibility != "" && input.Visibility != model.ArcaFeedbackPublic) ||
		(input.RequestID == nil && input.IncludeDiagnostics) ||
		(input.RequestID != nil && !validArcaDiagnosticRequestID(*input.RequestID)) {
		return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
	}
	input.ClientRequestID, input.Visibility = key, model.ArcaFeedbackPublic
	hash, err := arcaFeedbackInputHash(input)
	if err != nil {
		return model.ArcaFeedbackMutation{}, err
	}
	if input.RequestID != nil {
		evidence, err := readArcaFeedbackDiagnostic(ctx, actorID, *input.RequestID, adminDiagnostics)
		if err != nil {
			// A published snapshot belongs to this actor and client input.
			// A log outage or retention expiry must not hide a successful write.
			replay, lookupErr := model.FindArcaFeedbackCreation(ctx, actorID, key, hash)
			if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return replay, lookupErr
			}
			return model.ArcaFeedbackMutation{}, err
		}
		if input.IncludeDiagnostics {
			input.Description += "\n\n" + buildArcaRequestDiagnostic(*input.RequestID, evidence).Summary
			if len(input.Description) > arcaFeedbackTextLimit {
				return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
			}
		}
	}
	now := common.GetTimestamp()
	issue := model.ArcaFeedbackIssue{
		ID: uuid.NewString(), OwnerID: &actorID, Title: input.Title, Description: input.Description,
		Category: input.Category, Model: input.Model, RequestID: input.RequestID, Status: "open", Visibility: model.ArcaFeedbackPublic,
		Version: 1, CreationKey: key, CreationHash: hash, CreatedAt: now, UpdatedAt: now,
	}
	return model.CreateArcaFeedback(ctx, issue, actorRole)
}

func readArcaFeedbackDiagnostic(ctx context.Context, actorID int, requestID string, admin bool) (model.ArcaRequestDiagnosticEvidence, error) {
	var evidence model.ArcaRequestDiagnosticEvidence
	var err error
	if admin {
		evidence, err = model.ReadArcaAdminRequestDiagnosticEvidence(ctx, requestID)
	} else {
		evidence, err = model.ReadArcaRequestDiagnosticEvidence(ctx, actorID, requestID)
	}
	if err != nil {
		return evidence, errArcaDiagnosticUnavailable
	}
	if !evidence.HasUsageLog && !evidence.HasErrorLog {
		return evidence, errArcaDiagnosticMissing
	}
	return evidence, nil
}

func CommandArcaFeedback(c *gin.Context) {
	id, err := arcaFeedbackIssueID(c.Request.Context(), c.Param("id"))
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	var input arcaFeedbackCommandInput
	if err := decodeArcaFeedbackInput(c, &input, "action", "expected_version", "client_request_id", "visibility", "content", "status", "comment", "pinned", "title", "description", "category", "seq", "diagnostic_request_id"); err != nil {
		arcaFeedbackInputError(c, err)
		return
	}
	mutation, err := commandArcaFeedback(c.Request.Context(), c.GetInt("id"), c.GetInt("role"), id, input, false)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	if !mutation.Replayed {
		action := "arca_feedback." + input.Action
		model.RecordAuditLog(c, model.AuditLog{UserId: c.GetInt("id"), ActorRole: c.GetInt("role"),
			Category: model.AuditCategoryOperation, Action: action, Success: true,
			Other: model.AuditOther{Op: &model.AuditOperation{Action: action, Params: model.AuditFields{"issue_id": id}}},
		})
	}
	common.ApiSuccess(c, mutation)
}

func commandArcaFeedback(ctx context.Context, actorID, actorRole int, id string, input arcaFeedbackCommandInput, adminDiagnostics bool) (model.ArcaFeedbackMutation, error) {
	key, validKey := arcaFeedbackUUID(input.ClientRequestID)
	if !validKey || input.ExpectedVersion < 1 || (input.Visibility != "" && input.Visibility != model.ArcaFeedbackPublic) {
		return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
	}
	input.ClientRequestID, input.Visibility = key, model.ArcaFeedbackPublic
	event := model.ArcaFeedbackEvent{
		IssueID: id, ActorID: &actorID, Visibility: model.ArcaFeedbackPublic,
		ClientRequestID: &key, CreatedAt: common.GetTimestamp(),
	}
	hasIssueEdits := input.Title != nil || input.Description != nil || input.Category != nil
	if (input.Action != "edit_issue" && hasIssueEdits) ||
		(input.Action != "set_pinned" && input.Pinned != nil) ||
		(input.Action != "edit_reply" && input.Action != "delete_reply" && input.Seq != nil) ||
		(input.Action != "reply" && input.Action != "edit_reply" && input.Content != nil) ||
		(input.Action != "set_status" && (input.Status != nil || input.Comment != nil)) ||
		(input.Action != "reply" && input.DiagnosticRequestID != nil) {
		return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
	}
	switch input.Action {
	case "reply":
		if input.Content == nil || !isArcaFeedbackText(*input.Content) ||
			(input.DiagnosticRequestID != nil && !validArcaDiagnosticRequestID(*input.DiagnosticRequestID)) {
			return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
		}
		event.Kind, event.Content = "reply", input.Content
	case "set_status":
		if input.Status == nil || !isArcaFeedbackStatus(*input.Status) ||
			(input.Comment != nil && (len(*input.Comment) > arcaFeedbackTextLimit || strings.ContainsRune(*input.Comment, 0))) ||
			(*input.Status == "resolved" && (input.Comment == nil || !isArcaFeedbackText(*input.Comment))) {
			return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
		}
		event.Kind, event.Content, event.ToStatus = "status", input.Comment, input.Status
	case "set_pinned":
		if input.Pinned == nil {
			return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
		}
		event.Kind = "pin"
	case "edit_issue":
		if input.Title != nil {
			input.Title = new(strings.TrimSpace(*input.Title))
		}
		if !hasIssueEdits || (input.Title != nil && !isArcaFeedbackTitle(*input.Title)) ||
			(input.Description != nil && !isArcaFeedbackText(*input.Description)) ||
			(input.Category != nil && !isArcaFeedbackCategory(*input.Category)) {
			return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
		}
		event.Kind = "issue_edit"
	case "delete_issue":
		event.Kind = "issue_delete"
	case "edit_reply", "delete_reply":
		if input.Seq == nil || *input.Seq <= 0 || (input.Action == "edit_reply" && (input.Content == nil || !isArcaFeedbackText(*input.Content))) {
			return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
		}
		event.TargetSeq = input.Seq
		if input.Action == "edit_reply" {
			event.Kind = "reply_edit"
		} else {
			event.Kind = "reply_delete"
		}
	default:
		return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
	}

	hash, err := arcaFeedbackInputHash(input)
	if err != nil {
		return model.ArcaFeedbackMutation{}, err
	}
	event.PayloadHash = hash
	if input.DiagnosticRequestID != nil {
		evidence, err := readArcaFeedbackDiagnostic(ctx, actorID, *input.DiagnosticRequestID, adminDiagnostics)
		if err != nil {
			replay, lookupErr := model.FindArcaFeedbackCommandReplay(ctx, event, actorRole)
			if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return replay, lookupErr
			}
			return model.ArcaFeedbackMutation{}, err
		}
		content := *input.Content + "\n\n" + buildArcaRequestDiagnostic(*input.DiagnosticRequestID, evidence).Summary
		if !isArcaFeedbackText(content) {
			return model.ArcaFeedbackMutation{}, errArcaFeedbackInput
		}
		event.Content = &content
	}
	switch event.Kind {
	case "reply", "status":
		return model.ApplyArcaFeedbackCommand(ctx, event, input.ExpectedVersion, actorRole)
	default:
		edit := model.ArcaFeedbackEdit{
			Pinned: input.Pinned, Title: input.Title, Description: input.Description,
			Category: input.Category, Reply: input.Content,
		}
		return model.ApplyArcaFeedbackEdit(ctx, event, input.ExpectedVersion, actorRole, edit)
	}
}

func arcaFeedbackIssueID(ctx context.Context, value string) (string, error) {
	if id, ok := arcaFeedbackUUID(value); ok {
		return id, nil
	}
	seq, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seq <= 0 || strconv.FormatInt(seq, 10) != value {
		return "", gorm.ErrRecordNotFound
	}
	return model.ResolveArcaFeedbackIssueNumber(ctx, seq)
}

func decodeArcaFeedbackInput(c *gin.Context, target any, allowed ...string) error {
	body := http.MaxBytesReader(c.Writer, c.Request.Body, arcaFeedbackWriteLimit)
	var fields map[string]json.RawMessage
	if err := common.DecodeJsonBody(body, &fields); err != nil {
		return err
	}
	for field := range fields {
		if !slices.Contains(allowed, field) {
			return errors.New("unexpected feedback field")
		}
	}
	data, err := common.Marshal(fields)
	if err != nil {
		return err
	}
	return common.Unmarshal(data, target)
}

func arcaFeedbackInputHash(input any) (string, error) {
	data, err := common.Marshal(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func arcaFeedbackQueryInt(c *gin.Context, key string, fallback, minimum, maximum int64) (int64, bool) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	return value, err == nil && value >= minimum && value <= maximum
}

func arcaFeedbackUUID(value string) (string, bool) {
	if len(value) != 36 {
		return "", false
	}
	parsed, err := uuid.Parse(value)
	return parsed.String(), err == nil
}

func isArcaFeedbackCategory(value string) bool {
	return slices.Contains([]string{"bug", "model", "access", "suggestion", "other"}, value)
}

func isArcaFeedbackStatus(value string) bool {
	return slices.Contains([]string{"open", "resolved", "closed"}, value)
}

func isArcaFeedbackText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= arcaFeedbackTextLimit && !strings.ContainsRune(value, 0)
}

func arcaFeedbackInputError(c *gin.Context, err error) {
	var overflow *http.MaxBytesError
	if errors.As(err, &overflow) {
		arcaFeedbackHTTPError(c, http.StatusRequestEntityTooLarge, "Invalid request", "ARCA_FEEDBACK_TOO_LARGE")
		return
	}
	arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid request", "ARCA_FEEDBACK_INVALID")
}

var (
	errArcaFeedbackInput         = errors.New("invalid feedback parameters")
	errArcaDiagnosticMissing     = errors.New("request diagnostics not available for this account")
	errArcaDiagnosticUnavailable = errors.New("request diagnostics service unavailable")
)

func isArcaFeedbackTitle(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= 200 && !strings.ContainsRune(value, 0)
}

func arcaFeedbackStorageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errArcaFeedbackInput):
		arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid parameters", "ARCA_FEEDBACK_INVALID")
	case errors.Is(err, errArcaDiagnosticMissing):
		arcaFeedbackHTTPError(c, http.StatusNotFound, "Request failed", "ARCA_FEEDBACK_REQUEST_UNAVAILABLE")
	case errors.Is(err, errArcaDiagnosticUnavailable):
		arcaFeedbackHTTPError(c, http.StatusServiceUnavailable, "Database error, please contact the administrator", "ARCA_DIAGNOSTIC_UNAVAILABLE")
	case errors.Is(err, model.ErrArcaFeedbackForbidden):
		arcaFeedbackHTTPError(c, http.StatusForbidden, "Unauthorized, insufficient privileges", "ARCA_FEEDBACK_FORBIDDEN")
	case errors.Is(err, model.ErrArcaFeedbackConflict):
		arcaFeedbackHTTPError(c, http.StatusConflict, "Conflict", "ARCA_FEEDBACK_CONFLICT")
	case errors.Is(err, gorm.ErrRecordNotFound):
		arcaFeedbackHTTPError(c, http.StatusNotFound, "Request failed", "ARCA_FEEDBACK_NOT_FOUND")
	default:
		common.SysLog(common.LogText("feedback storage operation failed: %T", err))
		arcaFeedbackHTTPError(c, http.StatusServiceUnavailable, "Database error, please contact the administrator", "ARCA_FEEDBACK_UNAVAILABLE")
	}
}

func arcaFeedbackHTTPError(c *gin.Context, status int, message, code string) {
	common.ApiErrorStatus(c, status, common.NewMessage(message), gin.H{"code": code})
}
