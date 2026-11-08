package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetrelay "github.com/QuantumNous/new-api/relay/asset"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	assetSyncPrepareTaskType = "asset_sync_prepare"
	assetManualSyncTaskType  = "asset_manual_sync"
	assetManualSyncBatchSize = 200
)

type assetSyncPreparePayload struct {
	ChannelID  int       `json:"channel_id"`
	SnapshotAt time.Time `json:"snapshot_at"`
}

// SyncAssetsToChannel queues mapping preparation; uploads and moderation are
// handled separately by asset_manual_sync. An active preparation is reused per channel.
func SyncAssetsToChannel(ctx context.Context, channelID int) (*model.SystemTask, error) {
	db := model.DB.WithContext(ctx)
	var channel model.Channel
	if err := db.First(&channel, channelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.NewMessage("Channel does not exist")
		}
		return nil, err
	}
	if channel.GetSetting().TaskPluginKey != "doubao" {
		return nil, common.NewMessage("Asset synchronization requires a DoubaoVideo channel")
	}

	activeKey := fmt.Sprintf("%s:%d", assetSyncPrepareTaskType, channelID)
	var active model.SystemTask
	err := db.Where("active_key = ?", activeKey).Take(&active).Error
	if err == nil {
		return &active, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	adaptor, err := assetrelay.GetAdaptor(&channel)
	if err != nil {
		return nil, err
	}
	request := dto.ListAssetGroupsRequest{
		Filter: &dto.ListAssetGroupsFilter{
			GroupType: new("AIGC"),
		},
		PageNumber: 1, PageSize: 1,
	}
	if _, err = adaptor.ListAssetGroups(&request); err != nil {
		return nil, err
	}
	encoded, err := common.Marshal(assetSyncPreparePayload{ChannelID: channelID, SnapshotAt: time.Now()})
	if err != nil {
		return nil, err
	}
	taskID, err := model.GenerateSystemTaskID()
	if err != nil {
		return nil, err
	}
	task := &model.SystemTask{
		Status:    model.SystemTaskStatusPending,
		Type:      assetSyncPrepareTaskType,
		Payload:   string(encoded),
		ActiveKey: &activeKey,
		TaskID:    taskID,
	}
	if err = db.Create(task).Error; err != nil {
		return nil, err
	}
	notifySystemTaskRunner()
	return task, nil
}

// GetChannelAssetSyncProgress reports the channel's current manual mappings,
// not a historical batch. taskID optionally identifies a preparation returned
// by SyncAssetsToChannel; its success only means the queue was prepared.
func GetChannelAssetSyncProgress(ctx context.Context, channelID int, taskID string) (*dto.AssetSyncProgress, error) {
	db := model.DB.WithContext(ctx)
	var channel model.Channel
	if err := db.Select("id").First(&channel, channelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.NewMessage("Channel does not exist")
		}
		return nil, err
	}

	progress := &dto.AssetSyncProgress{ChannelID: channelID}
	var active model.SystemTask
	err := db.Where("active_key = ?", fmt.Sprintf("%s:%d", assetSyncPrepareTaskType, channelID)).Take(&active).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	progress.Preparing = err == nil
	preparation := &active
	if taskID != "" {
		var task model.SystemTask
		if err := db.Where("task_id = ?", taskID).Where("type = ?", assetSyncPrepareTaskType).Take(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, common.NewMessage("Invalid parameters")
			}
			return nil, err
		}
		var payload assetSyncPreparePayload
		if err := task.DecodePayload(&payload); err != nil {
			return nil, err
		}
		if payload.ChannelID != channelID {
			return nil, common.NewMessage("Invalid parameters")
		}
		preparation = &task
	}
	if preparation.TaskID != "" {
		progress.PrepareTask = &dto.AssetSyncPreparation{
			TaskID: preparation.TaskID,
			Status: string(preparation.Status),
			Error:  preparation.Error,
		}
	}

	var counts []struct {
		Status string
		Count  int64
	}
	if err := db.Model(&model.AssetChannel{}).Select("status, COUNT(*) AS count").
		Where("channel_id = ?", channelID).Where("manual = ?", true).
		Group("status").Scan(&counts).Error; err != nil {
		return nil, err
	}
	for _, count := range counts {
		progress.Total += count.Count
		switch count.Status {
		case model.AssetStatusPending:
			progress.Pending = count.Count
		case model.AssetStatusProcessing:
			progress.Processing = count.Count
		case model.AssetStatusActive:
			progress.Active = count.Count
		case model.AssetStatusFailed:
			progress.Failed = count.Count
		}
	}
	return progress, nil
}

func init() {
	RegisterSystemTaskHandler(assetSyncPrepareHandler{})
	RegisterSystemTaskHandler(assetSyncHandler{
		taskType: assetManualSyncTaskType, manual: true, batchSize: assetManualSyncBatchSize,
	})
}

type assetSyncPrepareHandler struct{}

func (assetSyncPrepareHandler) Type() string { return assetSyncPrepareTaskType }

func (assetSyncPrepareHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	var payload assetSyncPreparePayload
	if err := task.DecodePayload(&payload); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	queued, err := prepareChannelAssetMappings(model.DB.WithContext(ctx), payload)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if queued > 0 {
		if _, _, err = EnqueueSystemTask(assetManualSyncTaskType, nil); err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
	}
	result := map[string]any{"channel_id": payload.ChannelID, "queued": queued}
	if err = model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, result, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

// The cutoff excludes later uploads. Human assets and failed local assets are
// skipped. Each batch commits independently, preserving existing mappings.
func prepareChannelAssetMappings(db *gorm.DB, payload assetSyncPreparePayload) (int64, error) {
	var channel model.Channel
	if err := db.First(&channel, payload.ChannelID).Error; err != nil {
		return 0, err
	}
	if channel.GetSetting().TaskPluginKey != "doubao" {
		return 0, common.NewMessage("Asset synchronization requires a DoubaoVideo channel")
	}
	var queued int64
	var assets []model.Asset
	existing := db.Model(&model.AssetChannel{}).Select("1").
		Where("asset_channels.asset_id = assets.id").
		Where("channel_id = ?", payload.ChannelID)
	humanGroup := db.Model(&model.AssetGroup{}).Select("1").
		Where("asset_groups.id = assets.group_id").
		Where("group_type = ?", "LivenessFace")
	err := db.Select("id", "user_id").
		Where("create_time <= ?", payload.SnapshotAt).
		Where("status <> ?", model.AssetStatusFailed).
		Where("NOT EXISTS (?)", humanGroup).
		Where("NOT EXISTS (?)", existing).
		FindInBatches(&assets, assetManualSyncBatchSize, func(tx *gorm.DB, _ int) error {
			rows := make([]model.AssetChannel, 0, len(assets))
			for _, asset := range assets {
				rows = append(rows, model.AssetChannel{
					Status:    model.AssetStatusPending,
					ChannelId: payload.ChannelID,
					UserId:    asset.UserId,
					AssetId:   asset.Id,
					Manual:    true,
				})
			}
			result := tx.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows)
			queued += result.RowsAffected
			return result.Error
		}).Error
	return queued, err
}
