package controller

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type arcaMCPPage struct {
	Limit int `json:"limit"`
	Skip  int `json:"skip"`
}

func (page arcaMCPPage) valid() bool {
	return page.Limit >= 1 && page.Limit <= 100 && page.Skip >= 0 && page.Skip <= 1_000_000
}

func (page arcaMCPPage) selectIDs(ids []string) []string {
	start := min(page.Skip, len(ids))
	return ids[start:min(start+page.Limit, len(ids))]
}

func validArcaMCPOperationLabel(value string, maxBytes int) bool {
	return len(value) <= maxBytes && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

func readArcaMCPOperation(ctx context.Context, name string, args arcaMCPToolArguments) (any, error) {
	switch name {
	case "arca_users_list":
		query := struct {
			arcaMCPPage
			Username string `json:"username"`
			Group    string `json:"group"`
		}{arcaMCPPage: arcaMCPPage{Limit: 20}}
		if len(args.Path) != 0 ||
			(len(args.Query) != 0 && decodeArcaMCPObject(args.Query, &query, "limit", "skip", "username", "group") != nil) ||
			!query.valid() || !validArcaMCPOperationLabel(query.Username, 128) || !validArcaMCPOperationLabel(query.Group, 128) {
			return nil, errArcaFeedbackInput
		}
		return model.ReadArcaMCPUsers(ctx, query.Group, query.Username, query.Skip, query.Limit)
	case "arca_user_get", "arca_benefits_get", "arca_provider_get":
		key := "user_id"
		if name == "arca_provider_get" {
			key = "channel_id"
		}
		id, err := strconv.Atoi(args.Path[key])
		if err != nil || id < 1 || len(args.Path) != 1 || len(args.Query) != 0 {
			return nil, errArcaFeedbackInput
		}
		if name == "arca_provider_get" {
			return model.ReadArcaMCPProvider(ctx, id)
		}
		user, err := model.ReadArcaMCPUser(ctx, id)
		if err != nil || name == "arca_user_get" {
			return user, err
		}
		return readArcaMCPBenefits(ctx, user)
	case "arca_groups_list", "arca_subscription_plans_list":
		page := arcaMCPPage{Limit: 20}
		if len(args.Path) != 0 ||
			(len(args.Query) != 0 && decodeArcaMCPObject(args.Query, &page, "limit", "skip") != nil) || !page.valid() {
			return nil, errArcaFeedbackInput
		}
		if name == "arca_groups_list" {
			groups := slices.Sorted(maps.Keys(ratio_setting.GetGroupRatioCopy()))
			groups = slices.DeleteFunc(groups, func(group string) bool { return group == "auto" || group == "*" })
			return gin.H{"groups": page.selectIDs(groups), "total": len(groups)}, nil
		}
		policy, version, err := model.LoadArcaEntitlementPolicy(ctx)
		if err != nil {
			return nil, err
		}
		type planRow struct {
			ID string `json:"id"`
			model.ArcaSubscriptionPlan
		}
		planIDs := slices.Sorted(maps.Keys(policy.Plans))
		plans := make([]planRow, 0, min(page.Limit, len(planIDs)))
		for _, id := range page.selectIDs(planIDs) {
			plans = append(plans, planRow{ID: id, ArcaSubscriptionPlan: policy.Plans[id]})
		}
		return gin.H{"plans": plans, "version": version, "total": len(planIDs)}, nil
	case "arca_model_routes":
		query := struct {
			arcaMCPPage
			Group string `json:"group"`
			Model string `json:"model"`
		}{arcaMCPPage: arcaMCPPage{Limit: 20}}
		if len(args.Path) != 0 || decodeArcaMCPObject(args.Query, &query, "group", "model", "limit", "skip") != nil ||
			query.Group == "" || query.Model == "" || !query.valid() ||
			!validArcaMCPOperationLabel(query.Group, 128) || !validArcaMCPOperationLabel(query.Model, 255) {
			return nil, errArcaFeedbackInput
		}
		routes, err := model.ReadArcaMCPModelRoutes(ctx, query.Group, query.Model, query.Skip, query.Limit)
		return gin.H{"group": query.Group, "model": query.Model, "routes": routes}, err
	default:
		return nil, errArcaFeedbackInput
	}
}

func readArcaMCPBenefits(ctx context.Context, user model.ArcaMCPUser) (any, error) {
	policy, version, err := model.LoadArcaEntitlementPolicy(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	snapshot, err := arcaEntitlementPreview(ctx, policy, user.ID, user.Group, nil, now)
	if err != nil {
		return nil, err
	}
	balances, err := model.ListArcaUsageGrants(ctx, user.ID, now)
	if err != nil {
		return nil, err
	}
	type grant struct {
		ID              int      `json:"id"`
		Family          string   `json:"family,omitempty"`
		Models          []string `json:"models,omitempty"`
		StartsAt        int64    `json:"starts_at"`
		EndsAt          int64    `json:"ends_at,omitempty"`
		AmountMicros    int64    `json:"amount_micros"`
		RemainingMicros int64    `json:"remaining_micros"`
	}
	grants := make([]grant, 0, len(balances))
	for _, balance := range balances {
		grants = append(grants, grant{
			ID: balance.ID, Family: balance.Family, Models: balance.Models, StartsAt: balance.StartsAt, EndsAt: balance.EndsAt,
			AmountMicros: balance.AmountMicros, RemainingMicros: balance.RemainingMicros,
		})
	}
	return gin.H{"group": user.Group, "version": version, "entitlements": snapshot, "grants": grants}, nil
}

type arcaMCPSubscriptionAssignment struct {
	ExpectedVersion *int64  `json:"expected_version"`
	SubjectType     string  `json:"subject_type"`
	UserID          *int    `json:"user_id,omitempty"`
	Group           string  `json:"group,omitempty"`
	PlanID          *string `json:"plan_id"`
}

func writeArcaMCPSubscriptionAssignment(c *gin.Context, actor arcaMCPIdentity, name string, args arcaMCPToolArguments) (result any, err error) {
	var input arcaMCPSubscriptionAssignment
	if len(args.Path) != 0 || len(args.Query) != 0 || len(args.Body) > 4<<10 ||
		decodeArcaMCPObject(args.Body, &input, "expected_version", "subject_type", "user_id", "group", "plan_id") != nil ||
		input.ExpectedVersion == nil || *input.ExpectedVersion < 0 || *input.ExpectedVersion >= common.MaxWalletQuota ||
		input.PlanID == nil || len(*input.PlanID) > 64 {
		return nil, errArcaFeedbackInput
	}
	switch input.SubjectType {
	case "user":
		if input.UserID == nil || *input.UserID <= 0 || input.Group != "" {
			return nil, errArcaFeedbackInput
		}
	case "group":
		if input.UserID != nil || input.Group == "" || input.Group == "*" || input.Group == "auto" ||
			!validArcaMCPOperationLabel(input.Group, 128) {
			return nil, errArcaFeedbackInput
		}
	default:
		return nil, errArcaFeedbackInput
	}

	ctx := c.Request.Context()
	binding, err := arcaFeedbackInputHash(struct {
		Operation string
		Actor     arcaMCPIdentity
		Body      arcaMCPSubscriptionAssignment
	}{name, actor, input})
	if err != nil {
		return nil, err
	}
	dryRun := args.DryRun == nil || *args.DryRun
	if !dryRun {
		if _, err := authorizeArcaMCPMutation(ctx, actor, args, binding); err != nil {
			return nil, err
		}
		defer func() {
			model.RecordAuditLog(c, model.AuditLog{
				UserId: actor.UserID, ActorRole: actor.Role, Category: model.AuditCategoryOperation,
				Action: name, TokenRef: c.GetString("access_token_ref"), Success: err == nil,
				Other: model.AuditOther{Op: &model.AuditOperation{Action: name, Params: model.AuditFields{
					"subject_type": input.SubjectType, "user_id": input.UserID, "group": input.Group,
					"plan_id": *input.PlanID, "expected_version": *input.ExpectedVersion,
				}}},
			})
		}()
	}

	policy, version, err := model.LoadArcaEntitlementPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if version != *input.ExpectedVersion {
		return nil, model.ErrArcaEntitlementConflict
	}
	planID := *input.PlanID
	if _, found := policy.Plans[planID]; planID != "" && !found {
		return nil, gorm.ErrRecordNotFound
	}

	// The compare-and-swap version covers both the chosen plan definition
	// and the direct assignment being edited.
	var target string
	assignments := &policy.GroupSubscriptions
	impact := gin.H{"subject_type": input.SubjectType, "plan_id": planID}
	var targetUser model.ArcaMCPUser
	if input.SubjectType == "user" {
		target = strconv.Itoa(*input.UserID)
		assignments = &policy.UserSubscriptions
		targetUser, err = model.ReadArcaMCPUser(ctx, *input.UserID)
		if err != nil && !(planID == "" && (*assignments)[target] != "" && errors.Is(err, gorm.ErrRecordNotFound)) {
			return nil, err
		}
		impact["user_id"] = *input.UserID
		if targetUser.ID > 0 {
			current := policy.EffectiveForGroup(targetUser.ID, targetUser.Group)
			impact["current_effective_plan_id"] = current.SubscriptionID
			impact["current_effective_source"] = current.Source
		}
	} else {
		target = input.Group
		if !ratio_setting.ContainsGroupRatio(target) && !(planID == "" && (*assignments)[target] != "") {
			return nil, gorm.ErrRecordNotFound
		}
		impact["group"] = target
		impact["applies_to"] = "group_members_without_direct_plan"
	}
	previous := (*assignments)[target]
	impact["previous_plan_id"] = previous
	impact["changed"] = previous != planID
	if planID == "" {
		delete(*assignments, target)
	} else {
		if *assignments == nil {
			*assignments = make(map[string]string)
		}
		(*assignments)[target] = planID
	}
	if targetUser.ID > 0 {
		proposed := policy.EffectiveForGroup(targetUser.ID, targetUser.Group)
		impact["effective_plan_id"] = proposed.SubscriptionID
		impact["effective_source"] = proposed.Source
	}
	if err := policy.Validate(); err != nil {
		return nil, errArcaFeedbackInput
	}

	if dryRun {
		confirmation, err := authorizeArcaMCPMutation(ctx, actor, args, binding)
		if err != nil {
			return nil, err
		}
		return gin.H{"dry_run": true, "tool": name, "confirmation": confirmation.Ticket, "expires_at": confirmation.ExpiresAt,
			"version": version, "assignment": impact}, nil
	}
	if previous != planID {
		version, err = model.SaveArcaEntitlementPolicy(ctx, version, policy)
		if err != nil {
			return nil, err
		}
	}
	return gin.H{"dry_run": false, "version": version, "assignment": impact}, nil
}
