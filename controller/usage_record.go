package controller

import (
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var usageIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,191}$`)

type usageRecordQuery struct {
	UpdatedFrom       *int64 `form:"updated_from" binding:"required"`
	UpdatedTo         *int64 `form:"updated_to" binding:"required"`
	RequestID         string `form:"request_id"`
	TaskID            string `form:"task_id"`
	TokenID           *int   `form:"token_id" binding:"omitempty,min=1"`
	ExternalAccountID string `form:"external_account_id"`
	Model             string `form:"model"`
	Status            string `form:"status"`
	Cursor            string `form:"cursor" binding:"max=256"`
	Limit             *int   `form:"limit" binding:"omitempty,min=1,max=1000"`
}

type usageRecordCursor struct {
	UpdatedAt int64 `json:"updated_at"`
	Sequence  int64 `json:"sequence"`
}

func ListUsageRecords(c *gin.Context) {
	var query usageRecordQuery
	if err := c.ShouldBindQuery(&query); err != nil || query.UpdatedFrom == nil || query.UpdatedTo == nil ||
		*query.UpdatedFrom < 0 || *query.UpdatedFrom >= *query.UpdatedTo {
		writeUsageRecordError(c, http.StatusBadRequest, "invalid_request", "Invalid usage filters or time range")
		return
	}
	for _, value := range []string{query.RequestID, query.TaskID, query.ExternalAccountID, query.Model} {
		if value != "" && !usageIdentifierPattern.MatchString(value) {
			writeUsageRecordError(c, http.StatusBadRequest, "invalid_request", "Invalid usage identifier")
			return
		}
	}
	if query.Status != "" && !slices.Contains([]string{"pending", "settled", "not_charged", "refunded", "partially_refunded"}, query.Status) {
		writeUsageRecordError(c, http.StatusBadRequest, "invalid_request", "Invalid usage status")
		return
	}
	accountID := c.GetInt("id")
	filter := model.UsageRecordFilter{
		UpdatedFrom: *query.UpdatedFrom, UpdatedTo: *query.UpdatedTo, RequestID: query.RequestID,
		TaskID: query.TaskID, ExternalAccountID: query.ExternalAccountID, Model: query.Model, Status: query.Status,
	}
	if query.TokenID != nil {
		filter.TokenID = *query.TokenID
	}
	limit := 100
	if query.Limit != nil {
		limit = *query.Limit
	}
	scope, err := common.Marshal(struct {
		AccountID int
		Filter    model.UsageRecordFilter
	}{accountID, filter})
	if err != nil {
		writeUsageRecordInternalError(c, err)
		return
	}
	var cursor usageRecordCursor
	if query.Cursor != "" {
		position, signature, ok := strings.Cut(query.Cursor, ".")
		if !ok || !hmac.Equal([]byte(signature), []byte(usageCursorSignature(scope, position))) {
			writeUsageRecordError(c, http.StatusBadRequest, "invalid_request", "Invalid usage cursor")
			return
		}
		payload, err := base64.RawURLEncoding.DecodeString(position)
		if err != nil || common.Unmarshal(payload, &cursor) != nil || cursor.Sequence <= 0 ||
			cursor.UpdatedAt < filter.UpdatedFrom || cursor.UpdatedAt >= filter.UpdatedTo {
			writeUsageRecordError(c, http.StatusBadRequest, "invalid_request", "Invalid usage cursor")
			return
		}
	}
	rows, err := model.ListUsageRecords(c.Request.Context(), accountID, filter, cursor.UpdatedAt, cursor.Sequence, limit+1)
	if err != nil {
		writeUsageRecordInternalError(c, err)
		return
	}
	var nextCursor *string
	scannedUntil := filter.UpdatedTo
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		payload, err := common.Marshal(usageRecordCursor{UpdatedAt: last.UpdatedAt, Sequence: last.Sequence})
		if err != nil {
			writeUsageRecordInternalError(c, err)
			return
		}
		position := base64.RawURLEncoding.EncodeToString(payload)
		nextCursor = new(position + "." + usageCursorSignature(scope, position))
		// Later pages can still contain revisions in this same second.
		scannedUntil = last.UpdatedAt
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "next_cursor": nextCursor, "has_more": hasMore, "scanned_until": scannedUntil})
}

func usageCursorSignature(scope []byte, position string) string {
	return common.GenerateHMAC("usage-record-cursor:" + position + ":" + string(scope))
}

func GetUsageRecord(c *gin.Context) {
	recordID := c.Param("record_id")
	if !usageIdentifierPattern.MatchString(recordID) {
		writeUsageRecordError(c, http.StatusBadRequest, "invalid_request", "Invalid usage record identifier")
		return
	}
	record, err := model.GetUsageRecord(c.Request.Context(), c.GetInt("id"), recordID)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		writeUsageRecordError(c, http.StatusNotFound, "resource_not_found", "Usage record not found")
	case err != nil:
		writeUsageRecordInternalError(c, err)
	default:
		c.JSON(http.StatusOK, record)
	}
}

func writeUsageRecordInternalError(c *gin.Context, err error) {
	logger.LogError(c.Request.Context(), common.LogText("usage record query failed: %s", err.Error()))
	writeUsageRecordError(c, http.StatusInternalServerError, "internal_error", "Failed to query usage records")
}

func writeUsageRecordError(c *gin.Context, status int, code, message string) {
	errorType := "invalid_request_error"
	if status >= 500 {
		errorType = "server_error"
	}
	c.JSON(status, gin.H{"error": gin.H{
		"code": code, "type": errorType, "message": message, "param": nil,
		"request_id": c.GetString(common.RequestIdKey), "retryable": status >= 500,
		"dispatch_state": "not_sent", "retry_after_seconds": nil,
	}})
}
