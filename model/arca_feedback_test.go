package model

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestArcaFeedbackCommandTransaction(t *testing.T) {
	databases := []struct {
		name, dsnEnv, versionQuery string
		connect                    func(string) gorm.Dialector
	}{
		{"sqlite", "", "SELECT sqlite_version()", func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{"mysql", "TEST_MYSQL_DSN", "SELECT version()", func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{"postgres", "TEST_POSTGRES_DSN", "SELECT version()", func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	}
	for _, driver := range databases {
		t.Run(driver.name, func(t *testing.T) {
			var dsn string
			if driver.dsnEnv != "" {
				dsn = os.Getenv(driver.dsnEnv)
				if dsn == "" {
					t.Skip(driver.dsnEnv + " not configured")
				}
			}
			database, err := gorm.Open(driver.connect(dsn), &gorm.Config{
				NamingStrategy: schema.NamingStrategy{TablePrefix: "arca_forum_test_"},
			})
			require.NoError(t, err)
			sqlDB, err := database.DB()
			require.NoError(t, err)
			if driver.name == "sqlite" {
				sqlDB.SetMaxOpenConns(1)
			}
			previous := DB
			DB = database
			t.Cleanup(func() {
				DB = previous
				assert.NoError(t, database.Migrator().DropTable(&ArcaMCPConfirmation{}, &ArcaFeedbackNotification{}, &ArcaFeedbackEvent{}, &ArcaFeedbackIssue{}, &User{}))
				assert.NoError(t, sqlDB.Close())
			})
			var version string
			require.NoError(t, database.Raw(driver.versionQuery).Scan(&version).Error)
			t.Logf("database version: %s", version)
			require.NoError(t, database.AutoMigrate(&User{}))
			owner := User{Username: "forum-owner", AffCode: "own", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
			participant := User{Username: "forum-participant", AffCode: "part", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
			require.NoError(t, database.Create(&owner).Error)
			require.NoError(t, database.Create(&participant).Error)
			for range 2 {
				require.NoError(t, database.AutoMigrate(&ArcaFeedbackIssue{}, &ArcaFeedbackEvent{}, &ArcaFeedbackNotification{}, &ArcaMCPConfirmation{}))
			}

			ctx := context.Background()
			topic := ArcaFeedbackIssue{
				ID: uuid.NewString(), OwnerID: &owner.Id, Title: "Access is at 50%", Description: "Reproduction",
				Category: "access", Status: "open", Visibility: ArcaFeedbackPublic, Version: 1,
				CreationKey: uuid.NewString(), CreationHash: "topic-content", CreatedAt: 100, UpdatedAt: 100,
			}
			created, err := CreateArcaFeedback(ctx, topic, owner.Role)
			require.NoError(t, err)
			require.Positive(t, created.Issue.ActivitySeq)
			require.Equal(t, owner.Username, *created.Issue.OwnerUsername)
			page, err := ListArcaFeedback(ctx, ArcaFeedbackFilter{Search: "AT 50%", Limit: 20})
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			assert.Empty(t, page.Items[0].Description)
			empty, err := ListArcaFeedback(ctx, ArcaFeedbackFilter{Search: "missing%", Limit: 20})
			require.NoError(t, err)
			assert.Empty(t, empty.Items)

			reply := ArcaFeedbackEvent{
				IssueID: topic.ID, ActorID: &participant.Id, ActorRole: "user", Kind: "reply", Visibility: ArcaFeedbackPublic,
				Content: new("I can reproduce this."), ClientRequestID: new(uuid.NewString()), PayloadHash: "reply-content", CreatedAt: 101,
			}
			var outcomes [2]ArcaFeedbackMutation
			var failures [2]error
			var workers sync.WaitGroup
			for i := range outcomes {
				workers.Go(func() {
					outcomes[i], failures[i] = ApplyArcaFeedbackCommand(ctx, reply, 1, participant.Role)
				})
			}
			workers.Wait()
			require.NoError(t, failures[0])
			require.NoError(t, failures[1])
			assert.NotEqual(t, outcomes[0].Replayed, outcomes[1].Replayed)
			assert.EqualValues(t, 2, outcomes[0].Issue.Version)
			assert.EqualValues(t, 2, outcomes[1].Issue.Version)

			changedReply := reply
			changedReply.PayloadHash = "different"
			_, err = ApplyArcaFeedbackCommand(ctx, changedReply, 2, participant.Role)
			require.ErrorIs(t, err, ErrArcaFeedbackConflict)
			staleReply := reply
			staleReply.ClientRequestID = new(uuid.NewString())
			_, err = ApplyArcaFeedbackCommand(ctx, staleReply, 1, participant.Role)
			require.ErrorIs(t, err, ErrArcaFeedbackConflict)

			status := ArcaFeedbackEvent{
				IssueID: topic.ID, ActorID: &owner.Id, ActorRole: "admin", Kind: "status", Visibility: ArcaFeedbackPublic,
				ToStatus: new("closed"), ClientRequestID: new(uuid.NewString()), PayloadHash: "close-topic", CreatedAt: 102,
			}
			closed, err := ApplyArcaFeedbackCommand(ctx, status, 2, common.RoleAdminUser)
			require.NoError(t, err)
			assert.Equal(t, "closed", closed.Issue.Status)
			_, err = ApplyArcaFeedbackCommand(ctx, status, 2, common.RoleCommonUser)
			require.ErrorIs(t, err, ErrArcaFeedbackForbidden, "loss of current permission also forbids a replay")

			replayed, err := CreateArcaFeedback(ctx, topic, owner.Role)
			require.NoError(t, err)
			assert.True(t, replayed.Replayed)
			assert.EqualValues(t, 3, replayed.Issue.Version)
			assert.Equal(t, "closed", replayed.Issue.Status)

			detail, err := GetArcaFeedbackDetail(ctx, topic.ID, 0, 2)
			require.NoError(t, err)
			require.Len(t, detail.Events, 2)
			assert.True(t, detail.HasMore)
			assert.Equal(t, "created", detail.Events[0].Kind)
			assert.Equal(t, "reply", detail.Events[1].Kind)
			assert.Equal(t, participant.Username, *detail.Events[1].ActorUsername)

			final, err := GetArcaFeedbackDetail(ctx, topic.ID, detail.NextEventCursor, 20)
			require.NoError(t, err)
			require.Len(t, final.Events, 1)
			assert.False(t, final.HasMore)
			assert.Equal(t, "open", *final.Events[0].FromStatus)
			assert.Equal(t, "closed", *final.Events[0].ToStatus)

			ticket := ArcaMCPConfirmation{
				TicketHash:  AccessTokenFingerprint(uuid.NewString()),
				BindingHash: AccessTokenFingerprint(uuid.NewString()),
				ActorID:     owner.Id, TokenID: 17, ExpiresAt: common.GetTimestamp() + 300,
			}
			require.NoError(t, StoreArcaMCPConfirmation(ctx, ticket))
			wrongToken := ticket
			wrongToken.TokenID++
			used, err := ConsumeArcaMCPConfirmation(ctx, wrongToken)
			require.NoError(t, err)
			assert.False(t, used, "mismatched credential cannot consume the ticket")
			var consumed [2]bool
			for i := range consumed {
				workers.Go(func() {
					consumed[i], failures[i] = ConsumeArcaMCPConfirmation(ctx, ticket)
				})
			}
			workers.Wait()
			require.NoError(t, failures[0])
			require.NoError(t, failures[1])
			assert.NotEqual(t, consumed[0], consumed[1], "confirmation is consumed atomically")

		})
	}
}
