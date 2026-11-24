package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const pendingUsageRecordKey = "pending_usage_record"
const pendingVideoUsageRecordKey = "pending_video_usage_record"
const videoGenerationUsagePath = "/v1/video/generations"

func tracksRelayBillingPath(requestPath string) bool {
	path, _, _ := strings.Cut(requestPath, "?")
	switch path {
	case "/v1/responses", "/v1/chat/completions", "/v1/messages", "/v1/images/generations", "/v1/images/edits", videoGenerationUsagePath, constant.UpscaleTaskPath:
		return true
	}
	return false
}

// BeginRelayUsageRecord journals admission before the request can incur a
// charge. If the process stops mid-request, the amounts remain unknown.
func BeginRelayUsageRecord(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if c.Request.Method != http.MethodPost || info.UserId <= 0 || info.TokenId <= 0 || info.RequestId == "" {
		return nil
	}
	var capability string
	switch c.FullPath() {
	case "/v1/responses", "/v1/chat/completions", "/v1/messages":
		capability = "text"
	case "/v1/images/generations":
		capability = "image_generation"
	case "/v1/images/edits":
		capability = "image_edit"
	default:
		return nil
	}
	return beginUsageRecord(c, info, pendingUsageRecordKey, capability, nil)
}

func BeginVideoUsageRecord(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if c.Request.Method != http.MethodPost || (c.FullPath() != videoGenerationUsagePath && c.FullPath() != constant.UpscaleTaskPath) ||
		info.UserId <= 0 || info.TokenId <= 0 || info.RequestId == "" || info.PublicTaskID == "" {
		return nil
	}
	if _, ok := c.Get(pendingVideoUsageRecordKey); ok {
		return nil
	}
	capability := "video_generation"
	if info.Action == "video_edit" {
		capability = "video_edit"
	}
	if c.FullPath() == constant.UpscaleTaskPath {
		capability = constant.TaskActionUpscaleVideo
	}
	return beginUsageRecord(c, info, pendingVideoUsageRecordKey, capability, new(info.PublicTaskID))
}

func beginUsageRecord(c *gin.Context, info *relaycommon.RelayInfo, key, capability string, taskID *string) *types.NewAPIError {
	now := common.GetTimestamp()
	record := model.UsageRecord{
		AccountID: info.UserId, TokenID: info.TokenId, RecordID: "bill_" + info.RequestId, RequestID: info.RequestId,
		TaskID: taskID, Version: 1, Model: info.OriginModelName, Capability: capability, Status: "pending", RequestStatus: "running",
		Currency: "USD", Quantities: model.JSONValue("[]"), Metadata: model.JSONValue("{}"), CreatedAt: now, UpdatedAt: now,
	}
	priceVersion, _, err := RelayUsagePriceVersion(info)
	if err == nil {
		record.PriceVersion = priceVersion
		err = model.DB.WithContext(c.Request.Context()).Create(&record).Error
	}
	if err != nil {
		logger.LogError(c, common.LogText("failed to begin usage record for request %s: %s", info.RequestId, err.Error()))
		return types.NewErrorWithStatusCode(fmt.Errorf("failed to record request for accounting"), types.ErrorCodeUpdateDataError, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	c.Set(key, record)
	return nil
}

// ConfirmVideoUsageCharge runs after submit-time settlement, when a task's
// starting quota has been charged. Polling confirms its separate outcome.
func ConfirmVideoUsageCharge(c *gin.Context, info *relaycommon.RelayInfo, task *model.Task) {
	pending, ok := common.GetContextKeyType[model.UsageRecord](c, pendingVideoUsageRecordKey)
	if !ok || task.Quota < 0 || !tracksVideoUsage(task) ||
		pending.RequestID != task.PrivateData.Execution.RequestID || pending.TaskID == nil || *pending.TaskID != task.TaskID {
		return
	}
	priceVersion, quotaPerUSD, err := RelayUsagePriceVersion(info)
	if err != nil {
		logger.LogError(c, common.LogText("failed to identify usage price for task %s: %s", task.TaskID, err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
	defer cancel()
	charge := model.TaskUsageCharge{Quota: task.Quota, QuotaPerUSD: quotaPerUSD, PriceVersion: priceVersion}
	if err := model.RecordTaskUsageCharge(ctx, task, charge); err != nil {
		logger.LogError(c, common.LogText("failed to record the initial charge for task %s: %s", task.TaskID, err.Error()))
		return
	}
	// For an immediate result, the submit adaptor has already supplied the
	// final quota and there will be no polling transition.
	recordVideoUsageOutcome(ctx, task)
}

func tracksVideoUsage(task *model.Task) bool {
	execution := task.PrivateData.Execution
	return execution != nil && execution.RequestID != "" &&
		(execution.RequestPath == videoGenerationUsagePath || execution.RequestPath == constant.UpscaleTaskPath)
}

func recordVideoUsageOutcome(ctx context.Context, task *model.Task) {
	if task.Quota < 0 || !tracksVideoUsage(task) {
		return
	}
	var requestStatus string
	switch task.Status {
	case model.TaskStatusSuccess:
		requestStatus = "succeeded"
	case model.TaskStatusFailure:
		requestStatus = "failed"
	default:
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	outcome := model.TaskUsageOutcome{Quota: task.Quota, RequestStatus: requestStatus}
	if err := model.RecordTaskUsageOutcome(ctx, task, outcome); err != nil {
		logger.LogError(ctx, common.LogText("failed to record the final charge for task %s: %s", task.TaskID, err.Error()))
	}
}

func recordRelayUsageSettlement(c *gin.Context, info *relaycommon.RelayInfo, actualQuota int) {
	pending, ok := common.GetContextKeyType[model.UsageRecord](c, pendingUsageRecordKey)
	if !ok || pending.RequestID != info.RequestId || actualQuota < 0 {
		return
	}
	priceVersion, quotaPerUSD, err := RelayUsagePriceVersion(info)
	if err != nil {
		logger.LogError(c, common.LogText("failed to identify usage price for request %s: %s", info.RequestId, err.Error()))
		return
	}
	amount := decimal.NewFromInt(int64(actualQuota)).DivRound(decimal.NewFromFloat(quotaPerUSD), 8).StringFixed(8)
	status := "settled"
	if actualQuota == 0 {
		status = "not_charged"
	}
	requestStatus := "succeeded"
	if stream := info.StreamStatus; stream != nil {
		switch stream.ResponseOutcome() {
		case string(relaycommon.ResponseOutcomeFailed), string(relaycommon.ResponseOutcomeIncomplete):
			requestStatus = "failed"
		case string(relaycommon.ResponseOutcomeCancelled):
			requestStatus = "cancelled"
		default:
			if !stream.IsNormalEnd() || stream.HasErrors() {
				requestStatus = "unknown"
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
	defer cancel()
	quantities, _ := common.GetContextKeyType[[]model.UsageQuantity](c, "usage_record_quantities")
	if len(quantities) == 0 {
		line := model.UsageQuantity{Metric: "task", Quantity: "1", UnitQuantity: 1, UnitPrice: amount, Amount: amount}
		if info.PriceData.UsePrice && (pending.Capability == "image_generation" || pending.Capability == "image_edit") && info.BillingImageCount != nil && *info.BillingImageCount > 0 {
			count := decimal.NewFromInt(int64(*info.BillingImageCount))
			line.Metric, line.Quantity = "image", count.String()
			line.UnitPrice = decimal.RequireFromString(amount).DivRound(count, 8).StringFixed(8)
		}
		quantities = []model.UsageQuantity{line}
	}
	if err := model.RecordInitialUsageSettlement(ctx, pending, status, requestStatus, amount, priceVersion, common.GetTimestamp(), quantities); err != nil {
		logger.LogError(c, common.LogText("failed to record settlement for request %s: %s", info.RequestId, err.Error()))
	}
}

func RelayUsagePriceVersion(info *relaycommon.RelayInfo) (string, float64, error) {
	price := info.PriceData
	price.Quota, price.QuotaToPreConsume = 0, 0
	quotaPerUSD := common.QuotaPerUnit
	var expression string
	if snapshot := info.TieredBillingSnapshot; snapshot != nil {
		quotaPerUSD = snapshot.QuotaPerUnit
		expression = snapshot.ExprString
	}
	if quotaPerUSD <= 0 || math.IsNaN(quotaPerUSD) || math.IsInf(quotaPerUSD, 0) {
		return "", 0, fmt.Errorf("invalid quota unit for USD settlement")
	}
	snapshot, err := common.Marshal(struct {
		Model       string
		Price       hosttypes.PriceData
		Multipliers map[string]float64
		Expression  string
		QuotaPerUSD float64
	}{info.GetBillingModelName(), price, price.OtherRatios(), expression, quotaPerUSD})
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("xai-%x", sha256.Sum256(snapshot)), quotaPerUSD, nil
}
