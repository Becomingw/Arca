package model

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestArcaUsageGrantsApplyOnlyToEligibleRequests(t *testing.T) {
	for _, config := range []struct {
		name, dsn    string
		databaseType common.DatabaseType
		open         func(string) gorm.Dialector
	}{
		{"sqlite", "", common.DatabaseTypeSQLite, func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{"mysql", "TEST_MYSQL_DSN", common.DatabaseTypeMySQL, func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{"postgres", "TEST_POSTGRES_DSN", common.DatabaseTypePostgreSQL, func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	} {
		t.Run(config.name, func(t *testing.T) {
			dsn := os.Getenv(config.dsn)
			if config.dsn != "" && dsn == "" {
				t.Skip(config.dsn + " not configured")
			}
			db, err := gorm.Open(config.open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "arca_grant_test_"}, Logger: logger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			if config.dsn == "" {
				sqlDB.SetMaxOpenConns(1)
			}
			oldDB, oldLog, oldRate := DB, LOG_DB, common.QuotaPerUnit
			oldSQL := common.MainDatabaseType()
			common.SetMainDatabaseType(config.databaseType)
			DB, LOG_DB, common.QuotaPerUnit = db, nil, 1_000_000
			tables := []any{&ArcaBudgetAccount{}, &ArcaBudgetReservation{}, &ArcaUsageGrant{}, &ArcaBudgetGrantUse{}, &ArcaEntitlementPolicyRow{}, &Redemption{}, &User{}}
			t.Cleanup(func() {
				DB, LOG_DB, common.QuotaPerUnit = oldDB, oldLog, oldRate
				common.SetMainDatabaseType(oldSQL)
				require.NoError(t, db.Migrator().DropTable(tables...))
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(tables...))
			testArcaUsageGrantLedger(t, db)
		})
	}
}

func testArcaUsageGrantLedger(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	const userID = 17
	key := "0d41869391034f38a07e404a6e171fd0"
	revision, err := SaveArcaEntitlementPolicy(ctx, 0, ArcaEntitlementPolicy{ModelFamilies: map[string]ArcaModelFamily{"text": {Name: "Text", Models: []string{"model-a"}}}})
	require.NoError(t, err)
	spec := &ArcaUsageGrantSpec{Family: "text", StartsAt: now.Add(time.Hour).Unix(), EndsAt: now.Add(4 * time.Hour).Unix(), ValidForSeconds: 3600}
	scope, err := ResolveArcaUsageGrantSpec(ctx, spec, nil, now)
	require.NoError(t, err)
	_, err = SaveArcaEntitlementPolicy(ctx, revision, ArcaEntitlementPolicy{ModelFamilies: map[string]ArcaModelFamily{"text": {Name: "Text", Models: []string{"model-b"}}}})
	require.NoError(t, err)
	scope, err = ResolveArcaUsageGrantSpec(ctx, spec, scope, now)
	require.NoError(t, err)
	require.Equal(t, []string{"model-a"}, scope.Models)
	code := Redemption{Key: key, Name: "Focused grant", Quota: 10, Grant: scope}
	require.NoError(t, db.Create(&code).Error)
	user := User{Id: userID, Username: "grant-user", Quota: 25}
	require.NoError(t, db.Create(&user).Error)
	grant, err := RedeemArcaUsageGrant(ctx, userID, key, now)
	require.NoError(t, err)
	require.Equal(t, int64(10), grant.RemainingMicros)
	require.Equal(t, now.Add(time.Hour).Unix(), grant.StartsAt)
	require.Equal(t, now.Add(2*time.Hour).Unix(), grant.EndsAt)
	_, err = RedeemArcaUsageGrant(ctx, userID+1, key, now)
	require.ErrorIs(t, err, ErrArcaGrantUnavailable)
	require.NoError(t, db.First(&user, userID).Error)
	require.Equal(t, 25, user.Quota)

	reserve := func(id string, quota int, alias string, at time.Time, windows []ArcaBudgetWindow) error {
		return ReserveArcaManagedBudget(ctx, userID, id, alias, quota, windows, at, false, ArcaManagedAuthorization{PublicAlias: alias})
	}
	active := now.Add(time.Hour)
	require.ErrorIs(t, reserve("early", 1, "model-a", now, nil), ErrArcaBudgetExceeded)
	require.ErrorIs(t, reserve("other-model", 1, "model-b", active, nil), ErrArcaBudgetExceeded)
	require.ErrorIs(t, reserve("exact-scope", 1, "model-a-high", active, nil), ErrArcaBudgetExceeded)
	require.ErrorIs(t, reserve("expired", 1, "model-a", now.Add(2*time.Hour), nil), ErrArcaBudgetExceeded)

	windows := []ArcaBudgetWindow{{Scope: "total", Period: "week", Start: now.Unix(), LimitMicros: 5}}
	require.NoError(t, reserve("base-and-grant", 9, "model-a", active, windows))
	balances, err := ListArcaUsageGrants(ctx, userID, active)
	require.NoError(t, err)
	require.Equal(t, int64(6), balances[0].RemainingMicros)
	require.NoError(t, FinishArcaBudget(ctx, userID, "base-and-grant", new(13)))
	balances, err = ListArcaUsageGrants(ctx, userID, active)
	require.NoError(t, err)
	require.Equal(t, int64(2), balances[0].RemainingMicros)
	used, err := ReadArcaBudgetUsage(ctx, userID, windows, active)
	require.NoError(t, err)
	require.Equal(t, int64(5), used[0].UsedMicros)

	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := range 2 {
		wg.Go(func() { outcomes <- reserve(fmt.Sprintf("parallel-%d", i), 2, "model-a", active, windows) })
	}
	wg.Wait()
	close(outcomes)
	var allowed int
	for err := range outcomes {
		if err == nil {
			allowed++
			continue
		}
		require.ErrorIs(t, err, ErrArcaBudgetExceeded)
	}
	require.Equal(t, 1, allowed)
	for i := range 2 {
		require.NoError(t, FinishArcaBudget(ctx, userID, fmt.Sprintf("parallel-%d", i), nil))
	}
	windows[0].LimitMicros = 10
	require.NoError(t, reserve("actual-exceeds-quote", 5, "model-a", active, windows))
	balances, err = ListArcaUsageGrants(ctx, userID, active)
	require.NoError(t, err)
	require.Equal(t, int64(2), balances[0].RemainingMicros)
	require.NoError(t, FinishArcaBudget(ctx, userID, "actual-exceeds-quote", new(7)))
	balances, err = ListArcaUsageGrants(ctx, userID, active)
	require.NoError(t, err)
	require.Zero(t, balances[0].RemainingMicros)
	used, err = ReadArcaBudgetUsage(ctx, userID, windows, active)
	require.NoError(t, err)
	require.Equal(t, int64(10), used[0].UsedMicros)
}
