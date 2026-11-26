package service

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"gorm.io/gorm"
)

type arcaFeedbackDeliveryHandler struct{}

func init() {
	RegisterSystemTaskHandler(arcaFeedbackDeliveryHandler{})
}

func (arcaFeedbackDeliveryHandler) Type() string { return "arca_feedback_delivery" }

func (arcaFeedbackDeliveryHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("ARCA_FEEDBACK_DELIVERY_ENABLED", true)
}

func (arcaFeedbackDeliveryHandler) Interval() time.Duration { return time.Minute }

func (arcaFeedbackDeliveryHandler) NewPayload() any { return nil }

func (arcaFeedbackDeliveryHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	var attempted, failed int
	notices, err := model.ArcaFeedbackDeliveryQueue(ctx, common.GetTimestamp())
	if err == nil {
		for _, notice := range notices {
			if err = ctx.Err(); err != nil {
				break
			}
			var lease string
			lease, err = model.ClaimArcaFeedbackDelivery(ctx, notice.ID, common.GetTimestamp())
			if err != nil {
				break
			}
			if lease == "" {
				continue
			}
			attempted++
			delivered := deliverArcaFeedbackMention(ctx, notice) == nil
			if !delivered {
				failed++
			}
			if err = model.FinishArcaFeedbackDelivery(ctx, notice, lease, common.GetTimestamp(), delivered); err != nil {
				break
			}
		}
	}
	status, message := model.SystemTaskStatusSucceeded, ""
	if err != nil {
		status, message = model.SystemTaskStatusFailed, "Feedback delivery queue unavailable"
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, map[string]int{"attempted": attempted, "failed": failed}, message); err != nil {
		common.SysLog(common.LogText("feedback delivery task could not be marked complete"))
	}
}

func deliverArcaFeedbackMention(ctx context.Context, notice model.ArcaFeedbackNotification) error {
	visible, err := model.ArcaFeedbackDeliveryStillVisible(ctx, notice.IssueID)
	if err != nil || !visible {
		return err
	}
	user, err := model.GetUserById(notice.RecipientID, false)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if user.Status != common.UserStatusEnabled || user.Role < common.RoleAdminUser {
		return nil
	}
	base, err := url.Parse(strings.TrimSpace(system_setting.ServerAddress))
	if err != nil || base.Hostname() == "" || (base.Scheme != "https" && base.Scheme != "http") ||
		base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return errors.New("invalid public address for feedback notices")
	}
	base = base.JoinPath("feedback")
	base.RawQuery = url.Values{"topic": {notice.IssueID}}.Encode()
	link := base.String()

	setting := user.GetSetting()
	title := i18n.Translate(setting.Language, i18n.MsgArcaFeedbackMentionTitle)
	content := i18n.Translate(setting.Language, i18n.MsgArcaFeedbackMentionContent)
	if user.Id == common.GetEnvOrDefault("ARCA_FEEDBACK_SERVERCHAN_USER_ID", 0) {
		if key := strings.TrimSpace(os.Getenv("ARCA_FEEDBACK_SERVERCHAN_SENDKEY")); key != "" {
			return sendArcaFeedbackServerChan(ctx, key, title, content+"\n\n"+link)
		}
	}
	if setting.NotifyType == "" || setting.NotifyType == dto.NotifyTypeEmail {
		email := user.Email
		if setting.NotificationEmail != "" {
			email = setting.NotificationEmail
		}
		if email == "" {
			return nil
		}
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email || strings.ContainsAny(email, ";\r\n") {
			return errors.New("invalid feedback notification email address")
		}
		content = "<p>" + html.EscapeString(content) + "</p><p><a href=\"" + html.EscapeString(link) + "\">" + html.EscapeString(link) + "</a></p>"
	} else {
		content += "\n\n" + link
	}
	return NotifyUser(user.Id, user.Email, setting, dto.NewNotify("arca_feedback_mention", title, content, nil))
}

func sendArcaFeedbackServerChan(ctx context.Context, key, title, content string) error {
	if len(key) > 128 || !strings.HasPrefix(key, "SCT") ||
		strings.ContainsFunc(key, func(ch rune) bool {
			return !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_')
		}) {
		return errors.New("invalid ServerChan feedback credential")
	}
	form := url.Values{"title": {title}, "desp": {content}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://sctapi.ftqq.com/"+key+".send", strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("could not build ServerChan feedback request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := *GetSSRFProtectedHTTPClient()
	client.Timeout = 15 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return errors.New("ServerChan feedback delivery failed")
	}
	defer response.Body.Close()
	var result struct {
		Code *int `json:"code"`
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices ||
		common.DecodeJsonBody(io.LimitReader(response.Body, 4096), &result) != nil || result.Code == nil || *result.Code != 0 {
		return errors.New("ServerChan rejected the feedback notice")
	}
	return nil
}
