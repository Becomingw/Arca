package model

import (
	"context"
	"slices"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	arcaFeedbackMentionLimit = 20
	arcaFeedbackSnippetLimit = 200
	arcaFeedbackInboxLimit   = 200
)

type ArcaFeedbackNotification struct {
	ID               int64   `json:"id" gorm:"primaryKey;autoIncrement;index:idx_arca_inbox_recipient,priority:2"`
	RecipientID      int     `json:"-" gorm:"not null;index:idx_arca_inbox_recipient,priority:1;uniqueIndex:idx_arca_inbox_event,priority:1"`
	IssueID          string  `json:"issue_id" gorm:"type:varchar(36);not null;index"`
	EventSeq         int64   `json:"event_seq" gorm:"not null;uniqueIndex:idx_arca_inbox_event,priority:2"`
	ActorID          *int    `json:"actor_id,string"`
	Snippet          string  `json:"snippet" gorm:"type:varchar(200);not null"`
	CreatedAt        int64   `json:"created_at" gorm:"not null"`
	ReadAt           *int64  `json:"read_at"`
	DeliveryState    *string `json:"-" gorm:"type:varchar(16);index:idx_arca_delivery_due,priority:1"`
	DeliveryAt       *int64  `json:"-" gorm:"index:idx_arca_delivery_due,priority:2"`
	DeliveryLease    *string `json:"-" gorm:"type:varchar(36)"`
	DeliveryAttempts int     `json:"-"`
}

type ArcaFeedbackNotificationItem struct {
	ArcaFeedbackNotification
	Kind          string  `json:"kind"`
	IssueTitle    string  `json:"issue_title"`
	IssueNumber   int64   `json:"issue_number"`
	ActorUsername *string `json:"actor_username"`
}

type ArcaFeedbackNotificationPage struct {
	Items       []ArcaFeedbackNotificationItem `json:"items"`
	UnreadCount int64                          `json:"unread_count"`
	NextBefore  *int64                         `json:"next_before"`
}

func arcaFeedbackMentionNames(content string) []string {
	var names []string
	seen := make(map[string]bool)
	chars := []rune(content)
	for index := 0; index < len(chars) && len(names) < arcaFeedbackMentionLimit; index++ {
		if chars[index] != '@' {
			continue
		}
		if index > 0 {
			prev := chars[index-1]
			if unicode.IsLetter(prev) || unicode.IsNumber(prev) || prev == '_' {
				continue
			}
		}
		end := index + 1
		for end < len(chars) {
			ch := chars[end]
			if !(unicode.IsLetter(ch) || unicode.IsNumber(ch) || ch == '_' || ch == '.' || ch == '-') {
				break
			}
			end++
		}
		name := strings.TrimRight(string(chars[index+1:end]), ".-")
		index = end - 1
		if name == "" || len([]rune(name)) > 64 {
			continue
		}
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	return names
}

// InsertArcaFeedbackMentions is called inside the transaction creating a public
// feedback event. A rollback of that event also rolls back its inbox rows.
func InsertArcaFeedbackMentions(tx *gorm.DB, issueID string, eventSeq int64, actorID int, content string, createdAt int64) error {
	names := arcaFeedbackMentionNames(content)
	if len(names) == 0 {
		return nil
	}

	// Lock the recipient rows in a stable order to serialize retention on the
	// two server databases. In SQLite the surrounding transaction already writes.
	var users []User
	if err := lockForUpdate(tx).Select("id, username, role").Where("username IN ? AND status = ? AND id <> ?", names, common.UserStatusEnabled, actorID).
		Order("id").Find(&users).Error; err != nil {
		return err
	}
	characters := []rune(content)
	snippet := string(characters[:min(len(characters), arcaFeedbackSnippetLimit)])
	notifications := make([]ArcaFeedbackNotification, 0, len(users))
	for _, user := range users {
		// Database collations differ. Exact username matching follows Arca
		// behavior and keeps an imprecise database match out of the inbox.
		if !slices.Contains(names, user.Username) {
			continue
		}
		notice := ArcaFeedbackNotification{
			RecipientID: user.Id, IssueID: issueID, EventSeq: eventSeq,
			ActorID: &actorID, Snippet: snippet, CreatedAt: createdAt,
		}
		if user.Role >= common.RoleAdminUser {
			notice.DeliveryState, notice.DeliveryAt = new("pending"), &notice.CreatedAt
		}
		notifications = append(notifications, notice)
	}
	if len(notifications) == 0 {
		return nil
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "recipient_id"}, {Name: "event_seq"}},
		DoNothing: true,
	}).Create(&notifications).Error; err != nil {
		return err
	}

	for _, notification := range notifications {
		var cutoff []int64
		if err := lockForUpdate(tx).Model(&ArcaFeedbackNotification{}).Where("recipient_id = ?", notification.RecipientID).
			Order("id DESC").Offset(arcaFeedbackInboxLimit-1).Limit(1).Pluck("id", &cutoff).Error; err != nil {
			return err
		}
		if len(cutoff) > 0 {
			if err := tx.Where("recipient_id = ? AND id < ?", notification.RecipientID, cutoff[0]).
				Delete(&ArcaFeedbackNotification{}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// arcaFeedbackInbox restricts every read and update to one recipient and its
// public discussions. The issue subquery also excludes unavailable topics.
func arcaFeedbackInbox(db *gorm.DB, recipient int) *gorm.DB {
	issues := db.Model(&ArcaFeedbackIssue{}).Select("id").Where("visibility = ? AND deleted_at IS NULL", ArcaFeedbackPublic)
	return db.Model(&ArcaFeedbackNotification{}).Where("recipient_id = ? AND issue_id IN (?)", recipient, issues)
}

func ListArcaFeedbackNotifications(ctx context.Context, recipient int, unreadOnly bool, before *int64, limit int) (ArcaFeedbackNotificationPage, error) {
	db := arcaFeedbackDB(ctx)
	page := ArcaFeedbackNotificationPage{Items: make([]ArcaFeedbackNotificationItem, 0)}
	if err := arcaFeedbackInbox(db, recipient).Where("read_at IS NULL").Count(&page.UnreadCount).Error; err != nil {
		return page, err
	}
	query := arcaFeedbackInbox(db, recipient)
	if unreadOnly {
		query = query.Where("read_at IS NULL")
	}
	if before != nil {
		query = query.Where("id < ?", *before)
	}
	var notifications []ArcaFeedbackNotification
	if err := query.Order("id DESC").Limit(limit + 1).Find(&notifications).Error; err != nil {
		return page, err
	}
	if len(notifications) == 0 {
		return page, nil
	}
	if len(notifications) > limit {
		notifications = notifications[:limit]
		page.NextBefore = &notifications[len(notifications)-1].ID
	}

	issueIDs := make([]string, 0, len(notifications))
	actorIDs := make([]int, 0, len(notifications))
	for _, notification := range notifications {
		issueIDs = append(issueIDs, notification.IssueID)
		if notification.ActorID != nil {
			actorIDs = append(actorIDs, *notification.ActorID)
		}
	}
	var issues []ArcaFeedbackIssue
	if err := db.Select("id, title").Where("id IN ? AND visibility = ? AND deleted_at IS NULL", issueIDs, ArcaFeedbackPublic).Find(&issues).Error; err != nil {
		return page, err
	}
	titles := make(map[string]string, len(issues))
	for _, issue := range issues {
		titles[issue.ID] = issue.Title
	}
	var openings []ArcaFeedbackEvent
	if err := db.Select("issue_id, seq").Where("issue_id IN ? AND kind = ?", issueIDs, "created").Find(&openings).Error; err != nil {
		return page, err
	}
	numbers := make(map[string]int64, len(openings))
	for _, opening := range openings {
		numbers[opening.IssueID] = opening.Seq
	}
	var actors []User
	if len(actorIDs) > 0 {
		if err := db.Select("id, username").Where("id IN ?", actorIDs).Find(&actors).Error; err != nil {
			return page, err
		}
	}
	usernames := make(map[int]string, len(actors))
	for _, actor := range actors {
		usernames[actor.Id] = actor.Username
	}
	for _, notification := range notifications {
		title, exists := titles[notification.IssueID]
		if !exists {
			continue
		}
		item := ArcaFeedbackNotificationItem{
			ArcaFeedbackNotification: notification,
			Kind:                     "mention",
			IssueTitle:               title,
			IssueNumber:              numbers[notification.IssueID],
		}
		if notification.ActorID != nil {
			if name, exists := usernames[*notification.ActorID]; exists {
				item.ActorUsername = &name
			} else {
				item.ActorID = nil
			}
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

func MarkArcaFeedbackNotificationsRead(ctx context.Context, recipient int, ids []int64) (int64, error) {
	var unread int64
	err := arcaFeedbackDB(ctx).Transaction(func(tx *gorm.DB) error {
		query := arcaFeedbackInbox(tx, recipient).Where("read_at IS NULL")
		if ids != nil {
			query = query.Where("id IN ?", ids)
		}
		if err := query.UpdateColumn("read_at", common.GetTimestamp()).Error; err != nil {
			return err
		}
		return arcaFeedbackInbox(tx, recipient).Where("read_at IS NULL").Count(&unread).Error
	})
	return unread, err
}

func ListArcaFeedbackMentionable(ctx context.Context, actorID int, prefix string) ([]string, error) {
	names := make([]string, 0)
	query := arcaFeedbackDB(ctx).Model(&User{}).Where("status = ? AND id <> ?", common.UserStatusEnabled, actorID)
	if prefix != "" {
		query = query.Where("LOWER(username) LIKE ? ESCAPE '!'", arcaFeedbackLikeEscape.Replace(strings.ToLower(prefix))+"%")
	}
	err := query.Order("username").Limit(500).Pluck("username", &names).Error
	return names, err
}
