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

func GetArcaTeamActivity(c *gin.Context) {
	days, ok := arcaFeedbackQueryInt(c, "days", 7, 1, 30)
	if !ok || (days != 7 && days != 30) {
		arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid parameters", "ARCA_ACTIVITY_INVALID")
		return
	}
	end := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	members, err := model.ReadArcaActivity(c.Request.Context(), end.AddDate(0, 0, -int(days)).Unix(), end.Unix())
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	type member struct {
		ID               string `json:"id"`
		Label            string `json:"label"`
		Rank             int    `json:"rank"`
		EstimatedMinutes int64  `json:"estimated_minutes"`
	}
	rows := make([]member, 0, 20)
	if len(members) < 3 {
		common.ApiSuccess(c, gin.H{"members": rows, "suppressed": true, "timezone": "UTC"})
		return
	}
	members = members[:min(20, len(members))]
	userIDs := make([]int, 0, len(members))
	for _, entry := range members {
		userIDs = append(userIDs, entry.UserID)
	}
	names, err := model.ReadArcaActivityNames(c.Request.Context(), userIDs)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	rank := 0
	var previous int64
	for i, entry := range members {
		if i == 0 || previous != entry.Windows {
			rank++
			previous = entry.Windows
		}
		label := names[entry.UserID]
		if label == "" {
			label = fmt.Sprintf("Member %03d", i+1)
		}
		rows = append(rows, member{
			ID: "member-" + strconv.Itoa(i+1), Label: label, Rank: rank, EstimatedMinutes: entry.Windows * 5,
		})
	}
	common.ApiSuccess(c, gin.H{"members": rows, "suppressed": false, "timezone": "UTC"})
}
