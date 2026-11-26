package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestArcaRequestDiagnosticsWithSeparateLogDatabase(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
	}{
		{name: "sqlite"},
		{name: "mysql", env: "TEST_ARCA_DIAGNOSTICS_MYSQL_DSN"},
		{name: "postgres", env: "TEST_ARCA_DIAGNOSTICS_POSTGRES_DSN"},
		{name: "clickhouse", env: "TEST_ARCA_DIAGNOSTICS_CLICKHOUSE_DSN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := "local"
			if tc.env != "" {
				dsn = os.Getenv(tc.env)
				if dsn == "" {
					t.Skipf("set %s to an isolated log database", tc.env)
				}
			}

			setupRelayRouterTestDB(t)
			require.NoError(t, model.DB.AutoMigrate(
				&model.UserSession{}, &model.ArcaFeedbackIssue{},
				&model.ArcaFeedbackEvent{}, &model.ArcaFeedbackNotification{},
			))
			t.Setenv("LOG_SQL_DSN", dsn)
			t.Setenv("LOG_SQL_CLICKHOUSE_TTL_DAYS", "0")
			mainPath := common.SQLitePath
			common.SQLitePath = fmt.Sprintf("file:%s-dedicated-logs?mode=memory&cache=shared", tc.name)
			common.IsMasterNode = true
			initErr := model.InitLogDB()
			common.IsMasterNode = false
			common.SQLitePath = mainPath
			require.NoError(t, initErr)
			logDB, err := model.LOG_DB.DB()
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, logDB.Close())
				model.LOG_DB = model.DB
			})

			owner := model.User{
				Username: "diagnostic-owner", AffCode: "diago", Group: "default",
				Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1,
			}
			other := model.User{
				Username: "diagnostic-other", AffCode: "diagx", Group: "default",
				Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1,
			}
			require.NoError(t, model.DB.Create(&owner).Error)
			require.NoError(t, model.DB.Create(&other).Error)
			ownerSession, err := service.CreateLoginSession(owner.Id, "password", "127.0.0.1", "diagnostic-test")
			require.NoError(t, err)
			otherSession, err := service.CreateLoginSession(other.Id, "password", "127.0.0.1", "diagnostic-test")
			require.NoError(t, err)
			requestID, conflictingID, missingID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			now := common.GetTimestamp()
			const privateEvidence = "private-evidence-canary"
			require.NoError(t, model.LOG_DB.Create(&[]model.Log{
				{RequestId: requestID, UserId: owner.Id, CreatedAt: now, Type: model.LogTypeError, ChannelId: 5, Content: privateEvidence, UpstreamRequestId: privateEvidence, ModelName: privateEvidence},
				{RequestId: requestID, UserId: owner.Id, CreatedAt: now, Type: model.LogTypeConsume, IsStream: true, Other: privateEvidence, TokenName: privateEvidence, Ip: privateEvidence},
				{RequestId: conflictingID, UserId: owner.Id, CreatedAt: now, Type: model.LogTypeConsume},
				{RequestId: conflictingID, UserId: other.Id, CreatedAt: now, Type: model.LogTypeError},
			}).Error)

			engine := gin.New()
			SetApiRouter(engine)
			request := func(method, path, bearer, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				if bearer != "" {
					req.Header.Set("Authorization", "Bearer "+bearer)
				}
				rec := httptest.NewRecorder()
				engine.ServeHTTP(rec, req)
				return rec
			}
			diagnosticPath := func(id string) string { return "/api/arca/requests/" + id + "/diagnostics" }
			rec := request(http.MethodGet, diagnosticPath(requestID), "", "")
			require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			rec = request(http.MethodGet, diagnosticPath(requestID), ownerSession.AccessToken, "")
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Contains(t, rec.Header().Get("Cache-Control"), "no-store")
			require.NotContains(t, rec.Body.String(), privateEvidence)
			var evidence struct {
				Success bool `json:"success"`
				Data    struct {
					FinalOutcome    string `json:"final_outcome"`
					AttemptFailures bool   `json:"has_attempt_failures"`
					Summary         string `json:"summary"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &evidence))
			require.True(t, evidence.Success)
			require.Equal(t, "unknown", evidence.Data.FinalOutcome)
			require.True(t, evidence.Data.AttemptFailures)
			summary := evidence.Data.Summary
			require.NotEmpty(t, summary)

			for _, probe := range []struct{ id, bearer string }{
				{requestID, otherSession.AccessToken},
				{conflictingID, ownerSession.AccessToken},
				{missingID, ownerSession.AccessToken},
			} {
				rec = request(http.MethodGet, diagnosticPath(probe.id), probe.bearer, "")
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &evidence))
				require.Equal(t, "unknown", evidence.Data.FinalOutcome)
				require.False(t, evidence.Data.AttemptFailures)
				require.NotEqual(t, summary, evidence.Data.Summary)
				require.NotContains(t, rec.Body.String(), privateEvidence)
			}

			type topic struct {
				Description string `json:"description"`
				RequestID   string `json:"request_id"`
			}
			submit := func(bearer, reference, key string, include bool) *httptest.ResponseRecorder {
				body, err := common.Marshal(map[string]any{
					"title": "Troubleshooting request", "description": "My report.", "category": "bug",
					"request_id": reference, "client_request_id": key, "include_diagnostics": include,
				})
				require.NoError(t, err)
				return request(http.MethodPost, "/api/arca/feedback", bearer, string(body))
			}

			rec = submit(otherSession.AccessToken, requestID, uuid.NewString(), false)
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			rec = submit(ownerSession.AccessToken, missingID, uuid.NewString(), true)
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

			var result struct {
				Data struct {
					Issue    topic `json:"issue"`
					Replayed bool  `json:"replayed"`
				} `json:"data"`
			}
			rec = submit(ownerSession.AccessToken, requestID, uuid.NewString(), false)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
			require.Equal(t, topic{Description: "My report.", RequestID: requestID}, result.Data.Issue)

			key := uuid.NewString()
			rec = submit(ownerSession.AccessToken, requestID, key, true)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
			require.False(t, result.Data.Replayed)
			require.Contains(t, result.Data.Issue.Description, "My report.")
			require.Contains(t, result.Data.Issue.Description, summary)
			require.NotContains(t, rec.Body.String(), privateEvidence)

			original := result.Data.Issue
			rec = submit(ownerSession.AccessToken, requestID, key, true)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
			require.True(t, result.Data.Replayed)
			require.Equal(t, original, result.Data.Issue)

			savedLogDB := model.LOG_DB
			model.LOG_DB = nil
			rec = submit(ownerSession.AccessToken, requestID, key, true)
			model.LOG_DB = savedLogDB
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
			require.True(t, result.Data.Replayed)
			require.Equal(t, original, result.Data.Issue)
		})
	}
}
