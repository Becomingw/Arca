package controller

import (
	"context"
	"crypto/rand"
	"errors"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const arcaMCPDefaultVersion = "2025-11-25"

var (
	arcaMCPAdmission = make(chan struct{}, 16)
	// Native model metadata uses the dashboard's model:read PAT scope and
	// AdminAuth. Channel reads additionally use their live Casbin permission.
	arcaMCPModelRead = authz.Permission{Resource: "model", Action: authz.ActionRead}
	arcaMCPUserRead  = authz.Permission{Resource: "user", Action: authz.ActionRead}
	arcaMCPGroupRead = authz.Permission{Resource: "group", Action: authz.ActionRead}
)

type arcaMCPRequest struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      common.RawMessage `json:"id"`
	Method  string            `json:"method"`
	Params  common.RawMessage `json:"params"`
}

type arcaMCPTool struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Permission   authz.Permission `json:"-"`
	AlsoRequires authz.Permission `json:"-"`
	InputSchema  gin.H            `json:"inputSchema"`
	Annotations  gin.H            `json:"annotations"`
}

func arcaMCPToolCatalog() []arcaMCPTool {
	obj := func(properties gin.H) gin.H {
		return gin.H{"type": "object", "properties": properties, "additionalProperties": false}
	}
	path := func(key string) gin.H {
		schema := obj(gin.H{key: gin.H{"type": "string"}})
		schema["required"] = []string{key}
		return schema
	}
	query := func(properties gin.H) gin.H { return obj(gin.H{"query": obj(properties)}) }
	str := gin.H{"type": "string"}
	num := gin.H{"type": "integer"}
	boo := gin.H{"type": "boolean"}
	write := func(idPath gin.H, bodyProps gin.H, requiredBody []string) gin.H {
		body := obj(bodyProps)
		body["required"] = requiredBody
		properties := gin.H{"body": body, "dry_run": gin.H{"type": "boolean", "default": true}, "confirmation": str}
		result := obj(properties)
		result["required"] = []string{"body"}
		if idPath != nil {
			properties["path"] = idPath
			result["required"] = []string{"body", "path"}
		}
		return result
	}
	readOnly := gin.H{"readOnlyHint": true, "openWorldHint": false}
	mutation := gin.H{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false}
	previewBody := obj(gin.H{"user_id": num, "api_key_id": num, "config": gin.H{"type": "object"}})
	previewBody["required"] = []string{"user_id"}
	previewSchema := obj(gin.H{"body": previewBody})
	previewSchema["required"] = []string{"body"}
	paging := gin.H{"limit": num, "skip": num}
	routeFilter := arcaMCPRequiredObject(obj(gin.H{"group": str, "model": str, "limit": num, "skip": num}), "group", "model")
	assignment := write(nil, gin.H{
		"expected_version": num,
		"subject_type":     gin.H{"type": "string", "enum": []string{"user", "group"}},
		"user_id":          num,
		"group":            str,
		"plan_id":          gin.H{"type": "string", "description": "An existing plan ID, or an empty string to clear a direct assignment."},
	}, []string{"expected_version", "subject_type", "plan_id"})
	return []arcaMCPTool{
		{Name: "arca_identity", Description: "Show the account and current permissions for this token.", InputSchema: obj(gin.H{}), Annotations: readOnly},
		{Name: "arca_feedback_list", Description: "List public Arca feedback topics", Permission: authz.ArcaFeedbackRead,
			InputSchema: query(gin.H{"category": str, "status": str, "search": str, "limit": num, "skip": num}),
			Annotations: readOnly},
		{Name: "arca_feedback_get", Description: "Read one public feedback topic and a page of its event history.", Permission: authz.ArcaFeedbackRead,
			InputSchema: gin.H{"type": "object", "properties": gin.H{"path": path("issue_id"), "query": obj(gin.H{"after_event": num, "limit": num})}, "required": []string{"path"}, "additionalProperties": false},
			Annotations: readOnly},
		{Name: "arca_request_diagnostics", Description: "Read a redacted request diagnostic summary.", Permission: authz.ArcaMonitoringRead,
			InputSchema: gin.H{"type": "object", "properties": gin.H{"path": path("request_id")}, "required": []string{"path"}, "additionalProperties": false},
			Annotations: readOnly},
		{Name: "arca_providers_list", Description: "List channels and their last observed test status without credentials or upstream addresses.", Permission: authz.ChannelRead,
			InputSchema: query(gin.H{"limit": num, "skip": num}), Annotations: readOnly},
		{Name: "arca_models_list", Description: "List configured public model metadata without upstream mappings.", Permission: arcaMCPModelRead,
			InputSchema: query(gin.H{"limit": num, "skip": num}), Annotations: readOnly},
		{Name: "arca_health_status", Description: "Check whether the gateway can reach its configured databases.", Permission: authz.ArcaMonitoringRead,
			InputSchema: obj(gin.H{}), Annotations: readOnly},
		{Name: "arca_benefits_preview", Description: "Preview a user's model access and usage limits without changing them.", Permission: authz.ArcaEntitlementsRead,
			InputSchema: previewSchema, Annotations: readOnly},
		{Name: "arca_users_list", Description: "List account IDs, roles, status and native groups. Optionally filter by an exact username or native group.", Permission: arcaMCPUserRead,
			InputSchema: query(gin.H{"limit": num, "skip": num, "username": str, "group": str}), Annotations: readOnly},
		{Name: "arca_user_get", Description: "Read an account's internal ID, native group, role and status.", Permission: arcaMCPUserRead,
			InputSchema: arcaMCPRequiredObject(obj(gin.H{"path": path("user_id")}), "path"), Annotations: readOnly},
		{Name: "arca_groups_list", Description: "List native groups available for subscription assignment.", Permission: arcaMCPGroupRead,
			InputSchema: query(paging), Annotations: readOnly},
		{Name: "arca_subscription_plans_list", Description: "List subscription plans and the current policy version for a previewed assignment.", Permission: authz.ArcaEntitlementsRead,
			InputSchema: query(paging), Annotations: readOnly},
		{Name: "arca_benefits_get", Description: "Read an account's effective plan, model rules, metered windows and temporary grants.", Permission: authz.ArcaEntitlementsRead,
			InputSchema: arcaMCPRequiredObject(obj(gin.H{"path": path("user_id")}), "path"), Annotations: readOnly},
		{Name: "arca_provider_get", Description: "Read a channel's persisted health, routing weights, credential counts and quota-probe status.", Permission: authz.ChannelRead,
			InputSchema: arcaMCPRequiredObject(obj(gin.H{"path": path("channel_id")}), "path"), Annotations: readOnly},
		{Name: "arca_model_routes", Description: "List stored channel candidates for an exact gateway model alias and native routing group.", Permission: authz.ChannelRead,
			AlsoRequires: arcaMCPModelRead,
			InputSchema:  arcaMCPRequiredObject(obj(gin.H{"query": routeFilter}), "query"), Annotations: readOnly},
		{Name: "arca_subscription_assign", Description: "Preview and confirm assigning or clearing a plan for one user or native group.", Permission: authz.ArcaEntitlementsWrite,
			InputSchema: assignment, Annotations: gin.H{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": false}},
		{Name: "arca_feedback_create", Description: "Preview and confirm creating a public feedback topic.", Permission: authz.ArcaFeedbackWrite,
			InputSchema: write(nil, gin.H{"title": str, "description": str, "category": str, "model": str, "visibility": str, "client_request_id": str, "request_id": str, "include_diagnostics": boo},
				[]string{"title", "description", "category", "client_request_id"}), Annotations: mutation},
		{Name: "arca_feedback_command", Description: "Preview and confirm a reply or status update on a public feedback topic.", Permission: authz.ArcaFeedbackWrite,
			InputSchema: write(path("issue_id"), gin.H{"expected_version": num, "client_request_id": str, "action": gin.H{"type": "string", "enum": []string{"reply", "set_status"}},
				"visibility": str, "content": str, "status": str, "comment": str, "diagnostic_request_id": str},
				[]string{"expected_version", "client_request_id", "action"}), Annotations: mutation},
	}
}

func arcaMCPRequiredObject(schema gin.H, required ...string) gin.H {
	schema["required"] = required
	return schema
}

type arcaMCPIdentity struct {
	UserID  int      `json:"user_id"`
	Role    int      `json:"role"`
	TokenID int      `json:"key_id"`
	Scopes  []string `json:"scopes"`
}

func (actor arcaMCPIdentity) can(permission authz.Permission) bool {
	if permission.Resource == "" {
		return true
	}
	if permission.Resource == authz.ArcaEntitlementsRead.Resource && actor.Role != common.RoleRootUser {
		return false
	}
	if !service.AccessTokenScopeGranted(actor.Scopes, service.AccessTokenScopeOf(permission)) {
		return false
	}
	if permission == arcaMCPModelRead || permission == arcaMCPUserRead || permission == arcaMCPGroupRead {
		return actor.Role >= common.RoleAdminUser
	}
	return authz.Can(actor.UserID, actor.Role, permission)
}

func (actor arcaMCPIdentity) canUse(tool arcaMCPTool) bool {
	return actor.can(tool.Permission) && actor.can(tool.AlsoRequires)
}

func ArcaMCP(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		c.Header("Allow", "POST")
		c.Status(http.StatusMethodNotAllowed)
		return
	}
	select {
	case arcaMCPAdmission <- struct{}{}:
		defer func() { <-arcaMCPAdmission }()
	default:
		c.Header("Retry-After", "1")
		c.Status(http.StatusTooManyRequests)
		return
	}
	if !slices.Contains([]string{"", "2025-03-26", "2025-06-18", "2025-11-25"}, c.GetHeader("MCP-Protocol-Version")) {
		c.Status(http.StatusBadRequest)
		return
	}
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		c.Status(http.StatusUnsupportedMediaType)
		return
	}
	acceptsJSON := false
	for accept := range strings.SplitSeq(c.GetHeader("Accept"), ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(accept))
		if err == nil && params["q"] != "0" && slices.Contains([]string{"application/json", "application/*", "*/*"}, mediaType) {
			acceptsJSON = true
			break
		}
	}
	if !acceptsJSON && c.GetHeader("Accept") != "" {
		c.Status(http.StatusNotAcceptable)
		return
	}
	var request arcaMCPRequest
	err = common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20), &request)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		c.Status(http.StatusRequestEntityTooLarge)
		return
	case err != nil:
		writeArcaMCPError(c, nil, -32700, "Parse error")
		return
	case request.JSONRPC != "2.0" || request.Method == "":
		writeArcaMCPError(c, nil, -32600, "Invalid Request")
		return
	}
	if len(request.ID) == 0 {
		// Only MCP notifications are accepted without a JSON-RPC ID.
		if strings.HasPrefix(request.Method, "notifications/") {
			c.Status(http.StatusAccepted)
		} else {
			writeArcaMCPError(c, nil, -32600, "Invalid Request")
		}
		return
	}
	idType := common.GetJsonType(request.ID)
	if len(request.ID) > 512 || (idType != "string" && idType != "number") {
		writeArcaMCPError(c, nil, -32600, "Invalid Request")
		return
	}
	scopes, _ := c.Get("access_token_scopes")
	granted, _ := scopes.([]string)
	actor := arcaMCPIdentity{UserID: c.GetInt("id"), Role: c.GetInt("role"), TokenID: c.GetInt("access_token_id"), Scopes: append([]string{}, granted...)}
	if actor.UserID <= 0 || actor.Role < common.RoleAdminUser || actor.TokenID <= 0 {
		c.Status(http.StatusUnauthorized)
		return
	}
	slices.Sort(actor.Scopes)
	var result any
	switch request.Method {
	case "initialize":
		var initialize struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if common.Unmarshal(request.Params, &initialize) != nil || initialize.ProtocolVersion == "" {
			writeArcaMCPError(c, request.ID, -32602, "Invalid params")
			return
		}
		if !slices.Contains([]string{"2025-03-26", "2025-06-18", "2025-11-25"}, initialize.ProtocolVersion) {
			initialize.ProtocolVersion = arcaMCPDefaultVersion
		}
		result = gin.H{"protocolVersion": initialize.ProtocolVersion, "serverInfo": gin.H{"name": "Arca", "version": "1.0.0"}, "capabilities": gin.H{"tools": gin.H{}}}
	case "ping":
		result = gin.H{}
	case "tools/list":
		catalog := arcaMCPToolCatalog()
		result = gin.H{"tools": slices.DeleteFunc(catalog, func(tool arcaMCPTool) bool { return !actor.canUse(tool) })}
	case "tools/call":
		var call struct {
			Name      string            `json:"name"`
			Arguments common.RawMessage `json:"arguments"`
		}
		if common.Unmarshal(request.Params, &call) != nil {
			writeArcaMCPError(c, request.ID, -32602, "Invalid params")
			return
		}
		tool, ok := arcaMCPAuthorizedTool(actor, call.Name)
		if !ok {
			writeArcaMCPError(c, request.ID, -32602, "Unknown or unauthorized tool")
			return
		}
		result = callArcaMCPTool(c, actor, tool, call.Arguments)
	default:
		writeArcaMCPError(c, request.ID, -32601, "Method not found")
		return
	}
	writeArcaMCPJSON(c, gin.H{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func arcaMCPAuthorizedTool(actor arcaMCPIdentity, name string) (arcaMCPTool, bool) {
	if old, ok := strings.CutPrefix(name, "afssr_"); ok {
		name = "arca_" + old
	}
	for _, tool := range arcaMCPToolCatalog() {
		if tool.Name == name && actor.canUse(tool) {
			return tool, true
		}
	}
	return arcaMCPTool{}, false
}

type arcaMCPToolArguments struct {
	Path         map[string]string `json:"path"`
	Query        common.RawMessage `json:"query"`
	Body         common.RawMessage `json:"body"`
	DryRun       *bool             `json:"dry_run"`
	Confirmation string            `json:"confirmation"`
}

func callArcaMCPTool(c *gin.Context, actor arcaMCPIdentity, tool arcaMCPTool, raw common.RawMessage) gin.H {
	var args arcaMCPToolArguments
	if len(raw) != 0 && decodeArcaMCPObject(raw, &args, "path", "query", "body", "dry_run", "confirmation") != nil {
		return arcaMCPToolError(errArcaFeedbackInput)
	}
	ctx := c.Request.Context()
	isWrite := tool.Permission.Action == authz.ActionWrite
	if !isWrite && (args.DryRun != nil || args.Confirmation != "" || (len(args.Body) != 0 && tool.Name != "arca_benefits_preview")) {
		return arcaMCPToolError(errArcaFeedbackInput)
	}
	var result any
	var err error
	switch tool.Name {
	case "arca_identity":
		if len(args.Path) != 0 || len(args.Query) != 0 {
			return arcaMCPToolError(errArcaFeedbackInput)
		}
		effective := []string{}
		for _, available := range arcaMCPToolCatalog() {
			if available.Permission.Resource != "" && actor.canUse(available) {
				scope := service.AccessTokenScopeOf(available.Permission)
				if !slices.Contains(effective, scope) {
					effective = append(effective, scope)
				}
			}
		}
		slices.Sort(effective)
		actor.Scopes = effective
		result = actor
	case "arca_feedback_list":
		result, err = readArcaMCPFeedbackList(ctx, args)
	case "arca_feedback_get":
		result, err = readArcaMCPFeedbackTopic(ctx, args)
	case "arca_request_diagnostics":
		id, valid := args.Path["request_id"]
		if !valid || len(args.Path) != 1 || len(args.Query) != 0 || !validArcaDiagnosticRequestID(id) {
			return arcaMCPToolError(errArcaFeedbackInput)
		}
		var evidence model.ArcaRequestDiagnosticEvidence
		evidence, err = model.ReadArcaAdminRequestDiagnosticEvidence(ctx, id)
		result = buildArcaRequestDiagnostic(id, evidence)
	case "arca_providers_list", "arca_models_list":
		query := struct {
			Limit int `json:"limit"`
			Skip  int `json:"skip"`
		}{Limit: 20}
		if len(args.Path) != 0 ||
			(len(args.Query) != 0 && decodeArcaMCPObject(args.Query, &query, "limit", "skip") != nil) ||
			query.Limit < 1 || query.Limit > 100 || query.Skip < 0 || query.Skip > 1_000_000 {
			return arcaMCPToolError(errArcaFeedbackInput)
		}
		if tool.Name == "arca_providers_list" {
			result, err = model.ReadArcaMCPChannels(ctx, query.Skip, query.Limit)
		} else {
			result, err = model.ReadArcaMCPModels(ctx, query.Skip, query.Limit)
		}
	case "arca_health_status":
		if len(args.Path) != 0 || len(args.Query) != 0 {
			return arcaMCPToolError(errArcaFeedbackInput)
		}
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		result = model.ReadArcaMCPHealth(pingCtx)
		cancel()
	case "arca_benefits_preview":
		var input struct {
			UserID   int                          `json:"user_id"`
			APIKeyID int                          `json:"api_key_id"`
			Config   *model.ArcaEntitlementPolicy `json:"config"`
		}
		if len(args.Path) != 0 || len(args.Query) != 0 || len(args.Body) > 256<<10 ||
			decodeArcaMCPObject(args.Body, &input, "user_id", "api_key_id", "config") != nil || input.UserID < 1 || input.APIKeyID < 0 {
			return arcaMCPToolError(errArcaFeedbackInput)
		}
		var status int
		result, status, err = getArcaEntitlementPreview(ctx, input.UserID, input.APIKeyID, input.Config)
		switch status {
		case http.StatusBadRequest:
			err = errArcaFeedbackInput
		case http.StatusNotFound:
			err = gorm.ErrRecordNotFound
		}
	case "arca_feedback_create", "arca_feedback_command":
		result, err = writeArcaMCPFeedback(c, actor, tool.Name, args)
	case "arca_users_list", "arca_user_get", "arca_groups_list", "arca_subscription_plans_list", "arca_benefits_get", "arca_provider_get", "arca_model_routes":
		result, err = readArcaMCPOperation(ctx, tool.Name, args)
	case "arca_subscription_assign":
		result, err = writeArcaMCPSubscriptionAssignment(c, actor, tool.Name, args)
	}
	if err != nil {
		return arcaMCPToolError(err)
	}
	data, err := common.Marshal(result)
	if err != nil || len(data) > 128<<10 {
		return arcaMCPToolResult("Result unavailable or too large. Reduce the query page size.", true)
	}
	return arcaMCPToolResult(string(data), false)
}

func readArcaMCPFeedbackList(ctx context.Context, args arcaMCPToolArguments) (model.ArcaFeedbackPage, error) {
	query := struct {
		Status   string `json:"status"`
		Category string `json:"category"`
		Search   string `json:"search"`
		Limit    int    `json:"limit"`
		Skip     int    `json:"skip"`
	}{Limit: 20}
	if len(args.Path) != 0 ||
		(len(args.Query) != 0 && decodeArcaMCPObject(args.Query, &query, "status", "category", "search", "limit", "skip") != nil) ||
		query.Limit < 1 || query.Limit > 100 || query.Skip < 0 || query.Skip > 1_000_000 ||
		(query.Status != "" && !isArcaFeedbackStatus(query.Status)) ||
		(query.Category != "" && !isArcaFeedbackCategory(query.Category)) ||
		utf8.RuneCountInString(query.Search) > 200 || strings.ContainsRune(query.Search, 0) {
		return model.ArcaFeedbackPage{}, errArcaFeedbackInput
	}
	return model.ListArcaFeedback(ctx, model.ArcaFeedbackFilter{Status: query.Status, Category: query.Category, Search: strings.TrimSpace(query.Search), Limit: query.Limit, Skip: query.Skip})
}

func readArcaMCPFeedbackTopic(ctx context.Context, args arcaMCPToolArguments) (model.ArcaFeedbackDetail, error) {
	query := struct {
		After int64 `json:"after_event"`
		Limit int   `json:"limit"`
	}{Limit: 20}
	if len(args.Path) != 1 ||
		(len(args.Query) != 0 && decodeArcaMCPObject(args.Query, &query, "after_event", "limit") != nil) ||
		query.After < 0 || query.Limit < 1 || query.Limit > 100 {
		return model.ArcaFeedbackDetail{}, errArcaFeedbackInput
	}
	id, err := arcaFeedbackIssueID(ctx, args.Path["issue_id"])
	if err != nil {
		return model.ArcaFeedbackDetail{}, err
	}
	return model.GetArcaFeedbackDetail(ctx, id, query.After, query.Limit)
}

func writeArcaMCPFeedback(c *gin.Context, actor arcaMCPIdentity, name string, args arcaMCPToolArguments) (any, error) {
	ctx := c.Request.Context()
	if len(args.Query) != 0 || len(args.Body) > arcaFeedbackWriteLimit {
		return nil, errArcaFeedbackInput
	}
	var create arcaFeedbackCreateInput
	var command arcaFeedbackCommandInput
	var payload any
	var issueID string
	hasDiagnostic := false
	switch name {
	case "arca_feedback_create":
		if len(args.Path) != 0 || decodeArcaMCPObject(args.Body, &create, "title", "description", "category", "model", "visibility", "client_request_id", "request_id", "include_diagnostics") != nil {
			return nil, errArcaFeedbackInput
		}
		hasDiagnostic = create.RequestID != nil || create.IncludeDiagnostics
		payload = create
	case "arca_feedback_command":
		if len(args.Path) != 1 || args.Path["issue_id"] == "" ||
			decodeArcaMCPObject(args.Body, &command, "action", "expected_version", "client_request_id", "visibility", "content", "status", "comment", "diagnostic_request_id") != nil ||
			(command.Action != "reply" && command.Action != "set_status") {
			return nil, errArcaFeedbackInput
		}
		var err error
		issueID, err = arcaFeedbackIssueID(ctx, args.Path["issue_id"])
		if err != nil {
			return nil, err
		}
		hasDiagnostic = command.DiagnosticRequestID != nil
		payload = command
	}
	if hasDiagnostic && (!actor.can(authz.ArcaMonitoringRead) || (command.DiagnosticRequestID != nil && command.Visibility != model.ArcaFeedbackPublic) ||
		(create.IncludeDiagnostics && create.Visibility != model.ArcaFeedbackPublic)) {
		return nil, model.ErrArcaFeedbackForbidden
	}
	binding, err := arcaFeedbackInputHash(struct {
		Operation string
		IssueID   string
		Actor     arcaMCPIdentity
		Body      any
	}{name, issueID, actor, payload})
	if err != nil {
		return nil, err
	}
	confirmation, err := authorizeArcaMCPMutation(ctx, actor, args, binding)
	if err != nil {
		return nil, err
	}
	if confirmation != nil {
		return gin.H{"dry_run": true, "confirmation": confirmation.Ticket, "expires_at": confirmation.ExpiresAt,
			"tool": name, "issue_id": issueID, "visibility": model.ArcaFeedbackPublic}, nil
	}

	var result model.ArcaFeedbackMutation
	if name == "arca_feedback_create" {
		result, err = createArcaFeedback(ctx, actor.UserID, actor.Role, create, hasDiagnostic)
	} else {
		result, err = commandArcaFeedback(ctx, actor.UserID, actor.Role, issueID, command, hasDiagnostic)
	}
	model.RecordAuditLog(c, model.AuditLog{
		UserId: actor.UserID, ActorRole: actor.Role, Category: model.AuditCategoryOperation,
		Action: name, TokenRef: c.GetString("access_token_ref"), Success: err == nil,
	})
	return result, err
}

type arcaMCPMutationPreview struct {
	Ticket    string
	ExpiresAt int64
}

func authorizeArcaMCPMutation(ctx context.Context, actor arcaMCPIdentity, args arcaMCPToolArguments, binding string) (*arcaMCPMutationPreview, error) {
	ticket := model.ArcaMCPConfirmation{BindingHash: binding, ActorID: actor.UserID, TokenID: actor.TokenID}
	if args.DryRun == nil || *args.DryRun {
		if args.Confirmation != "" {
			return nil, errArcaFeedbackInput
		}
		rawTicket := rand.Text()
		ticket.TicketHash = model.AccessTokenFingerprint(rawTicket)
		ticket.ExpiresAt = common.GetTimestamp() + 300
		if err := model.StoreArcaMCPConfirmation(ctx, ticket); err != nil {
			return nil, err
		}
		return &arcaMCPMutationPreview{Ticket: rawTicket, ExpiresAt: ticket.ExpiresAt}, nil
	}
	if len(args.Confirmation) < 16 || len(args.Confirmation) > 128 {
		return nil, errArcaMCPConfirmation
	}
	ticket.TicketHash = model.AccessTokenFingerprint(args.Confirmation)
	ok, err := model.ConsumeArcaMCPConfirmation(ctx, ticket)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errArcaMCPConfirmation
	}

	return nil, nil
}

func decodeArcaMCPObject(raw common.RawMessage, target any, allowed ...string) error {
	var fields map[string]common.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil || fields == nil {
		return errArcaFeedbackInput
	}
	for name := range fields {
		if !slices.Contains(allowed, name) {
			return errArcaFeedbackInput
		}
	}
	return common.Unmarshal(raw, target)
}

var errArcaMCPConfirmation = errors.New("missing, expired, or invalid confirmation")

func arcaMCPToolError(err error) gin.H {
	message := "Management operation unavailable."
	switch {
	case errors.Is(err, errArcaFeedbackInput):
		message = "Invalid tool arguments."
	case errors.Is(err, model.ErrArcaFeedbackForbidden):
		message = "The token cannot perform this operation."
	case errors.Is(err, model.ErrArcaFeedbackConflict):
		message = "The topic version or idempotency key changed. Read the current state before retrying."
	case errors.Is(err, model.ErrArcaEntitlementConflict):
		message = "The subscription policy version changed. Read the current policy and preview again."
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, errArcaDiagnosticMissing):
		message = "The requested record is unavailable."
	case errors.Is(err, errArcaMCPConfirmation):
		message = "The confirmation is missing, expired, consumed, or bound to different arguments."
	case errors.Is(err, model.ErrArcaMCPTicketLimit):
		message = "This key has too many pending previews. Complete one or wait for expiry."
	}
	return arcaMCPToolResult(message, true)
}

func arcaMCPToolResult(content string, isError bool) gin.H {
	return gin.H{"isError": isError, "content": []gin.H{{"type": "text", "text": content}}}
}

func writeArcaMCPError(c *gin.Context, id common.RawMessage, code int, message string) {
	writeArcaMCPJSON(c, gin.H{"jsonrpc": "2.0", "id": id, "error": gin.H{"code": code, "message": message}})
}

func writeArcaMCPJSON(c *gin.Context, value any) {
	data, err := common.Marshal(value)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}
