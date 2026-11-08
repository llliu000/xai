package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	assetSyncPrepareTaskType = "asset_sync_prepare"
	assetManualSyncTaskType  = "asset_manual_sync"
	assetManualSyncBatchSize = 50
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

// The cutoff excludes later uploads. Each batch commits independently, and
// conflicts preserve existing mappings and their original Manual flag.
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
	err := db.Select("id", "user_id").
		Where("create_time <= ?", payload.SnapshotAt).
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
