package model

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestArcaProfilePrivacyAndActivity(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var dialector gorm.Dialector
			switch engine {
			case "sqlite":
				dialector = sqlite.Open("file:" + uuid.NewString() + "?mode=memory&cache=shared")
			case "mysql":
				dsn := os.Getenv("TEST_ARCA_PROFILE_MYSQL_DSN")
				if dsn == "" {
					t.Skip("set TEST_ARCA_PROFILE_MYSQL_DSN to an isolated test database")
				}
				dialector = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_ARCA_PROFILE_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("set TEST_ARCA_PROFILE_POSTGRES_DSN to an isolated test database")
				}
				dialector = postgres.Open(dsn)
			}
			var err error
			originalDB := DB
			originalActivity := arcaActivityEnabled.Load()
			DB, err = gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			db, err := DB.DB()
			require.NoError(t, err)
			t.Cleanup(func() {
				arcaActivityEnabled.Store(originalActivity)
				DB = originalDB
				require.NoError(t, db.Close())
			})
			require.NoError(t, DB.AutoMigrate(&User{}, &ArcaProfile{}, &ArcaActivityWindow{}, &ArcaFeedbackIssue{}, &ArcaFeedbackEvent{}))
			require.NoError(t, DB.AutoMigrate(&ArcaProfile{}, &ArcaActivityWindow{}))
			ctx := context.Background()
			suffix := uuid.NewString()[:6]
			var users [3]User
			for i := range users {
				name := fmt.Sprintf("%s%d", suffix, i)
				users[i] = User{Username: name, AffCode: name, Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
				require.NoError(t, DB.Create(&users[i]).Error)
			}
			consumerID := users[0].Id
			handle := uuid.NewString()
			require.NoError(t, StoreArcaActivityPrivacy(ctx, consumerID, true))
			require.NoError(t, StoreArcaAvatar(ctx, consumerID, &handle, []byte("png-canary")))
			profile, err := ReadArcaProfile(ctx, consumerID)
			require.NoError(t, err)
			require.True(t, profile.LeaderboardAnonymous)
			require.Equal(t, handle, *profile.AvatarHandle)
			handles, err := ReadArcaAvatarHandles(ctx, consumerID, []int{users[0].Id, users[1].Id})
			require.NoError(t, err)
			require.Equal(t, map[int]string{consumerID: handle}, handles)
			handles, err = ReadArcaAvatarHandles(ctx, users[1].Id, []int{consumerID})
			require.NoError(t, err)
			require.Empty(t, handles)
			image, err := ReadArcaAvatar(ctx, handle)
			require.NoError(t, err)
			require.Equal(t, []byte("png-canary"), image)
			require.NoError(t, StoreArcaAvatar(ctx, consumerID, nil, nil))
			_, err = ReadArcaAvatar(ctx, handle)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			profile, err = ReadArcaProfile(ctx, consumerID)
			require.NoError(t, err)
			require.True(t, profile.LeaderboardAnonymous)

			EnableArcaActivity()
			now := common.GetTimestamp()
			// Use a separated window each test invocation while also removing
			// fixtures at the end so a retained test DB can be reused.
			bucket := now - now%300
			t.Cleanup(func() {
				for _, user := range users {
					require.NoError(t, DB.Where("user_id = ?", user.Id).Delete(&ArcaActivityWindow{}).Error)
					require.NoError(t, DB.Where("user_id = ?", user.Id).Delete(&ArcaProfile{}).Error)
					require.NoError(t, DB.Unscoped().Delete(&User{}, user.Id).Error)
				}
			})
			for _, user := range users {
				require.NoError(t, RecordArcaActivity(ctx, user.Id, bucket))
				require.NoError(t, RecordArcaActivity(ctx, user.Id, bucket+299))
			}
			require.NoError(t, RecordArcaActivity(ctx, consumerID, bucket+300))
			members, err := ReadArcaActivity(ctx, bucket, bucket+600)
			require.NoError(t, err)
			require.Len(t, members, 3)
			require.Equal(t, ArcaActivityMember{UserID: consumerID, Windows: 2}, members[0])
			names, err := ReadArcaActivityNames(ctx, []int{users[0].Id, users[1].Id, users[2].Id})
			require.NoError(t, err)
			require.Equal(t, map[int]string{users[1].Id: users[1].Username, users[2].Id: users[2].Username}, names)
		})
	}
}
