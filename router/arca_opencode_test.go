package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArcaOpenCodeConfig(t *testing.T) {
	for _, tc := range []struct {
		name, variable string
	}{
		{name: "sqlite"}, {name: "mysql", variable: "TEST_ARCA_OPENCODE_MYSQL_DSN"},
		{name: "postgres", variable: "TEST_ARCA_OPENCODE_POSTGRES_DSN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.variable != "" && os.Getenv(tc.variable) == "" {
				t.Skip("Set an isolated test database DSN in " + tc.variable)
			}
			setupRelayRouterTestDB(t)
			if tc.variable != "" {
				sqliteDB, err := model.DB.DB()
				require.NoError(t, err)
				t.Cleanup(func() {
					require.NoError(t, sqliteDB.Close())
				})
				require.NoError(t, os.Setenv("SQL_DSN", os.Getenv(tc.variable)))
				require.NoError(t, model.InitDB())
				model.LOG_DB = model.DB
			}
			require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}, &model.Channel{}))

			previousAddress, previousMode := system_setting.ServerAddress, operation_setting.SelfUseModeEnabled
			system_setting.ServerAddress, operation_setting.SelfUseModeEnabled = "https://arca.example/infra", true
			t.Cleanup(func() {
				system_setting.ServerAddress, operation_setting.SelfUseModeEnabled = previousAddress, previousMode
			})
			suffix, err := common.GenerateRandomCharsKey(10)
			require.NoError(t, err)
			user := model.User{Username: "opencode-" + suffix, AffCode: suffix[:4], Status: common.UserStatusEnabled, Group: "opencode-" + suffix}
			require.NoError(t, model.DB.Create(&user).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Unscoped().Delete(&user).Error) })

			channel := model.Channel{Type: constant.ChannelTypeOpenAI, Key: "do-not-expose-provider-key", Status: common.ChannelStatusEnabled}
			require.NoError(t, model.DB.Create(&channel).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Unscoped().Delete(&channel).Error) })
			aliases := []string{"allowed", "disallowed", "text-embedding-3-small", "{env:LOCAL_SECRET}"}
			for index, alias := range aliases {
				ability := model.Ability{Group: user.Group, Model: alias, ChannelId: channel.Id, Enabled: true}
				require.NoError(t, model.DB.Create(&ability).Error)
				t.Cleanup(func() {
					require.NoError(t, model.DB.Where("channel_id = ? AND model = ?", channel.Id, aliases[index]).Delete(&model.Ability{}).Error)
				})
			}
			token := model.Token{UserId: user.Id, Key: suffix, Status: common.TokenStatusEnabled, ExpiredTime: -1,
				UnlimitedQuota: true, ModelLimitsEnabled: true, ModelLimits: "allowed,text-embedding-3-small,{env:LOCAL_SECRET}"}
			require.NoError(t, model.DB.Create(&token).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Unscoped().Delete(&token).Error) })

			engine := gin.New()
			SetApiRouter(engine)
			for _, key := range []string{"", "nap_unowned", "sk-" + suffix} {
				request := httptest.NewRequest(http.MethodGet, "/api/opencode/config", nil)
				request.Host = "untrusted.example"
				request.Header.Set("X-Forwarded-Host", "forwarded.example")
				if key != "" {
					request.Header.Set("Authorization", "Bearer "+key)
				}
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
				assert.Contains(t, fmt.Sprint(response.Header().Values("Vary")), "Authorization")
				if key != "sk-"+suffix {
					assert.Equal(t, http.StatusUnauthorized, response.Code)
					continue
				}
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.JSONEq(t, `{
					"$schema": "https://opencode.ai/config.json",
					"provider": {"aether": {
						"name": "Arca", "npm": "@ai-sdk/openai-compatible",
						"options": {"baseURL":"https://arca.example/infra/v1"},
						"models":{"allowed":{"id":"allowed","name":"allowed"}}
					}}
				}`, response.Body.String())
			}

			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/opencode/setup", nil))
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, `attachment; filename="opencode-sync.py"`, response.Header().Get("Content-Disposition"))
			assert.Contains(t, response.Body.String(), "getpass.getpass")
			assert.NotContains(t, response.Body.String(), token.GetFullKey())
		})
	}
}
