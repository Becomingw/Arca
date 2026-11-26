package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestManagedSubscriptionSelectsOneWholePlan(t *testing.T) {
	legacy := ArcaAllocation{Monthly: new(900.0)}
	policy := ArcaEntitlementPolicy{
		Timezone:        "UTC",
		Default:         &legacy,
		Users:           map[string]ArcaAllocation{"13": legacy},
		Organizations:   map[string]ArcaOrganization{"old-team": {Name: "Existing team", Members: []int{13, 14}, ModelAccess: ArcaModelAccess{Mode: "deny_all"}}},
		UserModelAccess: map[string]ArcaModelAccess{"13": {Mode: "deny_all"}},
		Plans: map[string]ArcaSubscriptionPlan{
			"personal": {Name: "Personal", Allocation: ArcaAllocation{Weekly: new(5.0)}, ModelAccess: ArcaModelAccess{Mode: "specific", Models: []string{"model-a"}}},
			"team":     {Name: "Team", Allocation: ArcaAllocation{Daily: new(10.0)}, ModelAccess: ArcaModelAccess{Mode: "specific", Models: []string{"model-b"}}},
		},
		UserSubscriptions:  map[string]string{"13": "personal"},
		GroupSubscriptions: map[string]string{"staff": "team"},
	}
	require.NoError(t, policy.Validate())

	direct := policy.EffectiveForGroup(13, "staff")
	require.Equal(t, "personal", direct.SubscriptionID)
	require.Equal(t, "user_subscription", direct.Source)
	require.True(t, direct.Allocation.InternalFunding)
	require.Equal(t, 5.0, *direct.Allocation.Weekly)
	require.Nil(t, direct.Allocation.Daily)
	require.Nil(t, direct.Allocation.Monthly)
	require.True(t, direct.ModelAccess.Allows("model-a"))
	require.False(t, direct.ModelAccess.Allows("model-b"))

	inherited := policy.EffectiveForGroup(14, "staff")
	require.Equal(t, "team", inherited.SubscriptionID)
	require.Equal(t, "group_subscription", inherited.Source)
	require.True(t, inherited.ModelAccess.Allows("model-b"))
	now := time.Date(2026, 11, 26, 15, 30, 0, 0, time.UTC)
	windows, err := policy.Windows(inherited, "model-b", now)
	require.NoError(t, err)
	require.Len(t, windows, 1)
	require.Equal(t, "day", windows[0].Period)
	require.Equal(t, time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC).Unix(), windows[0].Start)
	require.EqualValues(t, 10_000_000, windows[0].LimitMicros)

	fallback := policy.EffectiveForGroup(15, "unassigned")
	require.Equal(t, "default", fallback.Source)
	require.Empty(t, fallback.SubscriptionID)
}

func TestManagedSubscriptionRejectsDanglingAssignmentsAndUncoveredAliases(t *testing.T) {
	policy := ArcaEntitlementPolicy{
		Timezone:      "UTC",
		ModelFamilies: map[string]ArcaModelFamily{"text": {Name: "Text", Models: []string{"model-a"}}},
		Plans: map[string]ArcaSubscriptionPlan{"limited": {
			Name: "Limited", Allocation: ArcaAllocation{Families: map[string]ArcaSubPool{"text": {Daily: new(1.0)}}},
			ModelAccess: ArcaModelAccess{Mode: "specific", Models: []string{"model-a", "model-b"}},
		}},
	}
	require.ErrorContains(t, policy.Validate(), "covering every allowed model")
	limited := policy.Plans["limited"]
	limited.ModelAccess.Models = []string{"model-a"}
	policy.Plans["limited"] = limited
	require.NoError(t, policy.Validate())
	policy.GroupSubscriptions = map[string]string{"staff": "missing"}
	require.ErrorContains(t, policy.Validate(), "invalid group subscription assignment")
}

func TestManagedSubscriptionUsesStoredNativeUserGroup(t *testing.T) {
	previousDB, previousGroupColumn := DB, commonGroupCol
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	connection, err := database.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	DB, commonGroupCol = database, "`group`"
	t.Cleanup(func() {
		DB, commonGroupCol = previousDB, previousGroupColumn
		require.NoError(t, connection.Close())
	})
	require.NoError(t, database.AutoMigrate(&User{}))
	user := User{Username: "managed-group-subject", Group: "staff"}
	require.NoError(t, database.Create(&user).Error)
	policy := ArcaEntitlementPolicy{
		Plans: map[string]ArcaSubscriptionPlan{"team": {
			Name: "Team", Allocation: ArcaAllocation{Weekly: new(1.0)},
			ModelAccess: ArcaModelAccess{Mode: "unrestricted"},
		}},
		GroupSubscriptions: map[string]string{"staff": "team"},
	}
	actual, err := policy.EffectiveForUser(user.Id)
	require.NoError(t, err)
	require.Equal(t, "team", actual.SubscriptionID)
	require.Equal(t, "group_subscription", actual.Source)
}
