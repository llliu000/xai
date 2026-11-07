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
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	assetSyncTaskType     = "asset_sync"
	assetSyncBatchSize    = 50
	assetSyncMaxRetries   = 8
	assetSyncPollInterval = 15 * time.Second
)

// SyncAssetsToChannel captures existing assets in one transaction. Later uploads
// do not extend this backfill; existing mappings retain their original Manual flag.
func SyncAssetsToChannel(ctx context.Context, channelID int) (int64, error) {
	var channel model.Channel
	if err := model.DB.First(&channel, channelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, common.NewMessage("Channel does not exist")
		}
		return 0, err
	}
	if channel.GetSetting().TaskPluginKey != "doubao" {
		return 0, common.NewMessage("Asset synchronization requires a DoubaoVideo channel")
	}
	var queued int64
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var assets []model.Asset
		existing := tx.Model(&model.AssetChannel{}).Select("asset_id").Where("channel_id = ?", channelID)
		if err := tx.Select("id", "user_id").Where("id NOT IN (?)", existing).
			Order("id ASC").Find(&assets).Error; err != nil {
			return err
		}
		if len(assets) == 0 {
			return nil
		}
		rows := make([]model.AssetChannel, 0, len(assets))
		for _, asset := range assets {
			rows = append(rows, model.AssetChannel{
				UserId: asset.UserId, AssetId: asset.Id, ChannelId: channelID,
				Manual: true, Status: model.AssetStatusPending,
			})
		}
		result := tx.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(&rows, assetSyncBatchSize)
		queued = result.RowsAffected
		return result.Error
	})
	if err != nil {
		return 0, err
	}
	if queued > 0 {
		_, _, err = EnqueueSystemTask(assetSyncTaskType, nil)
	}
	return queued, err
}

func init() { RegisterSystemTaskHandler(assetSyncHandler{}) }

type assetSyncHandler struct{}

func (assetSyncHandler) Type() string            { return assetSyncTaskType }
func (assetSyncHandler) Enabled() bool           { return true }
func (assetSyncHandler) Interval() time.Duration { return assetSyncPollInterval }
func (assetSyncHandler) NewPayload() any         { return nil }

func (assetSyncHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	db := model.DB.WithContext(ctx)
	err := errors.Join(
		runAssetStatusSyncBatch(db),
		runAssetUploadBatch(db),
	)
	status := model.SystemTaskStatusSucceeded
	message := ""
	if err != nil {
		status = model.SystemTaskStatusFailed
		message = err.Error()
	}
	if err = model.FinishSystemTask(task.TaskID, runnerID, status, nil, message); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

// Workers use persisted mappings, including backfills to disabled channels.
func runAssetStatusSyncBatch(db *gorm.DB) error {
	var rows []model.AssetChannel
	if err := db.Where("status = ?", model.AssetStatusProcessing).
		Where("upstream_asset_id <> ?", "").
		Where("next_poll_at <= ?", time.Now().Unix()).
		Order("next_poll_at ASC").Order("id ASC").
		Limit(assetSyncBatchSize).Find(&rows).Error; err != nil {
		return err
	}
	acm := make(map[int][]*model.AssetChannel)
	for i := range rows {
		acm[rows[i].ChannelId] = append(acm[rows[i].ChannelId], &rows[i])
	}
	for channelID, pending := range acm {
		var (
			adaptor assetrelay.Adaptor
			channel model.Channel
		)

		err := db.First(&channel, channelID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err = deleteChannelAssetMappings(db, channelID); err != nil {
				return err
			}
			continue
		}
		if err == nil {
			adaptor, err = assetrelay.GetAdaptor(&channel)
		}

		if err != nil {
			for _, row := range pending {
				if taskErr := saveAssetSyncFailure(db, row, err.Error()); taskErr != nil {
					return taskErr
				}
			}
			continue
		}

		for _, row := range pending {
			if err = syncAssetChannelStatus(db, row, adaptor); err == nil {
				continue
			}
			if err = saveAssetSyncFailure(db, row, err.Error()); err != nil {
				return err
			}
		}
	}
	return nil
}

func runAssetUploadBatch(db *gorm.DB) error {
	var rows []model.AssetChannel
	if err := db.Preload("Asset").
		Where("status = ?", model.AssetStatusPending).
		Where("upstream_asset_id = ?", "").
		Where("next_poll_at <= ?", time.Now().Unix()).
		Order("next_poll_at ASC").
		Order("id ASC").
		Limit(assetSyncBatchSize).
		Find(&rows).Error; err != nil {
		return err
	}
	acm := make(map[int][]*model.AssetChannel)
	for i := range rows {
		acm[rows[i].ChannelId] = append(acm[rows[i].ChannelId], &rows[i])
	}
	for channelID, pending := range acm {
		var (
			adaptor assetrelay.Adaptor
			channel model.Channel
		)

		err := db.First(&channel, channelID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err = deleteChannelAssetMappings(db, channelID); err != nil {
				return err
			}
			continue
		}
		if err == nil {
			adaptor, err = assetrelay.GetAdaptor(&channel)
		}
		if err != nil {
			for _, row := range pending {
				if taskErr := saveAssetSyncFailure(db, row, err.Error()); taskErr != nil {
					return taskErr
				}
			}
			continue
		}
		groups := make(map[string][]*model.AssetChannel)
		for _, row := range pending {
			groups[row.Asset.GroupId] = append(groups[row.Asset.GroupId], row)
		}
		for groupID, members := range groups {
			upstreamID, groupErr := upstreamAssetSyncGroup(db, adaptor, channelID, groupID)
			if groupErr != nil {
				for _, row := range members {
					if err = saveAssetSyncFailure(db, row, groupErr.Error()); err != nil {
						return err
					}
				}
				continue
			}
			for _, row := range members {
				if err = uploadAssetChannel(db, row, adaptor, upstreamID); err == nil {
					continue
				}
				if err = saveAssetSyncFailure(db, row, err.Error()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func deleteChannelAssetMappings(db *gorm.DB, channelID int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", channelID).Delete(&model.AssetGroupChannel{}).Error; err != nil {
			return err
		}
		var rows []model.AssetChannel
		return tx.Select("id", "asset_id", "manual").Where("channel_id = ?", channelID).
			FindInBatches(&rows, assetSyncBatchSize, func(batchTx *gorm.DB, _ int) error {
				if err := batchTx.Delete(&rows).Error; err != nil {
					return err
				}
				for _, row := range rows {
					if err := refreshAssetSyncStatus(batchTx, row.AssetId, nil); err != nil {
						return err
					}
				}
				return nil
			}).Error
	})
}

func syncAssetChannelStatus(db *gorm.DB, row *model.AssetChannel, adaptor assetrelay.Adaptor) error {
	remote, err := adaptor.GetAsset(&dto.GetAssetRequest{Id: row.UpstreamAssetId})
	if err != nil {
		return fmt.Errorf("unable to retrieve asset: %v", err)
	}
	switch remote.Status {
	case model.AssetStatusActive, model.AssetStatusProcessing:
		var next int64
		if remote.Status == model.AssetStatusProcessing {
			next = time.Now().Add(assetSyncPollInterval).Unix()
		}
		return saveAssetChannelSyncResult(db, row, map[string]any{
			"status":       remote.Status,
			"next_poll_at": next,
			"retry_count":  0,
			"error":        nil,
		})
	case model.AssetStatusFailed:
		failure := dto.AssetError{
			Message: "Asset processing or moderation failed",
			Code:    "AssetFailed",
		}
		if remote.Error != nil {
			failure = *remote.Error
		}
		return saveAssetChannelSyncResult(db, row, map[string]any{
			"error":        datatypes.NewJSONType(failure),
			"status":       model.AssetStatusFailed,
			"next_poll_at": 0,
		})
	default:
		return fmt.Errorf("unsupported status %v", remote.Status)
	}
}

func uploadAssetChannel(db *gorm.DB, row *model.AssetChannel, adaptor assetrelay.Adaptor, groupID string) error {
	request := dto.CreateAssetRequest{
		AssetType: row.Asset.AssetType,
		Name:      row.Asset.Name,
		URL:       row.Asset.URL,
		GroupId:   groupID,
	}
	if row.Asset.Moderation != nil {
		request.Moderation = new(row.Asset.Moderation.Data())
	}
	created, err := adaptor.CreateAssets(&request)
	if err != nil {
		return err
	}
	if created == nil || created.Id == "" {
		return fmt.Errorf("unable to create upstream asset")
	}
	return saveAssetChannelSyncResult(db, row, map[string]any{
		"next_poll_at":      time.Now().Add(assetSyncPollInterval).Unix(),
		"status":            model.AssetStatusProcessing,
		"upstream_asset_id": created.Id,
		"retry_count":       0,
		"error":             nil,
	})
}

func upstreamAssetSyncGroup(db *gorm.DB, adaptor assetrelay.Adaptor, channelID int, localGroupID string) (string, error) {
	var group model.AssetGroup
	if err := db.Select("id", "group_type").First(&group, "id = ?", localGroupID).Error; err != nil {
		return "", err
	}
	groupID := ""
	if group.GroupType == "LivenessFace" {
		groupID = group.Id
	}
	var mapping model.AssetGroupChannel
	err := db.Where("channel_id = ?", channelID).Where("group_id = ?", groupID).Take(&mapping).Error
	if err == nil {
		return mapping.UpstreamGroupId, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) || group.GroupType == "LivenessFace" {
		return "", err
	}
	created, err := adaptor.CreateAssetGroup(&dto.CreateAssetGroupRequest{
		Name: generateAssetScopedID("666api-assets"), GroupType: "AIGC",
	})
	if err != nil {
		return "", err
	}
	if created == nil || created.Id == "" {
		return "", errors.New("empty upstream group response")
	}
	mapping = model.AssetGroupChannel{ChannelId: channelID, UpstreamGroupId: created.Id}
	if err = db.Create(&mapping).Error; err != nil {
		return "", err
	}
	return created.Id, nil
}

func saveAssetChannelSyncResult(db *gorm.DB, row *model.AssetChannel, updates map[string]any) error {
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AssetChannel{}).Where("id = ?", row.Id).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 || row.Manual || updates["status"] == row.Status {
			return nil
		}
		return refreshAssetSyncStatus(tx, row.AssetId, updates["error"])
	})
}

func saveAssetSyncFailure(db *gorm.DB, row *model.AssetChannel, message string) error {
	retries := row.RetryCount + 1
	status := row.Status
	next := time.Now().Add(min(15*time.Second*time.Duration(1<<min(retries-1, 5)), 5*time.Minute)).Unix()
	if retries >= assetSyncMaxRetries {
		status = model.AssetStatusFailed
		next = 0
	}
	return saveAssetChannelSyncResult(db, row, map[string]any{
		"error":       datatypes.NewJSONType(dto.AssetError{Code: "AssetSyncFailed", Message: message}),
		"retry_count": retries, "status": status, "next_poll_at": next,
	})
}

// Only the initial snapshot affects availability. Backfills never revoke Active.
func refreshAssetSyncStatus(tx *gorm.DB, assetID string, acError any) error {
	var counts []struct {
		Status string
		Count  int64
	}
	if err := tx.Model(&model.AssetChannel{}).Select("status, COUNT(*) AS count").
		Where("asset_id = ?", assetID).Where("manual = ?", false).
		Group("status").Scan(&counts).Error; err != nil {
		return err
	}
	if len(counts) == 0 {
		return nil
	}
	var total, active, failed int64
	for _, count := range counts {
		total += count.Count
		switch count.Status {
		case model.AssetStatusActive:
			active += count.Count
		case model.AssetStatusFailed:
			failed += count.Count
		}
	}
	status := model.AssetStatusProcessing
	if failed > 0 {
		status = model.AssetStatusFailed
	} else if active == total {
		status = model.AssetStatusActive
	}
	updates := map[string]any{"status": status, "error": nil, "update_time": time.Now()}
	if status == model.AssetStatusFailed {
		if acError == nil {
			updates["error"] = datatypes.NewJSONType(dto.AssetError{
				Message: "Asset synchronization failed on a required channel",
				Code:    "AssetChannelSyncFailed",
			})
		} else {
			updates["error"] = acError
		}
	}
	return tx.Model(&model.Asset{}).Where("id = ?", assetID).
		Where("status <> ?", model.AssetStatusActive).Where("status <> ?", status).
		Updates(updates).Error
}
