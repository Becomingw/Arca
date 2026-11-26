package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestArcaPersonalEntitlementsHideUnusableModelAliases(t *testing.T) {
	visiblePool := model.ArcaSubPool{Weekly: new(20.0)}
	hiddenPool := model.ArcaSubPool{Weekly: new(30.0)}
	snapshot := arcaEntitlementSnapshot{
		ArcaEffectiveEntitlement: model.ArcaEffectiveEntitlement{
			ModelAccess: model.ArcaModelAccess{Mode: "specific", Models: []string{"visible", "hidden"}},
			Allocation: model.ArcaAllocation{
				Models:   map[string]model.ArcaSubPool{"visible": visiblePool, "hidden": hiddenPool},
				Families: map[string]model.ArcaSubPool{"mixed": visiblePool, "private": hiddenPool},
			},
		},
		ModelFamilies: map[string]model.ArcaModelFamily{
			"mixed":      {Name: "Available", Models: []string{"visible", "hidden"}},
			"private":    {Name: "Internal", Models: []string{"hidden"}},
			"unassigned": {Name: "Other group", Models: []string{"visible"}},
		},
		Windows: []model.ArcaBudgetUsage{
			{ArcaBudgetWindow: model.ArcaBudgetWindow{Scope: "total"}},
			{ArcaBudgetWindow: model.ArcaBudgetWindow{Scope: "model", Name: "hidden", Models: []string{"hidden"}}},
			{ArcaBudgetWindow: model.ArcaBudgetWindow{Scope: "family", Name: "mixed", Models: []string{"visible", "hidden"}}},
			{ArcaBudgetWindow: model.ArcaBudgetWindow{Scope: "family", Name: "private", Models: []string{"hidden"}}},
		},
	}

	snapshot.projectPersonalAliases(map[string]bool{"visible": true})
	require.Equal(t, []string{"visible"}, snapshot.ModelAccess.Models)
	require.Equal(t, map[string]model.ArcaSubPool{"visible": visiblePool}, snapshot.Allocation.Models)
	require.Equal(t, map[string]model.ArcaSubPool{"mixed": visiblePool}, snapshot.Allocation.Families)
	require.Equal(t, map[string]model.ArcaModelFamily{"mixed": {Name: "Available", Models: []string{"visible"}}}, snapshot.ModelFamilies)
	require.Len(t, snapshot.Windows, 2)
	require.Equal(t, "total", snapshot.Windows[0].Scope)
	require.Equal(t, []string{"visible"}, snapshot.Windows[1].Models)
}
