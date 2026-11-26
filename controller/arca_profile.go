package controller

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
)

const arcaAvatarUploadLimit = 256 * 1024

func GetArcaProfile(c *gin.Context) {
	profile, err := model.ReadArcaProfile(c.Request.Context(), c.GetInt("id"))
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"avatar": profile.AvatarHandle, "leaderboard_anonymous": profile.LeaderboardAnonymous})
}

func PutArcaProfilePrivacy(c *gin.Context) {
	var input struct {
		Anonymous *bool `json:"leaderboard_anonymous"`
	}
	if err := decodeArcaFeedbackInput(c, &input, "leaderboard_anonymous"); err != nil {
		arcaFeedbackInputError(c, err)
		return
	}
	if input.Anonymous == nil {
		arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid parameters", "ARCA_PROFILE_INVALID")
		return
	}
	if err := model.StoreArcaActivityPrivacy(c.Request.Context(), c.GetInt("id"), *input.Anonymous); err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"leaderboard_anonymous": *input.Anonymous})
}

func PutArcaAvatar(c *gin.Context) {
	body := http.MaxBytesReader(c.Writer, c.Request.Body, arcaAvatarUploadLimit)
	defer body.Close()
	var input bytes.Buffer
	if _, err := input.ReadFrom(body); err != nil {
		arcaFeedbackInputError(c, err)
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(input.Bytes()))
	if err != nil || (format != "png" && format != "jpeg" && format != "webp") ||
		cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 256 || cfg.Height > 256 {
		arcaFeedbackHTTPError(c, http.StatusUnsupportedMediaType, "Invalid parameters", "ARCA_AVATAR_INVALID")
		return
	}
	picture, _, err := image.Decode(bytes.NewReader(input.Bytes()))
	if err != nil {
		arcaFeedbackHTTPError(c, http.StatusUnsupportedMediaType, "Invalid parameters", "ARCA_AVATAR_INVALID")
		return
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	handle := uuid.NewString()
	if err := model.StoreArcaAvatar(c.Request.Context(), c.GetInt("id"), &handle, encoded.Bytes()); err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"avatar": handle})
}

func DeleteArcaAvatar(c *gin.Context) {
	if err := model.StoreArcaAvatar(c.Request.Context(), c.GetInt("id"), nil, nil); err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"avatar": nil})
}

func GetArcaAvatarHandles(c *gin.Context) {
	var ids []int
	for text := range strings.SplitSeq(c.Query("ids"), ",") {
		id, err := strconv.Atoi(text)
		if err != nil || id <= 0 || len(ids) >= 100 {
			arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid parameters", "ARCA_PROFILE_INVALID")
			return
		}
		ids = append(ids, id)
	}
	handles, err := model.ReadArcaAvatarHandles(c.Request.Context(), c.GetInt("id"), ids)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, handles)
}

func GetArcaAvatar(c *gin.Context) {
	handle, ok := arcaFeedbackUUID(c.Param("handle"))
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	data, err := model.ReadArcaAvatar(c.Request.Context(), handle)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Cache-Control", "private, max-age=60")
	c.Data(http.StatusOK, "image/png", data)
}

func GetArcaHeatmap(c *gin.Context) {
	const day = 86400
	end := time.Now().UTC().Truncate(24*time.Hour).Unix() + day
	maxRange := operation_setting.GetLogSetting().MaxRangeSeconds()
	start := end - min(365*day, maxRange)
	type point struct {
		Date   string `json:"date"`
		Tokens int64  `json:"tokens"`
	}
	points := make([]point, 0)
	if common.DataExportEnabled {
		rows, err := model.GetQuotaDataByUserId(c.GetInt("id"), start, end-1)
		if err != nil {
			arcaFeedbackStorageError(c, err)
			return
		}
		days := make(map[int64]int64)
		for _, row := range rows {
			days[row.CreatedAt-row.CreatedAt%day] += int64(row.TokenUsed)
		}
		for ts := start; ts < end; ts += day {
			points = append(points, point{Date: time.Unix(ts, 0).UTC().Format(time.DateOnly), Tokens: days[ts]})
		}
	}
	common.ApiSuccess(c, gin.H{"available": common.DataExportEnabled, "timezone": "UTC", "days": points})
}

type arcaRankedMember struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Anonymous        bool   `json:"anonymous"`
	Rank             int    `json:"rank"`
	EstimatedMinutes int64  `json:"estimated_minutes"`
	ActiveDays       int    `json:"active_days"`
	LongestStreak    int    `json:"longest_streak"`
	ActiveToday      bool   `json:"active_today"`
	Requests         *int64 `json:"requests"`
	Tokens           *int64 `json:"tokens"`
}

type arcaActivityDaySummary struct {
	days, currentStreak, longestStreak int
	lastDay                            int64
}

func GetArcaTeamActivity(c *gin.Context) {
	days, ok := arcaFeedbackQueryInt(c, "days", 7, 1, 30)
	if !ok || (days != 7 && days != 30) {
		arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid parameters", "ARCA_ACTIVITY_INVALID")
		return
	}
	ctx := c.Request.Context()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	end := today.AddDate(0, 0, 1)
	startAt, endAt := end.AddDate(0, 0, -int(days)).Unix(), end.Unix()
	members, err := model.ReadArcaActivity(ctx, startAt, endAt)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	rows := make([]arcaRankedMember, 0, len(members))
	popular := make([]model.ArcaModelUsage, 0)
	bands := make([]model.ArcaActivityBand, 0)
	suppressed := len(members) < model.ArcaActivityMinimumMembers
	usageAvailable := common.DataExportEnabled
	if suppressed {
		common.ApiSuccess(c, gin.H{"members": rows, "popular_models": popular, "activity_bands": bands, "suppressed": true, "usage_available": usageAvailable, "timezone": "UTC"})
		return
	}
	userIDs := make([]int, 0, len(members))
	for _, entry := range members {
		userIDs = append(userIDs, entry.UserID)
	}
	names, err := model.ReadArcaActivityNames(ctx, userIDs)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	memberDays, err := model.ReadArcaActivityDays(ctx, userIDs, startAt, endAt)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	bands, err = model.ReadArcaActivityBands(ctx, startAt, endAt)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	daySummary := make(map[int]arcaActivityDaySummary, len(members))
	for _, day := range memberDays {
		summary := daySummary[day.UserID]
		summary.days++
		if summary.lastDay+86400 == day.DayAt {
			summary.currentStreak++
		} else {
			summary.currentStreak = 1
		}
		summary.longestStreak = max(summary.longestStreak, summary.currentStreak)
		summary.lastDay = day.DayAt
		daySummary[day.UserID] = summary
	}
	usage := make(map[int]model.ArcaMemberUsage)
	if usageAvailable {
		memberUsage, err := model.ReadArcaMemberUsage(ctx, userIDs, startAt, endAt)
		if err != nil {
			arcaFeedbackStorageError(c, err)
			return
		}
		for _, entry := range memberUsage {
			usage[entry.UserID] = entry
		}
		popular, err = model.ReadArcaPopularModels(ctx, startAt, endAt)
		if err != nil {
			arcaFeedbackStorageError(c, err)
			return
		}
	}

	rank := 0
	var previous int64
	for i, entry := range members {
		if i == 0 || previous != entry.Windows {
			rank++
			previous = entry.Windows
		}
		label := names[entry.UserID]
		anonymous := label == ""
		if anonymous {
			label = fmt.Sprintf("Member %03d", i+1)
		}
		activity := daySummary[entry.UserID]
		row := arcaRankedMember{
			ID: "member-" + strconv.Itoa(i+1), Label: label, Anonymous: anonymous, Rank: rank, EstimatedMinutes: entry.Windows * 5,
			ActiveDays: activity.days, LongestStreak: activity.longestStreak, ActiveToday: activity.lastDay == today.Unix(),
		}
		if recorded, ok := usage[entry.UserID]; ok {
			row.Requests, row.Tokens = &recorded.Requests, &recorded.Tokens
		}
		rows = append(rows, row)
	}
	common.ApiSuccess(c, gin.H{"members": rows, "popular_models": popular, "activity_bands": bands, "suppressed": false, "usage_available": usageAvailable, "timezone": "UTC"})
}
