package model

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	arcaPolicyRowID           = 1
	arcaDefaultPolicyTimezone = "Asia/Shanghai"
	arcaMaxBudgetUSD          = 100_000_000
	arcaUSDScale              = 1_000_000
)

var (
	ErrArcaEntitlementConflict = errors.New("entitlement policy has changed")
	arcaEntitlementsReady      atomic.Bool
)

type ArcaEntitlementPolicyRow struct {
	ID       int    `gorm:"primaryKey;autoIncrement:false"`
	Version  int64  `gorm:"not null"`
	Document []byte `gorm:"not null"`
}

type ArcaEntitlementPolicy struct {
	Timezone        string                      `json:"timezone"`
	Default         *ArcaAllocation             `json:"default,omitempty"`
	Organizations   map[string]ArcaOrganization `json:"organizations"`
	Groups          map[string]ArcaAllocation   `json:"groups"`
	Users           map[string]ArcaAllocation   `json:"users"`
	UserModelAccess map[string]ArcaModelAccess  `json:"user_model_access"`
	ModelFamilies   map[string]ArcaModelFamily  `json:"model_families"`
}

type ArcaOrganization struct {
	Name        string          `json:"name"`
	Members     []int           `json:"members"`
	ModelAccess ArcaModelAccess `json:"model_access"`
}

type ArcaModelFamily struct {
	Name   string   `json:"name"`
	Models []string `json:"models"`
}

type ArcaModelAccess struct {
	Mode   string   `json:"mode"`
	Models []string `json:"models,omitempty"`
}

type ArcaAllocation struct {
	InternalFunding bool                   `json:"internal_funding"`
	Unlimited       bool                   `json:"unlimited"`
	FiveHour        *float64               `json:"five_hour_budget_usd,omitempty"`
	Weekly          *float64               `json:"weekly_budget_usd,omitempty"`
	Monthly         *float64               `json:"monthly_budget_usd,omitempty"`
	Models          map[string]ArcaSubPool `json:"models"`
	Families        map[string]ArcaSubPool `json:"families"`
}

type ArcaSubPool struct {
	FiveHour *float64 `json:"five_hour_budget_usd,omitempty"`
	Daily    *float64 `json:"daily_budget_usd,omitempty"`
	Weekly   *float64 `json:"weekly_budget_usd,omitempty"`
	Monthly  *float64 `json:"monthly_budget_usd,omitempty"`
}

type ArcaEffectiveEntitlement struct {
	GroupIDs    []string        `json:"group_ids"`
	Source      string          `json:"source"`
	Allocation  ArcaAllocation  `json:"allocation"`
	ModelAccess ArcaModelAccess `json:"model_access"`
}

type ArcaBudgetWindow struct {
	Scope       string   `json:"scope"`
	Name        string   `json:"name"`
	Period      string   `json:"period"`
	Start       int64    `json:"start"`
	End         int64    `json:"end,omitempty"`
	LimitMicros int64    `json:"limit_micros"`
	Models      []string `json:"models,omitempty"`
}

func EnableArcaEntitlementRuntime() func() {
	previous := arcaEntitlementsReady.Swap(true)
	return func() { arcaEntitlementsReady.Store(previous) }
}

func ArcaEntitlementRuntimeEnabled() bool {
	return arcaEntitlementsReady.Load()
}

func LoadArcaEntitlementPolicy(ctx context.Context) (ArcaEntitlementPolicy, int64, error) {
	policy := ArcaEntitlementPolicy{Timezone: arcaDefaultPolicyTimezone}
	if DB == nil {
		return policy, 0, gorm.ErrInvalidDB
	}
	var row ArcaEntitlementPolicyRow
	result := DB.WithContext(ctx).Limit(1).Find(&row, arcaPolicyRowID)
	if result.Error != nil {
		return policy, 0, result.Error
	}
	if result.RowsAffected == 0 {
		return policy, 0, nil
	}
	err := common.Unmarshal(row.Document, &policy)
	return policy, row.Version, err
}

func SaveArcaEntitlementPolicy(ctx context.Context, version int64, policy ArcaEntitlementPolicy) (int64, error) {
	if version < 0 || version >= common.MaxWalletQuota {
		return 0, ErrArcaEntitlementConflict
	}
	if err := policy.Validate(); err != nil {
		return 0, err
	}
	document, err := common.Marshal(policy)
	if err != nil {
		return 0, err
	}
	if DB == nil {
		return 0, gorm.ErrInvalidDB
	}
	db := DB.WithContext(ctx)
	next := version + 1
	var result *gorm.DB
	if version == 0 {
		result = db.Create(&ArcaEntitlementPolicyRow{
			ID: arcaPolicyRowID, Version: next, Document: document,
		})
		if result.Error != nil {
			var existing ArcaEntitlementPolicyRow
			probe := db.Select("id").Limit(1).Find(&existing, arcaPolicyRowID)
			if probe.Error == nil && probe.RowsAffected == 1 {
				return 0, ErrArcaEntitlementConflict
			}
		}
	} else {
		result = db.Model(&ArcaEntitlementPolicyRow{}).
			Where("id = ? AND version = ?", arcaPolicyRowID, version).
			Updates(map[string]any{"version": next, "document": document})
	}
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, ErrArcaEntitlementConflict
	}
	return next, nil
}

func (p *ArcaEntitlementPolicy) Validate() error {
	if p.Timezone == "" {
		p.Timezone = arcaDefaultPolicyTimezone
	}
	if p.Timezone == "Local" || len(p.Timezone) > 128 {
		return errors.New("invalid entitlement timezone")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return errors.New("invalid entitlement timezone")
	}
	if len(p.Organizations) > 128 || len(p.ModelFamilies) > 32 || len(p.Groups)+len(p.Users)+len(p.UserModelAccess) > 10_000 {
		return errors.New("too many entitlement rules")
	}
	membershipCount := 0
	for id, group := range p.Organizations {
		membershipCount += len(group.Members)
		if !validArcaPolicyID(id) || !validArcaPolicyName(group.Name) || membershipCount > 10_000 {
			return errors.New("invalid organization definition")
		}
		members := make(map[int]bool, len(group.Members))
		for _, userID := range group.Members {
			if userID <= 0 || members[userID] {
				return errors.New("invalid or duplicate organization member")
			}
			members[userID] = true
		}
		if err := group.ModelAccess.validate(); err != nil {
			return err
		}
	}
	for id, family := range p.ModelFamilies {
		if !validArcaPolicyID(id) || !validArcaPolicyName(family.Name) || !validArcaModelNames(family.Models) {
			return errors.New("invalid model family definition")
		}
	}
	if p.Default != nil {
		if err := p.Default.validate(p.ModelFamilies); err != nil {
			return err
		}
	}
	for id, allocation := range p.Groups {
		if _, present := p.Organizations[id]; !present {
			return errors.New("allocation refers to an unknown organization")
		}
		if err := allocation.validate(p.ModelFamilies); err != nil {
			return err
		}
	}
	for id, allocation := range p.Users {
		if !validArcaUserID(id) {
			return errors.New("invalid user allocation subject")
		}
		if err := allocation.validate(p.ModelFamilies); err != nil {
			return err
		}
	}
	for id, access := range p.UserModelAccess {
		if !validArcaUserID(id) {
			return errors.New("invalid user model access subject")
		}
		if err := access.validate(); err != nil {
			return err
		}
	}
	return nil
}

func validArcaPolicyID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, ch := range id {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '_' && ch != '-' {
			return false
		}
	}
	return true
}

func validArcaPolicyName(name string) bool {
	return name != "" && len(name) <= 128 && strings.TrimSpace(name) == name && !strings.ContainsRune(name, 0)
}

func validArcaUserID(id string) bool {
	value, err := strconv.Atoi(id)
	return err == nil && value > 0 && strconv.Itoa(value) == id
}

func validArcaModelNames(names []string) bool {
	if len(names) < 1 || len(names) > 256 {
		return false
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if !validArcaPolicyName(name) || name == "*" || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

func (a ArcaModelAccess) validate() error {
	switch a.Mode {
	case "", "inherit", "unrestricted", "deny_all":
		if len(a.Models) == 0 {
			return nil
		}
	case "specific":
		if validArcaModelNames(a.Models) {
			return nil
		}
	}
	return errors.New("invalid model access rule")
}

func (a ArcaAllocation) validate(families map[string]ArcaModelFamily) error {
	totals := []*float64{a.FiveHour, a.Weekly, a.Monthly}
	if a.Unlimited {
		if a.FiveHour != nil || a.Weekly != nil || a.Monthly != nil || len(a.Models)+len(a.Families) > 0 {
			return errors.New("unlimited allocation cannot contain limits")
		}
		return nil
	}
	if len(a.Models)+len(a.Families) > 15 || !validArcaBudgetLimits(totals...) {
		return errors.New("invalid budget limits")
	}
	for name, pool := range a.Models {
		if !validArcaPolicyName(name) || !validArcaBudgetLimits(pool.FiveHour, pool.Daily, pool.Weekly, pool.Monthly) {
			return errors.New("invalid per-model budget")
		}
	}
	for id, pool := range a.Families {
		if _, present := families[id]; !present || !validArcaBudgetLimits(pool.FiveHour, pool.Daily, pool.Weekly, pool.Monthly) {
			return errors.New("invalid model family budget")
		}
	}
	return nil
}

func validArcaBudgetLimits(limits ...*float64) bool {
	for _, limit := range limits {
		if limit != nil && (math.IsNaN(*limit) || math.IsInf(*limit, 0) || *limit < 0 || *limit > arcaMaxBudgetUSD || (*limit > 0 && *limit < 0.000001)) {
			return false
		}
	}
	return true
}

func (p ArcaEntitlementPolicy) Effective(userID int) ArcaEffectiveEntitlement {
	user := strconv.Itoa(userID)
	result := ArcaEffectiveEntitlement{
		GroupIDs: []string{}, Source: "none", ModelAccess: ArcaModelAccess{Mode: "unrestricted"},
	}
	groupModels := map[string]bool{}
	groupACLRestricted, groupACLUnrestricted := false, false
	for id, group := range p.Organizations {
		if !slices.Contains(group.Members, userID) {
			continue
		}
		result.GroupIDs = append(result.GroupIDs, id)
		switch group.ModelAccess.Mode {
		case "unrestricted":
			groupACLUnrestricted = true
		case "specific", "deny_all":
			groupACLRestricted = true
			for _, name := range group.ModelAccess.Models {
				groupModels[name] = true
			}
		}
	}
	slices.Sort(result.GroupIDs)
	if personal, found := p.UserModelAccess[user]; found && personal.Mode != "" && personal.Mode != "inherit" {
		result.ModelAccess = personal
	} else if groupACLRestricted && !groupACLUnrestricted {
		result.ModelAccess = ArcaModelAccess{Mode: "deny_all"}
		if len(groupModels) != 0 {
			result.ModelAccess = ArcaModelAccess{Mode: "specific", Models: slices.Sorted(maps.Keys(groupModels))}
		}
	}

	if personal, found := p.Users[user]; found {
		result.Source, result.Allocation = "user", personal
		return result
	}
	for _, id := range result.GroupIDs {
		if group, exists := p.Groups[id]; exists {
			if result.Source != "groups" {
				result.Allocation = group
				result.Source = "groups"
			} else {
				result.Allocation = mergeArcaAllocations(result.Allocation, group)
			}
		}
	}
	if result.Source == "none" && p.Default != nil {
		result.Source, result.Allocation = "default", *p.Default
	}
	return result
}

func mergeArcaAllocations(a, b ArcaAllocation) ArcaAllocation {
	if a.Unlimited || b.Unlimited {
		return ArcaAllocation{Unlimited: true, InternalFunding: a.InternalFunding || b.InternalFunding}
	}
	return ArcaAllocation{
		InternalFunding: a.InternalFunding || b.InternalFunding,
		FiveHour:        arcaMaxLimit(a.FiveHour, b.FiveHour), Weekly: arcaMaxLimit(a.Weekly, b.Weekly),
		Monthly: arcaMaxLimit(a.Monthly, b.Monthly), Models: mergeArcaSubPools(a.Models, b.Models),
		Families: mergeArcaSubPools(a.Families, b.Families),
	}
}

func mergeArcaSubPools(a, b map[string]ArcaSubPool) map[string]ArcaSubPool {
	merged := make(map[string]ArcaSubPool, len(a)+len(b))
	maps.Copy(merged, a)
	for name, right := range b {
		if left, exists := merged[name]; exists {
			merged[name] = ArcaSubPool{
				FiveHour: arcaGenerousLimit(left.FiveHour, right.FiveHour), Daily: arcaGenerousLimit(left.Daily, right.Daily),
				Weekly: arcaGenerousLimit(left.Weekly, right.Weekly), Monthly: arcaGenerousLimit(left.Monthly, right.Monthly),
			}
		} else {
			merged[name] = right
		}
	}
	return merged
}

func arcaMaxLimit(a, b *float64) *float64 {
	if a == nil || (b != nil && *a < *b) {
		return b
	}
	return a
}

func arcaGenerousLimit(a, b *float64) *float64 {
	if a == nil || b == nil {
		return nil
	}
	return arcaMaxLimit(a, b)
}

// Allows checks the public alias. Routing and token restrictions still apply.
func (a ArcaModelAccess) Allows(alias string) bool {
	switch a.Mode {
	case "deny_all":
		return false
	case "specific":
		return slices.Contains(a.Models, alias)
	default:
		return true
	}
}

// ArcaRuntimeModelAccess skips lookup before startup and for unauthenticated
// infrastructure routes. A database or policy error fails closed once enabled.
func ArcaRuntimeModelAccess(ctx context.Context, userID int) (ArcaModelAccess, error) {
	if userID <= 0 || !ArcaEntitlementRuntimeEnabled() {
		return ArcaModelAccess{Mode: "unrestricted"}, nil
	}
	policy, _, err := LoadArcaEntitlementPolicy(ctx)
	if err != nil {
		return ArcaModelAccess{}, err
	}
	return policy.Effective(userID).ModelAccess, nil
}

func (p ArcaEntitlementPolicy) Windows(e ArcaEffectiveEntitlement, alias string, now time.Time) ([]ArcaBudgetWindow, error) {
	location, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return nil, err
	}
	if e.Allocation.Unlimited {
		return nil, nil
	}
	now = now.In(location)
	y, month, day := now.Date()
	midnight := time.Date(y, month, day, 0, 0, 0, 0, location)
	week := midnight.AddDate(0, 0, -((int(midnight.Weekday()) + 6) % 7))
	monthStart := time.Date(y, month, 1, 0, 0, 0, 0, location)
	var windows []ArcaBudgetWindow
	add := func(scope, name string, models []string, pool ArcaSubPool) {
		for _, entry := range []struct {
			period     string
			start, end time.Time
			limit      *float64
		}{
			{"rolling_5h", now.Add(-5 * time.Hour), time.Time{}, pool.FiveHour},
			{"day", midnight, midnight.AddDate(0, 0, 1), pool.Daily},
			{"week", week, week.AddDate(0, 0, 7), pool.Weekly},
			{"month", monthStart, monthStart.AddDate(0, 1, 0), pool.Monthly},
		} {
			if entry.limit == nil {
				continue
			}
			end := int64(0)
			if !entry.end.IsZero() {
				end = entry.end.Unix()
			}
			windows = append(windows, ArcaBudgetWindow{
				Scope: scope, Name: name, Period: entry.period, Start: entry.start.Unix(), End: end,
				LimitMicros: decimal.NewFromFloat(*entry.limit).Mul(decimal.NewFromInt(arcaUSDScale)).Floor().IntPart(), Models: models,
			})
		}
	}
	a := e.Allocation
	add("total", "", nil, ArcaSubPool{FiveHour: a.FiveHour, Weekly: a.Weekly, Monthly: a.Monthly})
	for _, name := range slices.Sorted(maps.Keys(a.Models)) {
		switch {
		case name == "*":
			// Legacy wildcard model budgets are aggregate restrictions.
			add("total", "*", nil, a.Models[name])
		default:
			add("model", name, []string{name}, a.Models[name])
		}
	}
	for _, id := range slices.Sorted(maps.Keys(a.Families)) {
		family, exists := p.ModelFamilies[id]
		if !exists {
			return nil, errors.New("budget refers to an unknown model family")
		}
		add("family", id, family.Models, a.Families[id])
	}
	if len(windows) > 64 {
		return nil, fmt.Errorf("effective allocation exceeds 64 budget windows")
	}
	if alias != "" {
		windows = slices.DeleteFunc(windows, func(window ArcaBudgetWindow) bool {
			return len(window.Models) > 0 && !slices.Contains(window.Models, alias)
		})
	}
	return windows, nil
}
