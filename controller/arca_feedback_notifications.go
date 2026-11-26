package controller

import (
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const arcaFeedbackNotificationPageLimit = 100

func ListArcaFeedbackNotifications(c *gin.Context) {
	recipient := c.GetInt("id")
	if recipient <= 0 {
		arcaFeedbackHTTPError(c, http.StatusUnauthorized, "Unauthorized, insufficient privileges", "ARCA_FEEDBACK_FORBIDDEN")
		return
	}
	limit, limitOK := arcaFeedbackQueryInt(c, "limit", 20, 1, arcaFeedbackNotificationPageLimit)
	before, beforeOK := arcaFeedbackQueryInt(c, "before", 0, 1, 1<<63-1)
	if !limitOK || !beforeOK {
		arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid parameters", "ARCA_FEEDBACK_INVALID")
		return
	}
	var cursor *int64
	if before > 0 {
		cursor = &before
	}
	unread := c.Query("unread")
	page, err := model.ListArcaFeedbackNotifications(c.Request.Context(), recipient, unread == "1" || unread == "true", cursor, int(limit))
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, page)
}

func MarkArcaFeedbackNotificationsRead(c *gin.Context) {
	recipient := c.GetInt("id")
	if recipient <= 0 {
		arcaFeedbackHTTPError(c, http.StatusUnauthorized, "Unauthorized, insufficient privileges", "ARCA_FEEDBACK_FORBIDDEN")
		return
	}
	var input struct {
		All bool    `json:"all"`
		IDs []int64 `json:"ids"`
	}
	if err := decodeArcaFeedbackInput(c, &input, "all", "ids"); err != nil {
		arcaFeedbackInputError(c, err)
		return
	}
	validIDs := len(input.IDs) > 0 && len(input.IDs) <= arcaFeedbackNotificationPageLimit &&
		!slices.ContainsFunc(input.IDs, func(id int64) bool { return id <= 0 })
	if input.All && input.IDs != nil || !input.All && !validIDs {
		arcaFeedbackHTTPError(c, http.StatusBadRequest, "Invalid request", "ARCA_FEEDBACK_INVALID")
		return
	}
	unread, err := model.MarkArcaFeedbackNotificationsRead(c.Request.Context(), recipient, input.IDs)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"unread_count": unread})
}

func ListArcaFeedbackMentionable(c *gin.Context) {
	prefix := strings.TrimSpace(c.Query("q"))
	if len(prefix) > 64 || strings.ContainsRune(prefix, 0) {
		arcaFeedbackStorageError(c, errArcaFeedbackInput)
		return
	}
	names, err := model.ListArcaFeedbackMentionable(c.Request.Context(), c.GetInt("id"), prefix)
	if err != nil {
		arcaFeedbackStorageError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": names})
}
