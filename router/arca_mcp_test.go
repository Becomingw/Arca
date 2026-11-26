package router

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

func TestArcaMCPOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, dialect := range []struct {
		kind common.DatabaseType
		env  string
		open func(string) gorm.Dialector
	}{
		{common.DatabaseTypeSQLite, "", func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{common.DatabaseTypeMySQL, "TEST_ARCA_MCP_MYSQL_DSN", mysql.Open},
		{common.DatabaseTypePostgreSQL, "TEST_ARCA_MCP_POSTGRES_DSN", postgres.Open},
	} {
		t.Run(string(dialect.kind), func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip(dialect.env + " not configured")
			}
			db, err := gorm.Open(dialect.open(dsn), &gorm.Config{Logger: logger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
			if dialect.kind == common.DatabaseTypeSQLite {
				sqlDB.SetMaxOpenConns(1)
			}
			tables := []any{&model.User{}, &model.Ability{}, &model.Channel{}, &model.UserAccessToken{}, &model.AuditLog{},
				&model.ArcaMCPConfirmation{}, &model.ArcaEntitlementPolicyRow{}, &model.ArcaBudgetAccount{},
				&model.ArcaBudgetReservation{}, &model.ArcaUsageGrant{}, &model.ArcaBudgetGrantUse{}}
			// Access token queries and audit writes use fixed table names.
			// Require an empty fixture database so no existing tables can be dropped.
			for _, table := range tables {
				require.False(t, db.Migrator().HasTable(table), "use an empty Arca MCP test database (%T already exists)", table)
			}
			t.Cleanup(func() { assert.NoError(t, db.Migrator().DropTable(tables...)) })
			require.NoError(t, db.AutoMigrate(tables...))
			oldDB, oldLog, oldRedis := model.DB, model.LOG_DB, common.RedisEnabled
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
			common.SetDatabaseTypes(dialect.kind, dialect.kind)
			t.Cleanup(func() {
				model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLog, oldRedis
				common.SetDatabaseTypes(oldMainType, oldLogType)
			})
			testArcaMCPOperations(t)
		})
	}
}

func testArcaMCPOperations(t *testing.T) {
	t.Helper()
	root := model.User{Username: "operations-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "ops-root"}
	member := model.User{Username: "private@example.t", Group: "default", Status: common.UserStatusEnabled,
		Email: "hidden@example.test", Password: "hidden-password-hash", AffCode: "ops-member"}
	require.NoError(t, model.DB.Create(&root).Error)
	require.NoError(t, model.DB.Create(&member).Error)

	makePAT := func(scopes ...string) string {
		t.Helper()
		raw := model.AccessTokenPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
		pat := model.UserAccessToken{Name: "operations", TokenHash: model.AccessTokenFingerprint(raw), TokenHint: model.AccessTokenHint(raw)}
		require.NoError(t, pat.SetScopes(scopes))
		require.NoError(t, model.CreateUserAccessToken(root.Id, &pat, service.AccessTokenMaxPerUser))
		return raw
	}
	full := makePAT("user:read", "group:read", "channel:read", "model:read", "arca_entitlements:read", "arca_entitlements:write")
	readOnly := makePAT("channel:read", "arca_entitlements:read")
	secondWriter := makePAT("arca_entitlements:write")
	t.Setenv("ARCA_MCP_ENABLED", "true")
	t.Setenv("ARCA_MCP_ALLOWED_HOSTS", "localhost")
	engine := gin.New()
	engine.POST("/mcp", middleware.DisableCache(), middleware.ArcaMCPAuth(), controller.ArcaMCP)
	type rpcResponse struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
		Result struct {
			IsError bool `json:"isError"`
			Tools   []struct {
				Name string `json:"name"`
			} `json:"tools"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	request := func(token, method string, params any) rpcResponse {
		t.Helper()
		payload, err := common.Marshal(gin.H{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
		req.Host = "localhost"
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var response rpcResponse
		require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
		return response
	}
	call := func(token, tool string, args gin.H, fails bool) string {
		t.Helper()
		response := request(token, "tools/call", gin.H{"name": tool, "arguments": args})
		require.Nil(t, response.Error)
		require.Len(t, response.Result.Content, 1)
		text := response.Result.Content[0].Text
		require.Equal(t, fails, response.Result.IsError, text)
		return text
	}

	ctx := t.Context()
	plan := func(limit float64) model.ArcaSubscriptionPlan {
		return model.ArcaSubscriptionPlan{Name: "Named plan", Allocation: model.ArcaAllocation{Daily: &limit},
			ModelAccess: model.ArcaModelAccess{Mode: "unrestricted"}}
	}
	policy := model.ArcaEntitlementPolicy{Plans: map[string]model.ArcaSubscriptionPlan{"group-plan": plan(10), "personal-plan": plan(20)},
		GroupSubscriptions: map[string]string{"default": "group-plan"}}
	version, err := model.SaveArcaEntitlementPolicy(ctx, 0, policy)
	require.NoError(t, err)
	require.EqualValues(t, 1, version)
	now := time.Now()
	require.NoError(t, model.DB.Create(&model.ArcaBudgetReservation{UserID: member.Id, RequestID: "mcp-request", Model: "arca-model",
		AdmittedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Status: "pending", CostMicros: 125_000, QuotaPerUSD: 500_000}).Error)
	require.NoError(t, model.DB.Create(&model.ArcaUsageGrant{ID: 6, UserID: member.Id, Name: "hidden-grant-note",
		StartsAt: now.Add(-time.Minute).Unix(), EndsAt: now.Add(time.Hour).Unix(), AmountMicros: 54_000}).Error)

	baseURL := "https://private-upstream.example.test/"
	channel := model.Channel{Key: "hidden-channel-key", BaseURL: &baseURL, Name: "Backup channel", Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 3, MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled}},
		OtherInfo:   `{"balance_detail":{"error":"hidden-quota-error","keys":{"0":{"error":"hidden-upstream-key"}}}}`}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "arca-model", ChannelId: channel.Id, Enabled: true, Weight: 3}).Error)

	tools := func(token string) []string {
		result := request(token, "tools/list", gin.H{})
		names := make([]string, 0, len(result.Result.Tools))
		for _, tool := range result.Result.Tools {
			names = append(names, tool.Name)
		}
		return names
	}
	assert.Contains(t, tools(full), "arca_model_routes")
	assert.Contains(t, tools(full), "arca_subscription_assign")
	assert.NotContains(t, tools(readOnly), "arca_model_routes")
	assert.NotContains(t, tools(readOnly), "arca_subscription_assign")
	denied := request(readOnly, "tools/call", gin.H{"name": "arca_model_routes", "arguments": gin.H{"query": gin.H{
		"group": "default", "model": "arca-model",
	}}})
	require.NotNil(t, denied.Error)

	text := call(full, "arca_users_list", gin.H{"query": gin.H{"username": member.Username}}, false)
	assert.Contains(t, text, fmt.Sprintf(`"id":%d`, member.Id))
	assert.NotContains(t, text, member.Username)
	assert.NotContains(t, text, member.Email)
	assert.NotContains(t, text, member.Password)
	assert.Contains(t, call(full, "arca_groups_list", gin.H{}, false), "default")
	assert.Contains(t, call(full, "arca_subscription_plans_list", gin.H{}, false), "personal-plan")

	userPath := gin.H{"path": gin.H{"user_id": fmt.Sprint(member.Id)}}
	text = call(readOnly, "arca_benefits_get", userPath, false)
	assert.Contains(t, text, `"subscription_id":"group-plan"`)
	assert.Contains(t, text, `"used_micros":0`)
	assert.Contains(t, text, `"reserved_micros":125000`)
	assert.Contains(t, text, `"remaining_micros":9875000`)
	assert.Contains(t, text, `"remaining_micros":54000`)
	assert.NotContains(t, text, `"role"`)
	assert.NotContains(t, text, `"status"`)
	assert.NotContains(t, text, "hidden-grant-note")

	text = call(readOnly, "arca_provider_get", gin.H{"path": gin.H{"channel_id": fmt.Sprint(channel.Id)}}, false)
	assert.Contains(t, text, `"enabled_keys":2`)
	assert.Contains(t, text, `"disabled_keys":1`)
	assert.Contains(t, text, `"keys_with_quota_errors":1`)
	assert.NotContains(t, text, channel.Key)
	assert.NotContains(t, text, baseURL)
	assert.NotContains(t, text, "hidden-quota-error")
	text = call(full, "arca_model_routes", gin.H{"query": gin.H{"group": "default", "model": "arca-model"}}, false)
	assert.Contains(t, text, fmt.Sprintf(`"channel_id":%d`, channel.Id))
	assert.Contains(t, text, `"weight":3`)

	const assignmentTool = "arca_subscription_assign"
	body := gin.H{"expected_version": 1, "subject_type": "user", "user_id": member.Id, "plan_id": "personal-plan"}
	preview := func(body gin.H) string {
		t.Helper()
		text := call(full, assignmentTool, gin.H{"body": body}, false)
		var preview struct {
			Ticket string `json:"confirmation"`
		}
		require.NoError(t, common.Unmarshal([]byte(text), &preview))
		require.NotEmpty(t, preview.Ticket)
		return preview.Ticket
	}
	ticket := preview(body)
	assert.Contains(t, call(full, "arca_benefits_get", userPath, false), `"subscription_id":"group-plan"`)
	assert.Contains(t, call(secondWriter, assignmentTool, gin.H{"body": body, "dry_run": false, "confirmation": ticket}, true), "bound to different arguments")
	body["plan_id"] = "group-plan"
	call(full, assignmentTool, gin.H{"body": body, "dry_run": false, "confirmation": ticket}, true)
	body["plan_id"] = "personal-plan"

	staleBody := gin.H{"expected_version": 1, "subject_type": "group", "group": "default", "plan_id": ""}
	staleTicket := preview(staleBody)
	confirmed := gin.H{"body": body, "dry_run": false, "confirmation": ticket}
	assert.Contains(t, call(full, assignmentTool, confirmed, false), `"version":2`)
	call(full, assignmentTool, confirmed, true)
	stale := gin.H{"body": staleBody, "dry_run": false, "confirmation": staleTicket}
	assert.Contains(t, call(full, assignmentTool, stale, true), "policy version changed")
	assert.Contains(t, call(full, assignmentTool, stale, true), "consumed")
	assert.Contains(t, call(full, "arca_benefits_get", userPath, false), `"subscription_id":"personal-plan"`)

	body["expected_version"], body["plan_id"] = 2, ""
	ticket = preview(body)
	call(full, assignmentTool, gin.H{"body": body, "dry_run": false, "confirmation": ticket}, false)
	assert.Contains(t, call(full, "arca_benefits_get", userPath, false), `"subscription_id":"group-plan"`)
	staleBody["expected_version"] = 3
	ticket = preview(staleBody)
	call(full, assignmentTool, gin.H{"body": staleBody, "dry_run": false, "confirmation": ticket}, false)
	assert.NotContains(t, call(full, "arca_benefits_get", userPath, false), `"subscription_id"`)
	var audits []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", assignmentTool).Order("id").Find(&audits).Error)
	require.Len(t, audits, 4)
	assert.True(t, audits[0].Success)
	assert.False(t, audits[1].Success)
	assert.Equal(t, "access_token", audits[0].AuthMethod)
	assert.Equal(t, root.Id, audits[0].UserId)
	require.NotNil(t, audits[0].Other.Op)
	assert.Contains(t, audits[0].Other.Op.Params, "expected_version")

	require.NoError(t, model.DB.Model(&root).UpdateColumn("role", common.RoleAdminUser).Error)
	assert.NotContains(t, tools(full), assignmentTool)
	assert.NotContains(t, tools(full), "arca_benefits_get")
	denied = request(full, "tools/call", gin.H{"name": assignmentTool, "arguments": gin.H{"body": body}})
	require.NotNil(t, denied.Error)
}
