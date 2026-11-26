package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestArcaCCSwitchUsage(t *testing.T) {
	for _, dialect := range []struct {
		name string
		env  string
		kind common.DatabaseType
		open func(string) gorm.Dialector
	}{
		{"sqlite", "", common.DatabaseTypeSQLite, func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{"mysql", "TEST_MYSQL_DSN", common.DatabaseTypeMySQL, func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{"postgres", "TEST_POSTGRES_DSN", common.DatabaseTypePostgreSQL, func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	} {
		t.Run(dialect.name, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip(dialect.env + " not configured")
			}
			db, err := gorm.Open(dialect.open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "arca_ccswitch_test_"}, Logger: logger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			if dialect.env == "" {
				sqlDB.SetMaxOpenConns(1)
			}
			tables := []any{&model.User{}, &model.Token{}, &model.Ability{}, &model.ArcaEntitlementPolicyRow{},
				&model.ArcaBudgetAccount{}, &model.ArcaBudgetReservation{}, &model.ArcaUsageGrant{}, &model.ArcaBudgetGrantUse{}}
			oldDB, oldLog, oldRate := model.DB, model.LOG_DB, common.QuotaPerUnit
			oldDialect, oldRedis, oldSelfUse := common.MainDatabaseType(), common.RedisEnabled, operation_setting.SelfUseModeEnabled
			common.SetMainDatabaseType(dialect.kind)
			model.DB, model.LOG_DB, common.QuotaPerUnit = db, nil, 1_000_000
			common.RedisEnabled, operation_setting.SelfUseModeEnabled = false, true
			t.Cleanup(model.EnableArcaEntitlementRuntime())
			t.Cleanup(func() {
				model.DB, model.LOG_DB, common.QuotaPerUnit = oldDB, oldLog, oldRate
				common.SetMainDatabaseType(oldDialect)
				common.RedisEnabled, operation_setting.SelfUseModeEnabled = oldRedis, oldSelfUse
				require.NoError(t, db.Migrator().DropTable(tables...))
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(tables...))
			testArcaCCSwitchUsage(t, db)
		})
	}
}

func testArcaCCSwitchUsage(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	user := model.User{Username: "eligible-owner", Group: "test-cc", Status: common.UserStatusEnabled, Quota: 123}
	other := model.User{Username: "different-owner", Group: "test-cc", Status: common.UserStatusEnabled, AffCode: "cc-other"}
	banned := model.User{Username: "disabled-owner", Group: "test-cc", Status: common.UserStatusDisabled, AffCode: "cc-banned"}
	require.NoError(t, db.Create(&[]*model.User{&user, &other, &banned}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: user.Group, Model: "visible-model", ChannelId: 1, Enabled: true},
		{Group: user.Group, Model: "hidden-model", ChannelId: 1, Enabled: true},
	}).Error)

	plans := model.ArcaEntitlementPolicy{
		Timezone: "Asia/Shanghai",
		Plans: map[string]model.ArcaSubscriptionPlan{
			"member": {Name: "Research plan", ModelAccess: model.ArcaModelAccess{Mode: "specific", Models: []string{"visible-model", "hidden-model"}},
				Allocation: model.ArcaAllocation{FiveHour: new(3.0), Daily: new(5.0),
					Models:   map[string]model.ArcaSubPool{"hidden-model": {Monthly: new(10.0)}},
					Families: map[string]model.ArcaSubPool{"shared": {Weekly: new(9.0)}}}},
			"group": {Name: "Group Research plan", ModelAccess: model.ArcaModelAccess{Mode: "unrestricted"},
				Allocation: model.ArcaAllocation{Daily: new(7.0)}},
		},
		ModelFamilies:      map[string]model.ArcaModelFamily{"shared": {Name: "Shared", Models: []string{"visible-model", "hidden-model"}}},
		UserSubscriptions:  map[string]string{strconv.Itoa(user.Id): "member"},
		GroupSubscriptions: map[string]string{user.Group: "group"},
	}
	_, err := model.SaveArcaEntitlementPolicy(ctx, 0, plans)
	require.NoError(t, err)
	effective := plans.EffectiveForGroup(user.Id, user.Group)
	for _, entry := range []struct {
		id, alias string
		quota     int
		settled   bool
	}{
		{"settled-other-key", "hidden-model", 200_000, true},
		{"reserved-request", "visible-model", 100_000, false},
	} {
		windows, err := plans.Windows(effective, entry.alias, now)
		require.NoError(t, err)
		require.NoError(t, model.ReserveArcaBudget(ctx, user.Id, entry.id, entry.alias, entry.quota, windows, now))
		if entry.settled {
			require.NoError(t, model.FinishArcaBudget(ctx, user.Id, entry.id, &entry.quota))
		}
	}
	require.NoError(t, db.Create(&[]model.ArcaUsageGrant{
		{ID: 1, UserID: user.Id, Name: "Scoped future grant", Models: []string{"visible-model", "hidden-model"},
			StartsAt: now.Add(time.Hour).Unix(), EndsAt: now.Add(2 * time.Hour).Unix(), AmountMicros: 1_300_000},
		{ID: 2, UserID: user.Id, Name: "Hidden model grant", Models: []string{"hidden-model"},
			StartsAt: now.Add(-time.Hour).Unix(), AmountMicros: 500_000},
		{ID: 3, UserID: other.Id, Name: "Another user's grant", StartsAt: now.Add(-time.Hour).Unix(), AmountMicros: 900_000},
		{ID: 4, UserID: user.Id, Name: "Expired grant", StartsAt: now.Add(-2 * time.Hour).Unix(),
			EndsAt: now.Add(-time.Hour).Unix(), AmountMicros: 700_000},
	}).Error)

	ipLimit := "203.0.113.10"
	base := model.Token{UserId: user.Id, Status: common.TokenStatusEnabled, ExpiredTime: -1, AllowUserQuotaQuery: true,
		ModelLimitsEnabled: true, ModelLimits: "visible-model"}
	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	SetApiRouter(engine)
	for _, tc := range []struct {
		name   string
		change func(*model.Token)
		status int
	}{
		{"active", nil, http.StatusOK},
		{"exhausted", func(token *model.Token) { token.Status = common.TokenStatusExhausted }, http.StatusOK},
		{"opt-out", func(token *model.Token) { token.AllowUserQuotaQuery = false }, http.StatusForbidden},
		{"expired-time", func(token *model.Token) { token.ExpiredTime = now.Add(-time.Minute).Unix() }, http.StatusUnauthorized},
		{"expired-status", func(token *model.Token) { token.Status = common.TokenStatusExpired }, http.StatusUnauthorized},
		{"revoked", func(token *model.Token) { token.Status = common.TokenStatusDisabled }, http.StatusUnauthorized},
		{"restricted-ip", func(token *model.Token) { token.AllowIps = &ipLimit }, http.StatusForbidden},
		{"disabled-owner", func(token *model.Token) { token.UserId = banned.Id }, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := base
			token.Key = "ccswitch-test-" + tc.name
			if tc.change != nil {
				tc.change(&token)
			}
			require.NoError(t, db.Create(&token).Error)
			request := httptest.NewRequest(http.MethodGet, "/api/ccswitch/usage?user_id="+strconv.Itoa(other.Id), nil)
			request.Header.Set("Authorization", "Bearer "+token.GetFullKey())
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
			assert.Contains(t, response.Header().Values("Vary"), "Authorization")
			if tc.status != http.StatusOK {
				assert.NotContains(t, response.Body.String(), plans.Plans["member"].Name)
				return
			}
			var body struct {
				Success bool
				Data    struct {
					SubscriptionName string `json:"subscription_name"`
					State            string
					Timezone         string
					Windows          []model.ArcaBudgetUsage
					Grants           []model.ArcaUsageGrantBalance
				}
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			assert.True(t, body.Success)
			assert.Equal(t, "Research plan", body.Data.SubscriptionName)
			assert.Equal(t, "Asia/Shanghai", body.Data.Timezone)
			assert.Equal(t, "active", body.Data.State)
			require.Len(t, body.Data.Windows, 3)
			for _, window := range body.Data.Windows {
				assert.EqualValues(t, 200_000, window.UsedMicros)
				assert.EqualValues(t, 100_000, window.ReservedMicros)
				assert.Equal(t, window.LimitMicros-300_000, window.RemainingMicros)
				switch window.Period {
				case "rolling_5h":
					assert.Zero(t, window.End)
				case "week":
					assert.Equal(t, "shared", window.Name)
					assert.Equal(t, []string{"visible-model"}, window.Models)
					assert.Greater(t, window.End, now.Unix())
				case "day":
					assert.Greater(t, window.End, now.Unix())
				default:
					t.Errorf("unexpected window %s", window.Period)
				}
			}
			require.Len(t, body.Data.Grants, 1)
			assert.Equal(t, "Scoped future grant", body.Data.Grants[0].Name)
			assert.Equal(t, []string{"visible-model"}, body.Data.Grants[0].Models)
			assert.Greater(t, body.Data.Grants[0].StartsAt, now.Unix())
			assert.EqualValues(t, 1_300_000, body.Data.Grants[0].RemainingMicros)
			for _, field := range []string{"hidden-model", "wallet", "Another user's grant", token.Key} {
				assert.NotContains(t, response.Body.String(), field)
			}
		})
	}
	request := httptest.NewRequest(http.MethodGet, "/api/ccswitch/usage?model=hidden-model", nil)
	request.Header.Set("Authorization", "Bearer sk-ccswitch-test-active")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	assert.Equal(t, http.StatusForbidden, response.Code)
	for _, auth := range []string{"", "sk-ccswitch-test-active", "Bearer sk-CCSwitch-test-active", "Bearer sk-ccswitch-test-active-123"} {
		request := httptest.NewRequest(http.MethodGet, "/api/ccswitch/usage", nil)
		request.Header.Set("Authorization", auth)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	}
}
