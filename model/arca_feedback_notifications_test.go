package model

import (
	"context"
	"errors"
	"os"
	"strings"
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

func TestArcaFeedbackNotificationsDatabaseMatrix(t *testing.T) {
	for _, database := range []struct {
		dialect common.DatabaseType
		dsnEnv  string
		open    func(string) gorm.Dialector
	}{
		{common.DatabaseTypeSQLite, "", func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{common.DatabaseTypeMySQL, "TEST_MYSQL_DSN", mysql.Open},
		{common.DatabaseTypePostgreSQL, "TEST_POSTGRES_DSN", postgres.Open},
	} {
		t.Run(string(database.dialect), func(t *testing.T) {
			dsn := os.Getenv(database.dsnEnv)
			if database.dsnEnv != "" && dsn == "" {
				t.Skip(database.dsnEnv + " is not configured")
			}
			db, err := gorm.Open(database.open(dsn), &gorm.Config{
				NamingStrategy: schema.NamingStrategy{TablePrefix: "arca_inbox_test_"},
			})
			require.NoError(t, err)
			connection, err := db.DB()
			require.NoError(t, err)
			connection.SetMaxOpenConns(1)
			previous, previousDialect := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(database.dialect)
			t.Cleanup(func() {
				DB = previous
				common.SetMainDatabaseType(previousDialect)
				assert.NoError(t, db.Migrator().DropTable(&ArcaFeedbackNotification{}, &ArcaFeedbackEvent{}, &ArcaFeedbackIssue{}, &User{}))
				assert.NoError(t, connection.Close())
			})
			require.NoError(t, db.AutoMigrate(&User{}, &ArcaFeedbackIssue{}, &ArcaFeedbackEvent{}, &ArcaFeedbackNotification{}))

			actor := User{Username: "threadauthor", AffCode: "author"}
			recipient := User{Username: "MentionRecipient", AffCode: "recipient", Role: common.RoleAdminUser}
			other := User{Username: "anothermember", AffCode: "another"}
			disabled := User{Username: "disabledmember", AffCode: "disabled", Status: common.UserStatusDisabled}
			require.NoError(t, db.Create(&actor).Error)
			require.NoError(t, db.Create(&recipient).Error)
			require.NoError(t, db.Create(&other).Error)
			require.NoError(t, db.Create(&disabled).Error)

			ctx := context.Background()
			content := "@mentionrecipient then @MentionRecipient. (@MentionRecipient) @threadauthor @disabledmember someone@anothermember " +
				strings.Repeat("测", 250)
			issue := ArcaFeedbackIssue{
				ID: uuid.NewString(), OwnerID: &actor.Id, Title: "Model support", Description: content,
				Category: "bug", Status: "open", Visibility: ArcaFeedbackPublic, Version: 1,
				CreationKey: uuid.NewString(), CreationHash: "topic", CreatedAt: 100, UpdatedAt: 100,
			}
			created, err := CreateArcaFeedback(ctx, issue, common.RoleCommonUser)
			require.NoError(t, err)
			replay, err := CreateArcaFeedback(ctx, issue, common.RoleCommonUser)
			require.NoError(t, err)
			require.True(t, replay.Replayed)

			for _, excluded := range []User{actor, other, disabled} {
				page, err := ListArcaFeedbackNotifications(ctx, excluded.Id, false, nil, 10)
				require.NoError(t, err)
				assert.Empty(t, page.Items, excluded.Username)
				assert.Zero(t, page.UnreadCount, excluded.Username)
			}
			first, err := ListArcaFeedbackNotifications(ctx, recipient.Id, false, nil, 10)
			require.NoError(t, err)
			require.Len(t, first.Items, 1)
			assert.EqualValues(t, 1, first.UnreadCount)
			item := first.Items[0]
			assert.Equal(t, "mention", item.Kind)
			assert.Equal(t, issue.ID, item.IssueID)
			assert.Equal(t, issue.Title, item.IssueTitle)
			assert.Equal(t, created.Issue.ActivitySeq, item.IssueNumber)
			assert.Equal(t, created.Issue.ActivitySeq, item.EventSeq)
			assert.Equal(t, actor.Username, *item.ActorUsername)
			assert.Equal(t, []rune(content)[:200], []rune(item.Snippet))

			queue, err := ArcaFeedbackDeliveryQueue(ctx, 101)
			require.NoError(t, err)
			require.Len(t, queue, 1)
			assert.Equal(t, item.ID, queue[0].ID)
			lease, err := ClaimArcaFeedbackDelivery(ctx, queue[0].ID, 101)
			require.NoError(t, err)
			require.NotEmpty(t, lease)
			secondLease, err := ClaimArcaFeedbackDelivery(ctx, queue[0].ID, 101)
			require.NoError(t, err)
			assert.Empty(t, secondLease, "only one worker may send")
			require.NoError(t, FinishArcaFeedbackDelivery(ctx, queue[0], lease, 101, false))
			queue, err = ArcaFeedbackDeliveryQueue(ctx, 161)
			require.NoError(t, err)
			require.Len(t, queue, 1)
			assert.Equal(t, 1, queue[0].DeliveryAttempts)
			lease, err = ClaimArcaFeedbackDelivery(ctx, queue[0].ID, 161)
			require.NoError(t, err)
			require.NoError(t, FinishArcaFeedbackDelivery(ctx, queue[0], lease, 161, true))
			queue, err = ArcaFeedbackDeliveryQueue(ctx, 162)
			require.NoError(t, err)
			assert.Empty(t, queue)

			aborted := errors.New("test rollback")
			err = db.Transaction(func(tx *gorm.DB) error {
				require.NoError(t, InsertArcaFeedbackMentions(tx, issue.ID, 10000, actor.Id, "@MentionRecipient", 101))
				return aborted
			})
			require.ErrorIs(t, err, aborted)
			page, err := ListArcaFeedbackNotifications(ctx, recipient.Id, true, nil, 10)
			require.NoError(t, err)
			require.Len(t, page.Items, 1)

			reply := ArcaFeedbackEvent{
				IssueID: issue.ID, ActorID: &actor.Id, ActorRole: "user", Kind: "reply",
				Visibility: ArcaFeedbackPublic, Content: new("@MentionRecipient @anothermember"),
				ClientRequestID: new(uuid.NewString()), PayloadHash: "reply", CreatedAt: 102,
			}
			_, err = ApplyArcaFeedbackCommand(ctx, reply, 1, actor.Role)
			require.NoError(t, err)
			page, err = ListArcaFeedbackNotifications(ctx, recipient.Id, true, nil, 1)
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			require.NotNil(t, page.NextBefore)
			assert.EqualValues(t, 2, page.UnreadCount)
			older, err := ListArcaFeedbackNotifications(ctx, recipient.Id, true, page.NextBefore, 1)
			require.NoError(t, err)
			require.Len(t, older.Items, 1)
			assert.Equal(t, item.ID, older.Items[0].ID)
			assert.Nil(t, older.NextBefore)

			remaining, err := MarkArcaFeedbackNotificationsRead(ctx, other.Id, []int64{item.ID})
			require.NoError(t, err)
			assert.EqualValues(t, 1, remaining)
			remaining, err = MarkArcaFeedbackNotificationsRead(ctx, recipient.Id, []int64{item.ID})
			require.NoError(t, err)
			assert.EqualValues(t, 1, remaining)
			remaining, err = MarkArcaFeedbackNotificationsRead(ctx, recipient.Id, nil)
			require.NoError(t, err)
			assert.Zero(t, remaining)
			remaining, err = MarkArcaFeedbackNotificationsRead(ctx, other.Id, []int64{})
			require.NoError(t, err)
			assert.EqualValues(t, 1, remaining)

			// Fill the retention window, then verify that insertion prunes only
			// the oldest rows of this recipient and retains the newest mention.
			seed := make([]ArcaFeedbackNotification, 199)
			for i := range seed {
				seed[i] = ArcaFeedbackNotification{RecipientID: recipient.Id, IssueID: issue.ID, EventSeq: int64(20000 + i)}
			}
			require.NoError(t, db.Create(&seed).Error)
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				return InsertArcaFeedbackMentions(tx, issue.ID, 40000, actor.Id, "@MentionRecipient", 103)
			}))
			var count int64
			require.NoError(t, db.Model(&ArcaFeedbackNotification{}).Where("recipient_id = ?", recipient.Id).Count(&count).Error)
			assert.EqualValues(t, 200, count)
			page, err = ListArcaFeedbackNotifications(ctx, recipient.Id, true, nil, 1)
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			assert.EqualValues(t, 40000, page.Items[0].EventSeq)

			require.NoError(t, db.Model(&ArcaFeedbackIssue{}).Where("id = ?", issue.ID).UpdateColumn("visibility", "private").Error)
			page, err = ListArcaFeedbackNotifications(ctx, recipient.Id, false, nil, 10)
			require.NoError(t, err)
			assert.Empty(t, page.Items)
			assert.Zero(t, page.UnreadCount)
		})
	}
}
