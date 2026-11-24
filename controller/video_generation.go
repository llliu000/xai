package controller

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetVideoGenerationTask(c *gin.Context) {
	task := findVideoWorkflowTask(c, relay.VideoGenerationPath)
	if task != nil {
		writeVideoTask(c, http.StatusOK, task, task.PrivateData.Execution.RequestID, task.Properties.OriginModelName)
	}
}

func findVideoWorkflowTask(c *gin.Context, path string) *model.Task {
	id := c.Param("task_id")
	if !usageIdentifierPattern.MatchString(id) {
		writeVideoTaskError(c, http.StatusBadRequest, "invalid_request", "Invalid task ID", "not_sent")
		return nil
	}
	task, found, err := model.GetByTaskId(c.GetInt("id"), id)
	if err != nil {
		logger.LogError(c, common.LogText("failed to query video task %s: %s", id, err.Error()))
		writeVideoTaskError(c, http.StatusInternalServerError, "internal_error", "Unable to read video task", "not_sent")
		return nil
	}
	if !found || task.TaskID != id || !task.AllowsRelayToken(c.GetInt("token_id")) || !task.ResultRetrievable() || task.PrivateData.Execution == nil ||
		task.PrivateData.Execution.RequestPath != path {
		writeVideoTaskError(c, http.StatusNotFound, "resource_not_found", "Video task not found", "not_sent")
		return nil
	}
	if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		limit, _ := common.GetContextKeyType[map[string]bool](c, constant.ContextKeyTokenModelLimit)
		if !middleware.TokenModelLimitAllows(limit, task.Properties.OriginModelName) {
			writeVideoTaskError(c, http.StatusForbidden, "model_not_allowed", "Token does not allow this model", "not_sent")
			return nil
		}
	}
	return task
}

func writeVideoTask(c *gin.Context, statusCode int, task *model.Task, requestID, modelName string) {
	c.Header("Cache-Control", "private, no-store")
	status := "queued"
	var taskError any
	switch task.Status {
	case model.TaskStatusInProgress:
		status = "running"
	case model.TaskStatusSuccess:
		status = "succeeded"
	case model.TaskStatusFailure:
		status = "failed"
		taskError = videoTaskErrorDetail("upstream_error", "Video generation failed", requestID, "accepted", false)
	}
	var progress *float64
	if value := strings.TrimSuffix(strings.TrimSpace(task.Progress), "%"); value != "" {
		if number, err := strconv.ParseFloat(value, 64); err == nil && number >= 0 && number <= 100 {
			progress = new(number)
		}
	}
	createdAt := task.CreatedAt
	if createdAt == 0 {
		createdAt = task.SubmitTime
	}
	var started, completed *int64
	if task.StartTime > 0 {
		started = new(task.StartTime)
	}
	if task.FinishTime > 0 {
		completed = new(task.FinishTime)
	}
	metadata := map[string]string{}
	if snapshot := task.PrivateData.Execution; snapshot != nil && snapshot.VideoMetadata != nil {
		metadata = snapshot.VideoMetadata
	}
	billing := videoTaskBilling(c, task, requestID)
	object := "video.task"
	var results []gin.H
	var expiresAt *int64
	if task.Platform == constant.MediaKitPlatform {
		object = "upscale.task"
		results, expiresAt = upscaleTaskResults(c, task)
		if status == "succeeded" {
			progress = new(float64(100))
		} else {
			progress = nil
		}
		if status == "failed" {
			taskError = videoTaskErrorDetail("upstream_error", "Video upscale failed", requestID, "accepted", false)
		}
	} else {
		results, expiresAt = videoGenerationResults(c, task)
		if status == "succeeded" && len(results) == 0 {
			if expiresAt != nil && *expiresAt <= common.GetTimestamp() {
				writeVideoTaskError(c, http.StatusGone, "result_expired", "Video result has expired", "not_sent")
			} else {
				writeVideoTaskError(c, http.StatusServiceUnavailable, "unavailable", "Video result details are not available", "not_sent")
			}
			return
		}
	}
	c.JSON(statusCode, gin.H{
		"task_id": task.TaskID, "request_id": requestID, "object": object, "model": modelName,
		"status": status, "progress": progress, "stage": nil, "queue_position": nil, "estimated_remaining_seconds": nil,
		"created_at": createdAt, "updated_at": max(createdAt, task.UpdatedAt), "started_at": started, "completed_at": completed,
		"result_expires_at": expiresAt, "results": results, "error": taskError, "billing": billing, "metadata": metadata,
	})
}

func videoTaskBilling(c *gin.Context, task *model.Task, requestID string) gin.H {
	billing := gin.H{"status": "pending", "currency": "USD", "amount": nil, "record_id": nil}
	if requestID != "" && task.UserId > 0 {
		record, err := model.GetUsageRecord(c.Request.Context(), task.UserId, "bill_"+requestID)
		if err == nil && record.TaskID != nil && *record.TaskID == task.TaskID {
			billing["status"], billing["amount"], billing["record_id"] = record.Status, record.NetAmount, record.RecordID
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.LogWarn(c, common.LogText("failed to read the usage record for video task %s: %s", task.TaskID, err.Error()))
		}
	}
	return billing
}

func videoGenerationResults(c *gin.Context, task *model.Task) ([]gin.H, *int64) {
	results := make([]gin.H, 0)
	if task.Status != model.TaskStatusSuccess {
		return results, nil
	}
	artifacts, err := projectTaskArtifacts(task)
	if err != nil {
		logger.LogWarn(c, common.LogText("failed to project artifacts for video task %s: %s", task.TaskID, err.Error()))
		return results, nil
	}
	var facts struct {
		UpdatedAt int64 `json:"updated_at"`
	}
	if common.Unmarshal(task.Data, &facts) != nil || facts.UpdatedAt <= 0 || facts.UpdatedAt > common.GetTimestamp() {
		return results, nil
	}
	expiresAt := new(time.Unix(facts.UpdatedAt, 0).Add(24 * time.Hour).Unix())
	if *expiresAt <= common.GetTimestamp() {
		return results, expiresAt
	}
	media := task.PrivateData.Execution.VideoResult
	if media == nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		measured, _, err := service.ProbePublicMP4(ctx, task.GetResultURL())
		if err != nil {
			return results, expiresAt
		}
		media = &measured
		if err := task.SaveVideoMediaResult(ctx, media); err != nil {
			logger.LogWarn(c, common.LogText("failed to save the verified video result for task %s: %s", task.TaskID, err.Error()))
		}
	}
	for _, artifact := range artifacts {
		if artifact.Type != "video" {
			continue
		}
		link, err := service.BuildTaskArtifactContentURL(task.TaskID, artifact.Key)
		if err != nil {
			logger.LogWarn(c, common.LogText("failed to build the artifact URL for video task %s: %s", task.TaskID, err.Error()))
			continue
		}
		target, err := url.Parse(link)
		if err != nil || target.Scheme != "https" {
			continue
		}
		results = append(results, gin.H{
			"file_id": task.TaskID + "." + artifact.Key, "url": link, "media_type": "video", "mime_type": "video/mp4",
			"duration_seconds": media.Duration, "fps": media.FPS, "expires_at": expiresAt, "size_bytes": media.Size, "sha256": media.SHA256,
			"width": media.Width, "height": media.Height, "video_codec": media.Codec, "has_audio": media.HasAudio,
		})
	}
	return results, expiresAt
}

func writeVideoTaskError(c *gin.Context, status int, code, message, dispatch string) {
	c.Header("Cache-Control", "private, no-store")
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}
	if status >= 500 {
		message = "Unable to complete the video request"
	}
	c.JSON(status, gin.H{"error": videoTaskErrorDetail(code, message, c.GetString(common.RequestIdKey), dispatch, status >= 500 || status == http.StatusTooManyRequests)})
}

func videoTaskErrorDetail(code, message, requestID, dispatch string, retryable bool) gin.H {
	errorType := "invalid_request_error"
	switch code {
	case "resource_not_found":
		errorType = "not_found_error"
	case "internal_error", "upstream_error":
		errorType = "api_error"
	case "model_not_allowed":
		errorType = "permission_error"
	}
	return gin.H{"code": code, "type": errorType, "message": message, "param": nil, "request_id": requestID,
		"retryable": retryable, "dispatch_state": dispatch, "retry_after_seconds": nil}
}
