package model

import (
	"context"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const ArcaFeedbackPublic = "public"

var (
	ErrArcaFeedbackConflict  = errors.New("feedback version or idempotency conflict")
	ErrArcaFeedbackForbidden = errors.New("feedback command forbidden")
)

type ArcaFeedbackIssue struct {
	ID            string  `json:"id" gorm:"type:varchar(36);primaryKey"`
	OwnerID       *int    `json:"owner_id,string" gorm:"index:idx_arca_feedback_creation,unique,priority:1"`
	OwnerUsername *string `json:"owner_username" gorm:"-"`
	Number        int64   `json:"number" gorm:"-"`
	Title         string  `json:"title" gorm:"type:varchar(200);not null"`
	Description   string  `json:"description,omitempty" gorm:"type:text;not null"`
	Category      string  `json:"category" gorm:"type:varchar(32);not null"`
	Model         *string `json:"model" gorm:"type:varchar(200)"`
	RequestID     *string `json:"request_id" gorm:"type:varchar(128)"`
	Status        string  `json:"status" gorm:"type:varchar(16);not null"`
	Visibility    string  `json:"visibility" gorm:"type:varchar(16);not null"`
	Version       int64   `json:"version" gorm:"not null"`
	ActivitySeq   int64   `json:"activity_seq" gorm:"not null;index"`
	PinnedAt      *int64  `json:"pinned_at"`
	EditedAt      *int64  `json:"edited_at"`
	DeletedAt     *int64  `json:"deleted_at"`
	CreatedAt     int64   `json:"created_at" gorm:"not null"`
	UpdatedAt     int64   `json:"updated_at" gorm:"not null"`
	CreationKey   string  `json:"-" gorm:"type:varchar(36);not null;index:idx_arca_feedback_creation,unique,priority:2"`
	CreationHash  string  `json:"-" gorm:"type:varchar(64);not null"`
}

type ArcaFeedbackEvent struct {
	Seq             int64   `json:"seq" gorm:"primaryKey;autoIncrement;index:idx_arca_feedback_issue_event,priority:2"`
	IssueID         string  `json:"issue_id" gorm:"type:varchar(36);not null;index:idx_arca_feedback_issue_event,priority:1"`
	ActorID         *int    `json:"actor_id,string" gorm:"index:idx_arca_feedback_command,unique,priority:1"`
	ActorUsername   *string `json:"actor_username" gorm:"-"`
	ActorRole       string  `json:"actor_role" gorm:"type:varchar(32);not null"`
	Kind            string  `json:"kind" gorm:"type:varchar(16);not null"`
	Visibility      string  `json:"visibility" gorm:"type:varchar(16);not null"`
	Content         *string `json:"content" gorm:"type:text"`
	TargetSeq       *int64  `json:"target_seq"`
	EditedAt        *int64  `json:"edited_at"`
	DeletedAt       *int64  `json:"deleted_at"`
	FromStatus      *string `json:"from_status" gorm:"type:varchar(16)"`
	ToStatus        *string `json:"to_status" gorm:"type:varchar(16)"`
	CreatedAt       int64   `json:"created_at" gorm:"not null"`
	ClientRequestID *string `json:"-" gorm:"type:varchar(36);index:idx_arca_feedback_command,unique,priority:2"`
	PayloadHash     string  `json:"-" gorm:"type:varchar(64)"`
}

type ArcaFeedbackFilter struct {
	Category string
	Status   string
	Search   string
	Skip     int
	Limit    int
}

type ArcaFeedbackPage struct {
	Items []ArcaFeedbackIssue `json:"items"`
	Total int64               `json:"total"`
}

type ArcaFeedbackDetail struct {
	Issue           ArcaFeedbackIssue   `json:"issue"`
	Events          []ArcaFeedbackEvent `json:"events"`
	NextEventCursor int64               `json:"next_event_cursor"`
	HasMore         bool                `json:"has_more"`
}

type ArcaFeedbackMutation struct {
	Issue    ArcaFeedbackIssue `json:"issue"`
	Replayed bool              `json:"replayed"`
}

var arcaFeedbackLikeEscape = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")

func ListArcaFeedback(ctx context.Context, filter ArcaFeedbackFilter) (ArcaFeedbackPage, error) {
	page := ArcaFeedbackPage{Items: make([]ArcaFeedbackIssue, 0)}
	query := arcaFeedbackDB(ctx).Model(&ArcaFeedbackIssue{}).Where("visibility = ? AND deleted_at IS NULL", ArcaFeedbackPublic)
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Search != "" {
		search := "%" + arcaFeedbackLikeEscape.Replace(strings.ToLower(filter.Search)) + "%"
		query = query.Where("LOWER(title) LIKE ? ESCAPE '!'", search)
	}
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	if err := query.Omit("description").Order("CASE WHEN pinned_at IS NULL THEN 1 ELSE 0 END, pinned_at DESC, activity_seq DESC").Offset(filter.Skip).Limit(filter.Limit).Find(&page.Items).Error; err != nil {
		return page, err
	}
	return page, fillArcaFeedbackAuthors(ctx, page.Items, nil)
}

func GetArcaFeedbackDetail(ctx context.Context, id string, after int64, limit int) (ArcaFeedbackDetail, error) {
	detail := ArcaFeedbackDetail{Events: make([]ArcaFeedbackEvent, 0), NextEventCursor: after}
	if err := findArcaFeedbackIssue(arcaFeedbackDB(ctx), id, &detail.Issue); err != nil {
		return detail, err
	}
	if err := arcaFeedbackDB(ctx).Where("issue_id = ? AND visibility = ? AND seq > ? AND seq <= ?", id, ArcaFeedbackPublic, after, detail.Issue.ActivitySeq).
		Order("seq ASC").Limit(limit + 1).Find(&detail.Events).Error; err != nil {
		return detail, err
	}
	if len(detail.Events) > limit {
		detail.HasMore = true
		detail.Events = detail.Events[:limit]
	}
	if len(detail.Events) > 0 {
		detail.NextEventCursor = detail.Events[len(detail.Events)-1].Seq
	}
	issues := []ArcaFeedbackIssue{detail.Issue}
	err := fillArcaFeedbackAuthors(ctx, issues, detail.Events)
	detail.Issue = issues[0]
	return detail, err
}

// FindArcaFeedbackCreation returns a matching topic already published by the
// caller, or rejects reuse of the same client key for different input.
func FindArcaFeedbackCreation(ctx context.Context, ownerID int, key string, hash string) (ArcaFeedbackMutation, error) {
	var existing ArcaFeedbackIssue
	err := arcaFeedbackDB(ctx).Where("owner_id = ? AND creation_key = ? AND visibility = ?", ownerID, key, ArcaFeedbackPublic).First(&existing).Error
	if err != nil {
		return ArcaFeedbackMutation{}, err
	}
	if existing.CreationHash != hash {
		return ArcaFeedbackMutation{}, ErrArcaFeedbackConflict
	}
	return arcaFeedbackMutation(ctx, existing, true)
}

func CreateArcaFeedback(ctx context.Context, issue ArcaFeedbackIssue, actorRole int) (ArcaFeedbackMutation, error) {
	if issue.OwnerID == nil || *issue.OwnerID <= 0 {
		return ArcaFeedbackMutation{}, ErrArcaFeedbackForbidden
	}
	db := arcaFeedbackDB(ctx)
	if replay, err := FindArcaFeedbackCreation(ctx, *issue.OwnerID, issue.CreationKey, issue.CreationHash); !errors.Is(err, gorm.ErrRecordNotFound) {
		return replay, err
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&issue).Error; err != nil {
			return err
		}
		event := ArcaFeedbackEvent{
			IssueID: issue.ID, ActorID: issue.OwnerID, ActorRole: arcaFeedbackActorRole(actorRole), Kind: "created",
			Visibility: ArcaFeedbackPublic, ToStatus: &issue.Status, CreatedAt: issue.CreatedAt,
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if err := InsertArcaFeedbackMentions(tx, issue.ID, event.Seq, *issue.OwnerID, issue.Description, event.CreatedAt); err != nil {
			return err
		}
		issue.ActivitySeq = event.Seq
		return tx.Model(&ArcaFeedbackIssue{}).Where("id = ?", issue.ID).UpdateColumn("activity_seq", event.Seq).Error
	})
	if err != nil {
		if replay, lookupErr := FindArcaFeedbackCreation(ctx, *issue.OwnerID, issue.CreationKey, issue.CreationHash); !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return replay, lookupErr
		}
		return ArcaFeedbackMutation{}, err
	}
	return arcaFeedbackMutation(ctx, issue, false)
}

func ApplyArcaFeedbackCommand(ctx context.Context, event ArcaFeedbackEvent, expectedVersion int64, actorRole int) (ArcaFeedbackMutation, error) {
	return applyArcaFeedbackMutation(ctx, event, expectedVersion, actorRole, ArcaFeedbackEdit{})
}

type ArcaFeedbackEdit struct {
	Pinned      *bool
	Title       *string
	Description *string
	Category    *string
	Reply       *string
}

func ApplyArcaFeedbackEdit(ctx context.Context, event ArcaFeedbackEvent, expectedVersion int64, actorRole int, edit ArcaFeedbackEdit) (ArcaFeedbackMutation, error) {
	return applyArcaFeedbackMutation(ctx, event, expectedVersion, actorRole, edit)
}

func applyArcaFeedbackMutation(ctx context.Context, event ArcaFeedbackEvent, expectedVersion int64, actorRole int, edit ArcaFeedbackEdit) (ArcaFeedbackMutation, error) {
	db := arcaFeedbackDB(ctx)
	initial, target, err := loadArcaFeedbackCommandTarget(db, event, actorRole)
	if err != nil {
		return ArcaFeedbackMutation{}, err
	}
	if replay, err := findArcaFeedbackCommandReplay(ctx, db, initial, event); !errors.Is(err, gorm.ErrRecordNotFound) {
		return replay, err
	}
	if initial.DeletedAt != nil {
		return ArcaFeedbackMutation{}, gorm.ErrRecordNotFound
	}

	columns := map[string]any{"version": gorm.Expr("version + ?", 1), "updated_at": event.CreatedAt}
	replyColumns := map[string]any{}
	switch event.Kind {
	case "status":
		if event.ToStatus == nil || initial.Status == *event.ToStatus {
			return ArcaFeedbackMutation{}, ErrArcaFeedbackConflict
		}
		event.FromStatus = &initial.Status
		columns["status"] = *event.ToStatus
	case "pin":
		if edit.Pinned == nil || (initial.PinnedAt != nil) == *edit.Pinned {
			return ArcaFeedbackMutation{}, ErrArcaFeedbackConflict
		}
		if *edit.Pinned {
			columns["pinned_at"], event.Content = event.CreatedAt, new("pinned")
		} else {
			columns["pinned_at"], event.Content = nil, new("unpinned")
		}
	case "issue_edit":
		if edit.Title != nil && *edit.Title != initial.Title {
			columns["title"] = *edit.Title
		}
		if edit.Description != nil && *edit.Description != initial.Description {
			columns["description"] = *edit.Description
		}
		if edit.Category != nil && *edit.Category != initial.Category {
			columns["category"] = *edit.Category
		}
		if len(columns) == 2 {
			return ArcaFeedbackMutation{}, ErrArcaFeedbackConflict
		}
		columns["edited_at"] = event.CreatedAt
	case "issue_delete":
		columns["deleted_at"] = event.CreatedAt
	case "reply_edit":
		if target.DeletedAt != nil || edit.Reply == nil || target.Content == nil || *target.Content == *edit.Reply {
			return ArcaFeedbackMutation{}, ErrArcaFeedbackConflict
		}
		replyColumns["content"], replyColumns["edited_at"] = *edit.Reply, event.CreatedAt
	case "reply_delete":
		if target.DeletedAt != nil {
			return ArcaFeedbackMutation{}, ErrArcaFeedbackConflict
		}
		replyColumns["content"], replyColumns["deleted_at"] = nil, event.CreatedAt
	case "reply":
	default:
		return ArcaFeedbackMutation{}, ErrArcaFeedbackForbidden
	}

	event.ActorRole = arcaFeedbackActorRole(actorRole)
	var issue ArcaFeedbackIssue
	err = db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&ArcaFeedbackIssue{}).
			Where("id = ? AND version = ? AND visibility = ? AND deleted_at IS NULL", event.IssueID, expectedVersion, ArcaFeedbackPublic).
			UpdateColumns(columns)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrArcaFeedbackConflict
		}
		if len(replyColumns) > 0 {
			result = tx.Model(&ArcaFeedbackEvent{}).Where("issue_id = ? AND seq = ? AND deleted_at IS NULL", event.IssueID, target.Seq).UpdateColumns(replyColumns)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrArcaFeedbackConflict
			}
			// Remove stale notification snippets when their source text changes.
			if err := tx.Where("issue_id = ? AND event_seq = ?", event.IssueID, target.Seq).Delete(&ArcaFeedbackNotification{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if event.Content != nil && (event.Kind == "reply" || event.Kind == "status") {
			if err := InsertArcaFeedbackMentions(tx, event.IssueID, event.Seq, *event.ActorID, *event.Content, event.CreatedAt); err != nil {
				return err
			}
		}
		if err := tx.Model(&ArcaFeedbackIssue{}).Where("id = ?", event.IssueID).UpdateColumn("activity_seq", event.Seq).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", event.IssueID).First(&issue).Error
	})
	if err != nil {
		if replay, lookupErr := FindArcaFeedbackCommandReplay(ctx, event, actorRole); !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return replay, lookupErr
		}
		return ArcaFeedbackMutation{}, err
	}
	return arcaFeedbackMutation(ctx, issue, false)
}

func FindArcaFeedbackCommandReplay(ctx context.Context, event ArcaFeedbackEvent, actorRole int) (ArcaFeedbackMutation, error) {
	db := arcaFeedbackDB(ctx)
	issue, _, err := loadArcaFeedbackCommandTarget(db, event, actorRole)
	if err != nil {
		return ArcaFeedbackMutation{}, err
	}
	return findArcaFeedbackCommandReplay(ctx, db, issue, event)
}

func findArcaFeedbackCommandReplay(ctx context.Context, db *gorm.DB, issue ArcaFeedbackIssue, event ArcaFeedbackEvent) (ArcaFeedbackMutation, error) {
	var previous ArcaFeedbackEvent
	err := db.Where("actor_id = ? AND client_request_id = ? AND visibility = ?", event.ActorID, event.ClientRequestID, ArcaFeedbackPublic).First(&previous).Error
	if err != nil {
		return ArcaFeedbackMutation{}, err
	}
	if previous.IssueID != event.IssueID || previous.Kind != event.Kind || previous.PayloadHash != event.PayloadHash {
		return ArcaFeedbackMutation{}, ErrArcaFeedbackConflict
	}
	if err := db.Where("id = ? AND visibility = ?", event.IssueID, ArcaFeedbackPublic).First(&issue).Error; err != nil {
		return ArcaFeedbackMutation{}, err
	}
	return arcaFeedbackMutation(ctx, issue, true)
}

func loadArcaFeedbackCommandTarget(db *gorm.DB, event ArcaFeedbackEvent, actorRole int) (ArcaFeedbackIssue, ArcaFeedbackEvent, error) {
	var issue ArcaFeedbackIssue
	var reply ArcaFeedbackEvent
	if event.ActorID == nil || *event.ActorID <= 0 {
		return issue, reply, ErrArcaFeedbackForbidden
	}
	if err := db.Where("id = ? AND visibility = ?", event.IssueID, ArcaFeedbackPublic).First(&issue).Error; err != nil {
		return issue, reply, err
	}
	admin := actorRole >= common.RoleAdminUser
	isOwner := issue.OwnerID != nil && *issue.OwnerID == *event.ActorID
	if issue.DeletedAt != nil && (event.Kind != "issue_delete" || (!admin && !isOwner)) {
		return issue, reply, gorm.ErrRecordNotFound
	}
	switch event.Kind {
	case "status":
		if event.ToStatus == nil || (!admin && (!isOwner || *event.ToStatus == "closed")) {
			return issue, reply, ErrArcaFeedbackForbidden
		}
	case "pin":
		if !admin {
			return issue, reply, ErrArcaFeedbackForbidden
		}
	case "issue_edit", "issue_delete":
		if !admin && !isOwner {
			return issue, reply, ErrArcaFeedbackForbidden
		}
	case "reply_edit", "reply_delete":
		if event.TargetSeq == nil {
			return issue, reply, gorm.ErrRecordNotFound
		}
		if err := db.Where("issue_id = ? AND seq = ? AND kind = ? AND visibility = ?", event.IssueID, *event.TargetSeq, "reply", ArcaFeedbackPublic).First(&reply).Error; err != nil {
			return issue, reply, err
		}
		isAuthor := reply.ActorID != nil && *reply.ActorID == *event.ActorID
		if !isAuthor && (event.Kind == "reply_edit" || !admin) {
			return issue, reply, ErrArcaFeedbackForbidden
		}
	case "reply":
	default:
		return issue, reply, ErrArcaFeedbackForbidden
	}
	return issue, reply, nil
}

func findArcaFeedbackIssue(db *gorm.DB, id string, issue *ArcaFeedbackIssue) error {
	return db.Where("id = ? AND visibility = ? AND deleted_at IS NULL", id, ArcaFeedbackPublic).First(issue).Error
}

func ResolveArcaFeedbackIssueNumber(ctx context.Context, number int64) (string, error) {
	var event ArcaFeedbackEvent
	err := arcaFeedbackDB(ctx).Select("issue_id").Where("seq = ? AND kind = ? AND visibility = ?", number, "created", ArcaFeedbackPublic).First(&event).Error
	return event.IssueID, err
}

func arcaFeedbackMutation(ctx context.Context, issue ArcaFeedbackIssue, replayed bool) (ArcaFeedbackMutation, error) {
	issues := []ArcaFeedbackIssue{issue}
	err := fillArcaFeedbackAuthors(ctx, issues, nil)
	return ArcaFeedbackMutation{Issue: issues[0], Replayed: replayed}, err
}

func fillArcaFeedbackAuthors(ctx context.Context, issues []ArcaFeedbackIssue, events []ArcaFeedbackEvent) error {
	if len(issues) > 0 {
		issueIDs := make([]string, 0, len(issues))
		for _, issue := range issues {
			issueIDs = append(issueIDs, issue.ID)
		}
		var created []ArcaFeedbackEvent
		if err := arcaFeedbackDB(ctx).Select("issue_id", "seq").Where("issue_id IN ? AND kind = ?", issueIDs, "created").Find(&created).Error; err != nil {
			return err
		}
		numbers := make(map[string]int64, len(created))
		for _, event := range created {
			numbers[event.IssueID] = event.Seq
		}
		for i := range issues {
			issues[i].Number = numbers[issues[i].ID]
			if issues[i].DeletedAt != nil {
				issues[i].Description = ""
			}
		}
	}
	ids := make(map[int]struct{})
	for _, issue := range issues {
		if issue.OwnerID != nil {
			ids[*issue.OwnerID] = struct{}{}
		}
	}
	for _, event := range events {
		if event.ActorID != nil {
			ids[*event.ActorID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	userIDs := make([]int, 0, len(ids))
	for id := range ids {
		userIDs = append(userIDs, id)
	}
	var users []User
	if err := arcaFeedbackDB(ctx).Select("id", "username").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return err
	}
	names := make(map[int]*string, len(users))
	for _, user := range users {
		names[user.Id] = new(user.Username)
	}
	for i := range issues {
		issue := &issues[i]
		if issue.OwnerID == nil {
			continue
		}
		issue.OwnerUsername = names[*issue.OwnerID]
		if issue.OwnerUsername == nil {
			issue.OwnerID = nil
		}
	}
	for i := range events {
		event := &events[i]
		if event.ActorID == nil {
			continue
		}
		event.ActorUsername = names[*event.ActorID]
		if event.ActorUsername == nil {
			event.ActorID = nil
		}
	}
	return nil
}

func arcaFeedbackActorRole(role int) string {
	switch role {
	case common.RoleRootUser:
		return "root"
	case common.RoleAdminUser:
		return "admin"
	default:
		return "user"
	}
}

// Feedback SQL can contain users' discussion text and preferences. Disable SQL
// diagnostics for these operations even when the gateway is in debug mode.
func arcaFeedbackDB(ctx context.Context) *gorm.DB {
	return DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard})
}
