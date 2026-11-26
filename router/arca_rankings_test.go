package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTeamRankingsProtectIdentitiesAndSmallCohorts(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.UserSession{}, &model.UserAccessToken{}, &model.AuditLog{},
		&model.ArcaProfile{}, &model.ArcaActivityWindow{}, &model.QuotaData{},
	))
	exportEnabled := common.DataExportEnabled
	common.DataExportEnabled = true
	t.Cleanup(func() { common.DataExportEnabled = exportEnabled })

	var users [4]model.User
	for i := range users {
		users[i] = model.User{
			Username: fmt.Sprintf("team-member-%d", i), AffCode: fmt.Sprintf("tm-%d", i),
			Group: "default", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
			AuthVersion: 1,
		}
		if i == 3 {
			users[i].Status = common.UserStatusDisabled
		}
		require.NoError(t, model.DB.Create(&users[i]).Error)
	}
	require.NoError(t, model.DB.Create(&model.ArcaProfile{UserID: users[0].Id, LeaderboardAnonymous: true}).Error)
	session, err := service.CreateLoginSession(users[1].Id, "password", "127.0.0.1", "team-test")
	require.NoError(t, err)

	engine := gin.New()
	SetApiRouter(engine)
	lookup := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/arca/team-activity?days=7", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res
	}
	guest := lookup("")
	require.Equal(t, http.StatusUnauthorized, guest.Code, guest.Body.String())
	require.Contains(t, guest.Header().Get("Cache-Control"), "no-store")
	require.NotContains(t, guest.Body.String(), "members")

	today := time.Now().UTC().Truncate(24 * time.Hour).Unix()
	window := func(i int, at int64) {
		t.Helper()
		require.NoError(t, model.DB.Create(&model.ArcaActivityWindow{UserID: users[i].Id, BucketAt: at}).Error)
	}
	window(0, today+3600)
	window(0, today-86400+3600)
	window(0, today-3*86400+3600)
	window(1, today+3600)
	window(3, today+3600) // disabled historical member must not count

	small := lookup(session.AccessToken)
	require.Equal(t, http.StatusOK, small.Code, small.Body.String())
	require.Contains(t, small.Body.String(), `"suppressed":true`)
	require.Contains(t, small.Body.String(), `"members":[]`)
	require.Contains(t, small.Body.String(), `"activity_bands":[]`)
	require.Contains(t, small.Body.String(), `"popular_models":[]`)

	window(2, today+3600)
	window(2, today+12*3600) // too few contributors to publish this time interval
	for i := range users {
		modelNames := []string{"shared-model"}
		if i != 2 {
			modelNames = append(modelNames, "hidden-model")
		}
		for _, name := range modelNames {
			require.NoError(t, model.DB.Create(&model.QuotaData{
				UserID: users[i].Id, ModelName: name, CreatedAt: today + 3600, Count: 2, TokenUsed: 7,
			}).Error)
		}
	}
	full := lookup(session.AccessToken)
	require.Equal(t, http.StatusOK, full.Code, full.Body.String())
	require.Contains(t, full.Header().Get("Cache-Control"), "no-store")
	require.NotContains(t, full.Body.String(), users[0].Username)
	require.NotContains(t, full.Body.String(), users[3].Username)
	require.NotContains(t, full.Body.String(), "hidden-model")
	require.NotContains(t, full.Body.String(), fmt.Sprint(today+3600))
	var report struct {
		Success bool `json:"success"`
		Data    struct {
			Suppressed bool `json:"suppressed"`
			Members    []struct {
				Anonymous        bool  `json:"anonymous"`
				ActiveDays       int   `json:"active_days"`
				LongestStreak    int   `json:"longest_streak"`
				ActiveToday      bool  `json:"active_today"`
				EstimatedMinutes int64 `json:"estimated_minutes"`
				Requests         int64 `json:"requests"`
				Tokens           int64 `json:"tokens"`
			} `json:"members"`
			Models []model.ArcaModelUsage   `json:"popular_models"`
			Bands  []model.ArcaActivityBand `json:"activity_bands"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(full.Body.Bytes(), &report))
	require.True(t, report.Success)
	require.False(t, report.Data.Suppressed)
	require.Len(t, report.Data.Members, 3)
	first := report.Data.Members[0]
	require.True(t, first.Anonymous)
	require.True(t, first.ActiveToday)
	require.Equal(t, 3, first.ActiveDays)
	require.Equal(t, 2, first.LongestStreak)
	require.EqualValues(t, 15, first.EstimatedMinutes)
	require.EqualValues(t, 4, first.Requests)
	require.EqualValues(t, 14, first.Tokens)
	require.Equal(t, []model.ArcaModelUsage{{Model: "shared-model", Requests: 6, Tokens: 21}}, report.Data.Models)
	require.Equal(t, []model.ArcaActivityBand{{StartSecond: 0, Windows: 5}}, report.Data.Bands)
}
