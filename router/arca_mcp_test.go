package router

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArcaMCPFeedbackConfirmation(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}, &model.UserAccessToken{}, &model.AuditLog{},
		&model.ArcaFeedbackIssue{}, &model.ArcaFeedbackEvent{}, &model.ArcaFeedbackNotification{}, &model.ArcaMCPConfirmation{}))
	person := model.User{Username: "arc-admin", AffCode: "ARCA-MCP", Group: "default",
		Role: common.RoleRootUser, Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, model.DB.Create(&person).Error)
	makePAT := func(name string, scopes ...string) (string, int) {
		t.Helper()
		raw := model.AccessTokenPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
		pat := model.UserAccessToken{Name: name, TokenHash: model.AccessTokenFingerprint(raw), TokenHint: model.AccessTokenHint(raw)}
		require.NoError(t, pat.SetScopes(scopes))
		require.NoError(t, model.CreateUserAccessToken(person.Id, &pat, service.AccessTokenMaxPerUser))
		return raw, pat.Id
	}
	writeToken, writeID := makePAT("write", "arca_feedback:read", "arca_feedback:write")
	readToken, readID := makePAT("read", "arca_feedback:read")
	session, err := service.CreateLoginSession(person.Id, "password", "127.0.0.1", "mcp-test")
	require.NoError(t, err)
	t.Setenv("ARCA_MCP_ALLOWED_HOSTS", "localhost")
	t.Setenv("ARCA_MCP_ALLOWED_ORIGINS", "")
	engine := gin.New()
	engine.POST("/mcp", middleware.DisableCache(), middleware.ArcaMCPAuth(), controller.ArcaMCP)
	request := func(token, host, origin, method string, params any) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := common.Marshal(gin.H{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
		req.Host = host
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}

	assert.Equal(t, http.StatusUnauthorized, request(session.AccessToken, "localhost", "", "tools/list", gin.H{}).Code)
	assert.Equal(t, http.StatusUnauthorized, request("sk-"+strings.Repeat("A", 43), "localhost", "", "tools/list", gin.H{}).Code)
	assert.Equal(t, http.StatusForbidden, request(writeToken, "mcp.example", "", "tools/list", gin.H{}).Code)
	assert.Equal(t, http.StatusForbidden, request(writeToken, "localhost", "https://foreign.example", "tools/list", gin.H{}).Code)
	rec := request(readToken, "localhost", "", "tools/list", gin.H{})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "arca_feedback_list")
	assert.NotContains(t, rec.Body.String(), "arca_feedback_create")
	assert.NotContains(t, rec.Body.String(), "arca_request_diagnostics")
	assert.Contains(t, rec.Header().Get("Cache-Control"), "no-store")
	rec = request(writeToken, "localhost", "", "initialize", gin.H{"protocolVersion": "2025-06-18"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "2025-06-18")

	body := gin.H{"title": "Original topic", "category": "bug", "description": "Issue detail", "client_request_id": uuid.NewString()}
	call := func(token string, arguments gin.H) (bool, string) {
		t.Helper()
		rec = request(token, "localhost", "", "tools/call", gin.H{"name": "arca_feedback_create", "arguments": arguments})
		require.Equal(t, http.StatusOK, rec.Code)
		var response struct {
			Result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response), rec.Body.String())
		require.Len(t, response.Result.Content, 1, rec.Body.String())
		return response.Result.IsError, response.Result.Content[0].Text
	}
	failed, preview := call(writeToken, gin.H{"body": body})
	require.False(t, failed, preview)
	assert.NotContains(t, preview, "Issue detail")
	var confirmation struct {
		Value string `json:"confirmation"`
	}
	require.NoError(t, common.Unmarshal([]byte(preview), &confirmation))
	require.NotEmpty(t, confirmation.Value)

	body["description"] = "Different body"
	failed, _ = call(writeToken, gin.H{"body": body, "dry_run": false, "confirmation": confirmation.Value})
	require.True(t, failed)
	body["description"] = "Issue detail"
	failed, result := call(writeToken, gin.H{"body": body, "dry_run": false, "confirmation": confirmation.Value})
	require.False(t, failed, result)
	assert.Contains(t, result, "Original topic")
	failed, _ = call(writeToken, gin.H{"body": body, "dry_run": false, "confirmation": confirmation.Value})
	require.True(t, failed)

	var rows int64
	require.NoError(t, model.DB.Model(&model.ArcaFeedbackIssue{}).Count(&rows).Error)
	assert.EqualValues(t, 1, rows)
	// Scope changes are observed on the next call without reinitializing MCP.
	_, err = model.UpdateUserAccessToken(person.Id, writeID, "write", []string{"arca_feedback:read"})
	require.NoError(t, err)
	rec = request(writeToken, "localhost", "", "tools/list", gin.H{})
	assert.NotContains(t, rec.Body.String(), "arca_feedback_create")
	_, err = model.DeleteUserAccessToken(person.Id, readID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, request(readToken, "localhost", "", "tools/list", gin.H{}).Code)
	// Account eligibility is also read live on every request.
	require.NoError(t, model.DB.Model(&person).UpdateColumn("role", common.RoleCommonUser).Error)
	assert.Equal(t, http.StatusForbidden, request(writeToken, "localhost", "", "tools/list", gin.H{}).Code)
}

func TestArcaFeedbackEditingAndPinPermissions(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}, &model.ArcaFeedbackIssue{},
		&model.ArcaFeedbackEvent{}, &model.ArcaFeedbackNotification{}))
	session := func(username string, role int) string {
		t.Helper()
		user := model.User{Username: username, AffCode: username, Group: "default", Role: role, Status: common.UserStatusEnabled, AuthVersion: 1}
		require.NoError(t, model.DB.Create(&user).Error)
		auth, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "feedback-test")
		require.NoError(t, err)
		return auth.AccessToken
	}
	owner := session("owner", common.RoleCommonUser)
	other := session("visitor", common.RoleCommonUser)
	admin := session("admin", common.RoleRootUser)

	engine := gin.New()
	SetApiRouter(engine)
	request := func(token, method, path string, args gin.H) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := common.Marshal(args)
		require.NoError(t, err)
		req := httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}
	rec := request(owner, http.MethodPost, "/api/arca/feedback", gin.H{
		"title": "Support topic", "category": "bug", "description": "Initial detail", "client_request_id": uuid.NewString(),
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var mutation struct {
		Data model.ArcaFeedbackMutation `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &mutation))
	issue := mutation.Data.Issue
	require.Positive(t, issue.Number)
	topicPath := fmt.Sprintf("/api/arca/feedback/%d", issue.Number)
	update := func(token string, version int64, args gin.H) *httptest.ResponseRecorder {
		t.Helper()
		args["expected_version"] = version
		if args["client_request_id"] == nil {
			args["client_request_id"] = uuid.NewString()
		}
		return request(token, http.MethodPost, topicPath+"/commands", args)
	}
	rec = update(owner, 1, gin.H{"action": "set_pinned", "pinned": true})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	rec = update(admin, 1, gin.H{"action": "set_pinned", "pinned": true})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = update(other, 2, gin.H{"action": "edit_issue", "title": "Not allowed"})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	rec = update(owner, 2, gin.H{"action": "edit_issue", "title": "Updated title"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = update(other, 3, gin.H{"action": "reply", "content": "Initial reply"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var detail struct {
		Data model.ArcaFeedbackDetail `json:"data"`
	}
	rec = request(owner, http.MethodGet, topicPath, nil)
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &detail))
	assert.Equal(t, "Updated title", detail.Data.Issue.Title)
	assert.NotNil(t, detail.Data.Issue.PinnedAt)
	reply := detail.Data.Events[len(detail.Data.Events)-1].Seq
	rec = update(admin, 4, gin.H{"action": "edit_reply", "seq": reply, "content": "Forbidden rewrite"})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	rec = update(other, 4, gin.H{"action": "edit_reply", "seq": reply, "content": "Updated reply"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = update(admin, 5, gin.H{"action": "delete_reply", "seq": reply})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = request(owner, http.MethodGet, topicPath, nil)
	assert.NotContains(t, rec.Body.String(), "Updated reply")
	args := gin.H{"action": "delete_issue"}
	rec = update(owner, 6, args)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = update(owner, 6, args)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"replayed":true`)
	rec = request(admin, http.MethodGet, topicPath, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
