package controller

import (
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/mediakit"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func ListUpscaleModels(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		limit, _ := common.GetContextKeyType[map[string]bool](c, constant.ContextKeyTokenModelLimit)
		if !middleware.TokenModelLimitAllows(limit, constant.MediaKitModel) {
			c.JSON(http.StatusOK, gin.H{"object": "list", "data": []any{}})
			return
		}
	}
	status, version := "unavailable", ""
	prices := make([]gin.H, 0)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	service.GetChannelConstraints(c).AddFilter(dto.ChannelFilter{Kind: dto.FilterMediaKitUpscale})
	selected, effectiveGroup, selectErr := service.SelectChannelForRequest(c, constant.MediaKitModel, &service.RetryParam{
		Ctx: c, ModelName: constant.MediaKitModel, TokenGroup: group, RequestPath: constant.UpscaleTaskPath, Retry: new(0),
	})
	configuredPrice, priced := ratio_setting.GetModelPrice(constant.MediaKitModel, false)
	if selectErr == nil && selected != nil && strings.TrimSpace(selected.Key) != "" && priced && configuredPrice > 0 && !math.IsInf(configuredPrice, 0) &&
		billing_setting.GetBillingMode(constant.MediaKitModel) != billing_setting.BillingModeTieredExpr {
		if effectiveGroup == "" {
			effectiveGroup = group
		}
		userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		info := &relaycommon.RelayInfo{OriginModelName: constant.MediaKitModel, UsingGroup: effectiveGroup, TokenGroup: group, UserGroup: userGroup, UserId: c.GetInt("id")}
		priceData, priceErr := helper.ModelPriceHelperPerCall(c, info)
		if priceErr == nil && priceData.UsePrice {
			info.PriceData = priceData
			snapshot, unit, snapshotErr := service.RelayUsagePriceVersion(info)
			if snapshotErr == nil {
				status, version = "available", snapshot
				amount := decimal.NewFromInt(int64(priceData.Quota)).DivRound(decimal.NewFromFloat(unit), 8).StringFixed(8)
				prices = append(prices, gin.H{"metric": "task", "unit_quantity": 1, "unit_price": amount, "currency": "USD", "qualifiers": gin.H{}})
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": []gin.H{{
		"id": constant.MediaKitModel, "display_name": "Volcengine MediaKit Video Upscale", "media_types": []string{"video"},
		"input_mime_types": []string{"video/mp4"}, "output_formats": []string{"mp4"}, "video_codecs": []string{},
		"scales": []int{2}, "supports_custom_size": false, "supports_cancel": false, "supported_enhancements": []string{},
		"limits": gin.H{"max_input_bytes": service.MP4MediaByteLimit(), "max_width": 1920, "max_height": 1920,
			"max_output_pixels": 3840 * 2160, "max_duration_seconds": nil, "min_scale": 2, "max_scale": 2, "max_fps": 120},
		"prices": prices, "price_version": version, "status": status,
	}}})
}

func setUpscaleTaskMetadata(c *gin.Context, snapshot *model.TaskExecutionSnapshot) {
	snapshot.VideoMetadata, _ = common.GetContextKeyType[map[string]string](c, mediakit.MetadataContextKey)
}

func GetUpscaleTask(c *gin.Context) {
	task := findVideoWorkflowTask(c, constant.UpscaleTaskPath)
	if task != nil {
		writeVideoTask(c, http.StatusOK, task, task.PrivateData.Execution.RequestID, task.Properties.OriginModelName)
	}
}

func GetUpscaleResult(c *gin.Context) {
	task := findVideoWorkflowTask(c, constant.UpscaleTaskPath)
	if task == nil {
		return
	}
	if task.Status != model.TaskStatusSuccess {
		writeVideoTaskError(c, http.StatusConflict, "task_not_ready", "Upscale result is not available", "not_sent")
		return
	}
	results, expiresAt := upscaleTaskResults(c, task)
	if expiresAt != nil && *expiresAt <= common.GetTimestamp() {
		writeVideoTaskError(c, http.StatusGone, "result_expired", "Upscale result has expired", "not_sent")
		return
	}
	if len(results) == 0 {
		writeVideoTaskError(c, http.StatusServiceUnavailable, "unavailable", "The upscale download is unavailable", "not_sent")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"task_id": task.TaskID, "status": "succeeded", "results": results,
		"billing": videoTaskBilling(c, task, task.PrivateData.Execution.RequestID)})
}

func upscaleTaskResults(c *gin.Context, task *model.Task) ([]gin.H, *int64) {
	results := make([]gin.H, 0)
	if task.Status != model.TaskStatusSuccess {
		return results, nil
	}
	var state mediakit.State
	if err := common.Unmarshal(task.PrivateData.PluginState, &state); err != nil || state.ExpiresAt <= 0 || state.Result == nil {
		return results, nil
	}
	expiry := &state.ExpiresAt
	if *expiry <= common.GetTimestamp() {
		return results, expiry
	}
	link, err := service.BuildTaskArtifactContentURL(task.TaskID, "video")
	if err != nil {
		return results, expiry
	}
	target, err := url.Parse(link)
	if err != nil || target.Scheme != "https" {
		return results, expiry
	}
	media := state.Result
	return append(results, gin.H{
		"file_id": task.TaskID + ".video", "url": link, "media_type": "video", "mime_type": "video/mp4", "expires_at": expiry,
		"size_bytes": media.Size, "sha256": media.SHA256, "width": media.Width, "height": media.Height,
		"duration_seconds": media.Duration, "fps": media.FPS, "video_codec": media.Codec, "has_audio": media.HasAudio,
	}), expiry
}

func upscaleArtifactExpired(task *model.Task) bool {
	if task.Platform != constant.MediaKitPlatform {
		return false
	}
	var state mediakit.State
	return common.Unmarshal(task.PrivateData.PluginState, &state) != nil || state.Result == nil || state.ExpiresAt <= common.GetTimestamp()
}
