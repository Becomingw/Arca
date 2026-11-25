package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestArcaFeedbackUsesDashboardSession(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.UserSession{}, &model.UserAccessToken{}, &model.AuditLog{},
		&model.ArcaFeedbackIssue{}, &model.ArcaFeedbackEvent{}, &model.ArcaFeedbackNotification{},
	))
	user := model.User{
		Username: "forum-member", AffCode: "forum", Group: "default",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	session, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "forum-test")
	require.NoError(t, err)
	suffix, err := common.GenerateRandomCharsKey(43)
	require.NoError(t, err)
	rawPAT := model.AccessTokenPrefix + suffix
	pat := model.UserAccessToken{
		Name: "forum-test", TokenHash: model.AccessTokenFingerprint(rawPAT), TokenHint: model.AccessTokenHint(rawPAT),
	}
	require.NoError(t, pat.SetScopes([]string{"profile:read"}))
	require.NoError(t, model.CreateUserAccessToken(user.Id, &pat, service.AccessTokenMaxPerUser))

	engine := gin.New()
	SetApiRouter(engine)
	for _, tc := range []struct {
		name, credential string
		want             int
	}{
		{"guest", "", http.StatusUnauthorized},
		{"personal access token", rawPAT, http.StatusForbidden},
		{"dashboard session", session.AccessToken, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/arca/feedback", nil)
			if tc.credential != "" {
				req.Header.Set("Authorization", "Bearer "+tc.credential)
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			require.Contains(t, rec.Header().Get("Cache-Control"), "no-store")
		})
	}

	recipient := model.User{
		Username: "forum-recipient", AffCode: "frcv", Group: "default",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(&recipient).Error)
	recipientSession, err := service.CreateLoginSession(recipient.Id, "password", "127.0.0.1", "forum-test")
	require.NoError(t, err)
	body, err := common.Marshal(map[string]string{
		"title": "Model troubleshooting", "description": "@forum-recipient The upstream returned an error.",
		"category": "bug", "client_request_id": uuid.NewString(),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/arca/feedback", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "Model troubleshooting")

	req = httptest.NewRequest(http.MethodGet, "/api/arca/feedback/notifications?unread=1", nil)
	req.Header.Set("Authorization", "Bearer "+recipientSession.AccessToken)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var inbox struct {
		Success bool `json:"success"`
		Data    struct {
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &inbox))
	require.True(t, inbox.Success, rec.Body.String())
	require.Equal(t, 1, inbox.Data.UnreadCount)
}
