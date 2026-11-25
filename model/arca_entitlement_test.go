package model

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
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

func TestArcaEntitlementsDatabaseMatrix(t *testing.T) {
	for _, driver := range []struct {
		name, dsnEnv, versionQuery string
		open                       func(string) gorm.Dialector
	}{
		{"sqlite", "", "SELECT sqlite_version()", func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{"mysql", "TEST_MYSQL_DSN", "SELECT version()", func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{"postgres", "TEST_POSTGRES_DSN", "SELECT version()", func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	} {
		t.Run(driver.name, func(t *testing.T) {
			dsn := os.Getenv(driver.dsnEnv)
			if driver.dsnEnv != "" && dsn == "" {
				t.Skip(driver.dsnEnv + " not configured")
			}
			database, err := gorm.Open(driver.open(dsn), &gorm.Config{
				NamingStrategy: schema.NamingStrategy{TablePrefix: "arca_ent_test_"},
				Logger:         logger.Discard,
			})
			require.NoError(t, err)
			sqlDB, err := database.DB()
			require.NoError(t, err)
			if driver.name == "sqlite" {
				sqlDB.SetMaxOpenConns(1)
			}
			previous, priorLog := DB, LOG_DB
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			types := map[string]common.DatabaseType{
				"sqlite": common.DatabaseTypeSQLite, "mysql": common.DatabaseTypeMySQL, "postgres": common.DatabaseTypePostgreSQL,
			}
			DB, LOG_DB = database, database
			common.SetDatabaseTypes(types[driver.name], types[driver.name])
			t.Cleanup(func() {
				DB, LOG_DB = previous, priorLog
				common.SetDatabaseTypes(oldMainType, oldLogType)
				assert.NoError(t, database.Migrator().DropTable(&Task{}, &Log{}, &ArcaBudgetReservation{}, &ArcaBudgetAccount{}, &ArcaEntitlementPolicyRow{}))
				assert.NoError(t, sqlDB.Close())
			})
			var version string
			require.NoError(t, database.Raw(driver.versionQuery).Scan(&version).Error)
			t.Logf("database version: %s", version)
			for range 2 {
				require.NoError(t, database.AutoMigrate(&ArcaEntitlementPolicyRow{}, &ArcaBudgetAccount{}, &ArcaBudgetReservation{}, &Log{}, &Task{}))
			}

			ctx := context.Background()
			policy := ArcaEntitlementPolicy{
				Timezone: "Asia/Taipei",
				Organizations: map[string]ArcaOrganization{
					"science": {Name: "Science", Members: []int{17, 20}, ModelAccess: ArcaModelAccess{Mode: "specific", Models: []string{"model-a"}}},
					"clinic":  {Name: "Clinic", Members: []int{17}, ModelAccess: ArcaModelAccess{Mode: "specific", Models: []string{"model-b"}}},
				},
				Groups: map[string]ArcaAllocation{
					"science": {Monthly: new(5.0), Families: map[string]ArcaSubPool{"text": {FiveHour: new(0.000010)}}},
					"clinic":  {Monthly: new(10.0)},
				},
				Users:           map[string]ArcaAllocation{"20": {Weekly: new(7.0)}},
				UserModelAccess: map[string]ArcaModelAccess{"20": {Mode: "deny_all"}},
				ModelFamilies:   map[string]ArcaModelFamily{"text": {Name: "Text", Models: []string{"model-a", "model-b"}}},
			}
			next, err := SaveArcaEntitlementPolicy(ctx, 0, policy)
			require.NoError(t, err)
			require.EqualValues(t, 1, next)
			_, err = SaveArcaEntitlementPolicy(ctx, 0, policy)
			require.ErrorIs(t, err, ErrArcaEntitlementConflict)
			policy, next, err = LoadArcaEntitlementPolicy(ctx)
			require.NoError(t, err)
			require.EqualValues(t, 1, next)
			effective := policy.Effective(17)
			require.Equal(t, []string{"clinic", "science"}, effective.GroupIDs)
			require.Equal(t, "groups", effective.Source)
			require.Equal(t, 10.0, *effective.Allocation.Monthly)
			require.True(t, effective.ModelAccess.Allows("model-a"))
			require.True(t, effective.ModelAccess.Allows("model-b"))
			require.False(t, effective.ModelAccess.Allows("model-c"))
			override := policy.Effective(20)
			require.Equal(t, "user", override.Source)
			require.Equal(t, 7.0, *override.Allocation.Weekly)
			require.Empty(t, override.Allocation.Families)
			require.False(t, override.ModelAccess.Allows("model-a"))

			now := time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)
			var budgetErr [2]error
			var workers sync.WaitGroup
			for i, alias := range []string{"model-a", "model-b"} {
				windows, err := policy.Windows(effective, alias, now)
				require.NoError(t, err)
				require.Len(t, windows, 2)
				workers.Go(func() {
					budgetErr[i] = ReserveArcaBudget(ctx, 17, fmt.Sprintf("request-%d", i), alias, 3, windows, now)
				})
			}
			workers.Wait()
			admitted := "request-0"
			if budgetErr[0] != nil {
				admitted = "request-1"
				require.ErrorIs(t, budgetErr[0], ErrArcaBudgetExceeded)
				require.NoError(t, budgetErr[1])
			} else {
				require.ErrorIs(t, budgetErr[1], ErrArcaBudgetExceeded)
			}
			windows, err := policy.Windows(effective, "model-a", now)
			require.NoError(t, err)
			usage, err := ReadArcaBudgetUsage(ctx, 17, windows, now)
			require.NoError(t, err)
			require.EqualValues(t, 6, usage[1].ReservedMicros)
			require.NoError(t, FinishArcaBudget(ctx, 17, admitted, new(2)))
			require.NoError(t, FinishArcaBudget(ctx, 17, admitted, new(3)))
			usage, err = ReadArcaBudgetUsage(ctx, 17, windows, now)
			require.NoError(t, err)
			require.EqualValues(t, 4, usage[1].UsedMicros)
			require.Zero(t, usage[1].ReservedMicros)
			require.NoError(t, ReserveArcaBudget(ctx, 17, "replacement", "model-a", 3, windows, now))
			require.NoError(t, FinishArcaBudget(ctx, 17, "replacement", nil))
			require.NoError(t, ReserveArcaBudget(ctx, 17, "after-refund", "model-a", 3, windows, now))

			future := now.Add(25 * time.Hour)
			futureWindows, err := policy.Windows(effective, "model-a", future)
			require.NoError(t, err)
			usage, err = ReadArcaBudgetUsage(ctx, 17, futureWindows, future)
			require.NoError(t, err)
			require.EqualValues(t, 6, usage[0].ReservedMicros, "a crashed in-flight request retains its estimated cost")
			require.Zero(t, usage[1].ReservedMicros, "the completed five-hour window does not retain the estimate")
			require.ErrorIs(t, ResizeArcaBudget(ctx, 17, "after-refund", "model-a", 4, futureWindows, future), ErrArcaEntitlementConflict)

			require.NoError(t, FinishArcaTaskBudget(ctx, 17, admitted, new(1)))
			require.NoError(t, FinishArcaBudget(ctx, 17, admitted, new(4)), "late submission settlement cannot overwrite the terminal task charge")
			usage, err = ReadArcaBudgetUsage(ctx, 17, futureWindows, future)
			require.NoError(t, err)
			require.EqualValues(t, 2, usage[0].UsedMicros)

			// An unrelated or unmarked consume log cannot settle the ledger.
			require.NoError(t, ReserveArcaBudget(ctx, 17, "unmarked", "model-a", 3, nil, now))
			logContext, _ := gin.CreateTestContext(nil)
			MarkArcaBudgetChargeFact(logContext, 17, "after-refund", 5)
			charge := attachArcaBudgetChargeFact(logContext, 17, "after-refund", 5, nil)
			require.NotNil(t, charge)
			require.Nil(t, attachArcaBudgetChargeFact(logContext, 18, "after-refund", 5, nil))
			require.Nil(t, attachArcaBudgetChargeFact(logContext, 17, "after-refund", 7, nil))
			require.NoError(t, database.Create(&[]Log{
				{UserId: 17, Type: LogTypeConsume, RequestId: "after-refund", Quota: 5, Other: charge.JSONString(), CreatedAt: now.Unix()},
				{UserId: 17, Type: LogTypeConsume, RequestId: "unmarked", Quota: 99, CreatedAt: now.Unix()},
				{UserId: 18, Type: LogTypeConsume, RequestId: "after-refund", Quota: 5, Other: charge.JSONString(), CreatedAt: now.Unix()},
			}).Error)
			require.NoError(t, ReserveArcaBudget(ctx, 17, "reconcile-with-log", "model-a", 1, futureWindows, future))
			usage, err = ReadArcaBudgetUsage(ctx, 17, futureWindows, future)
			require.NoError(t, err)
			require.EqualValues(t, 12, usage[0].UsedMicros)
			require.EqualValues(t, 8, usage[0].ReservedMicros)

			// A process can exit immediately after the terminal CAS. The
			// organization amount must already be durable at that boundary.
			task := Task{
				TaskID: "arca-terminal-boundary", UserId: 17, Quota: 3, Status: TaskStatusInProgress,
				PrivateData: TaskPrivateData{
					ArcaBudgetTracked: true,
					Execution:         &TaskExecutionSnapshot{RequestID: "terminal-budget-request"},
				},
			}
			require.NoError(t, ReserveArcaBudget(ctx, 17, "terminal-budget-request", "model-a", 3, nil, now))
			require.NoError(t, task.Insert())
			task.Status = TaskStatusSuccess
			won, err := task.UpdateWithStatusAndArcaBudget(ctx, TaskStatusInProgress, new(-1))
			require.Error(t, err)
			require.False(t, won)
			var persisted Task
			require.NoError(t, database.First(&persisted, task.ID).Error)
			require.Equal(t, TaskStatus(TaskStatusInProgress), persisted.Status)
			won, err = task.UpdateWithStatusAndArcaBudget(ctx, TaskStatusInProgress, new(4))
			require.NoError(t, err)
			require.True(t, won)
			require.NoError(t, database.First(&persisted, task.ID).Error)
			require.Equal(t, TaskStatus(TaskStatusSuccess), persisted.Status)
			var settled ArcaBudgetReservation
			require.NoError(t, database.Where("user_id = ? AND request_id = ?", 17, "terminal-budget-request").Take(&settled).Error)
			require.Equal(t, arcaBudgetSettled, settled.Status)
			require.EqualValues(t, 8, settled.CostMicros)
			task.Status = TaskStatusFailure
			won, err = task.UpdateWithStatusAndArcaBudget(ctx, TaskStatusInProgress, nil)
			require.NoError(t, err)
			require.False(t, won)
			require.NoError(t, database.Where("user_id = ? AND request_id = ?", 17, "terminal-budget-request").Take(&settled).Error)
			require.EqualValues(t, 8, settled.CostMicros)

			instant := Task{
				TaskID: "arca-inline-boundary", UserId: 17, Quota: 2, Status: TaskStatusSuccess,
				PrivateData: TaskPrivateData{
					ArcaBudgetTracked: true,
					Execution:         &TaskExecutionSnapshot{RequestID: "inline-budget-request"},
				},
			}
			require.NoError(t, ReserveArcaBudget(ctx, 17, "inline-budget-request", "model-a", 3, nil, now))
			require.NoError(t, instant.Insert())
			var inlineBudget ArcaBudgetReservation
			require.NoError(t, database.Where("user_id = ? AND request_id = ?", 17, "inline-budget-request").Take(&inlineBudget).Error)
			require.Equal(t, arcaBudgetSettled, inlineBudget.Status)
			require.EqualValues(t, 4, inlineBudget.CostMicros)
		})
	}
}
