package model

import (
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestUserQuotaSummary(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				driver = postgres.Open(dsn)
			}

			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "quota_summary_test_"}})
			require.NoError(t, err)
			connection, err := db.DB()
			require.NoError(t, err)
			connection.SetMaxOpenConns(1)
			priorDB := DB
			DB = db
			t.Cleanup(func() {
				DB = priorDB
				assert.NoError(t, db.Migrator().DropTable(&QuotaData{}, &User{}))
				assert.NoError(t, connection.Close())
			})
			require.NoError(t, db.AutoMigrate(&User{}, &QuotaData{}))

			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database version: %s", version)

			owner := User{Username: "history-owner", AffCode: "owner", RequestCount: 10833, UsedQuota: 937}
			other := User{Username: "other-member", AffCode: "other", RequestCount: 1}
			empty := User{Username: "no-history", AffCode: "empty"}
			for _, member := range []*User{&owner, &other, &empty} {
				require.NoError(t, db.Create(member).Error)
			}
			require.NoError(t, db.Create(&[]QuotaData{
				{UserID: owner.Id, CreatedAt: 1767970800, TokenUsed: 3_000_000_000, Count: 10828, NodeName: "legacy-arca-history"},
				{UserID: owner.Id, CreatedAt: 1795759200, TokenUsed: 3868, Count: 5, Quota: 937},
				{UserID: other.Id, CreatedAt: 1795759200, TokenUsed: 500, Count: 1},
			}).Error)

			summary, err := GetUserQuotaSummary(owner.Id)
			require.NoError(t, err)
			assert.Equal(t, UserQuotaSummary{TotalTokens: 3_000_003_868, RequestCount: 10833}, summary)

			summary, err = GetUserQuotaSummary(empty.Id)
			require.NoError(t, err)
			assert.Equal(t, UserQuotaSummary{}, summary)
		})
	}
}
