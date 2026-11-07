package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

func assetSyncTargets() ([]*model.Channel, error) {
	var channels []*model.Channel
	err := model.DB.Where("type = ?", constant.ChannelTypeTaskPlugin).
		Order("id ASC").Find(&channels).Error
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, errors.New("no channels found")
	}
	var targets []*model.Channel
	for _, channel := range channels {
		if channel.GetSetting().TaskPluginKey == "doubao" {
			targets = append(targets, channel)
		}
	}
	if len(targets) == 0 {
		return nil, errors.New("no targets found")
	}
	return targets, nil
}

// Bind every required channel atomically with the local asset. No upstream I/O
// runs in this transaction; committed Pending rows are the durable work queue.
func createAssetWithChannels(asset *model.Asset) error {
	targets, err := assetSyncTargets()
	if err != nil {
		return err
	}
	channels := make([]model.AssetChannel, 0, len(targets))
	for _, target := range targets {
		channels = append(channels, model.AssetChannel{
			Status:    model.AssetStatusPending,
			ChannelId: target.Id,
			AssetId:   asset.Id,
		})
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err = tx.Create(asset).Error; err != nil {
			return err
		}
		return tx.Create(&channels).Error
	})
	if err != nil {
		return err
	}
	_, _, _ = EnqueueSystemTask(assetSyncTaskType, nil)
	return nil
}

// SyncAssetsToChannel queues existing assets that have no mapping to this channel.
// Uploading and moderation polling remain the responsibility of the background worker.
func SyncAssetsToChannel(ctx context.Context, channelID int) (int64, error) {
	db := model.DB.WithContext(ctx)
	var channel model.Channel
	if err := db.First(&channel, channelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, common.NewMessage("Channel does not exist")
		}
		return 0, err
	}
	if channel.GetSetting().TaskPluginKey != "doubao" {
		return 0, common.NewMessage("Asset synchronization requires a DoubaoVideo channel")
	}
	var queued int64
	for {
		var ids []string
		existing := db.Model(&model.AssetChannel{}).Select("asset_id").Where("channel_id = ?", channelID)
		if err := db.Model(&model.Asset{}).Where("id NOT IN (?)", existing).Order("id ASC").
			Limit(100).Pluck("id", &ids).Error; err != nil {
			return 0, err
		}
		if len(ids) == 0 {
			break
		}
		rows := make([]model.AssetChannel, 0, len(ids))
		for _, id := range ids {
			rows = append(rows, model.AssetChannel{AssetId: id, ChannelId: channelID, Status: model.AssetStatusPending})
		}
		result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows)
		if result.Error != nil {
			return 0, result.Error
		}
		queued += result.RowsAffected
	}
	if queued > 0 {
		if _, _, err := EnqueueSystemTask(assetSyncTaskType, nil); err != nil {
			return 0, err
		}
	}
	return queued, nil
}

type assetSyncTransport struct{ ctx context.Context }

func (t assetSyncTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(req.Clone(t.ctx))
}

func init() { RegisterSystemTaskHandler(assetSyncHandler{}) }

type assetSyncHandler struct{}

func (assetSyncHandler) Type() string { return assetSyncTaskType }
func (assetSyncHandler) Enabled() bool {
	return true
}
func (assetSyncHandler) Interval() time.Duration { return assetSyncPollInterval }
func (assetSyncHandler) NewPayload() any         { return nil }

func (assetSyncHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	targets, err := assetSyncTargets()
	if err == nil {
		err = runAssetSyncBatch(ctx, targets)
	}
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func runAssetSyncBatch(ctx context.Context, targets []*model.Channel) error {
	ids := make([]int, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.Id)
	}
	var rows []model.AssetChannel
	if err := model.DB.Where("channel_id IN ?", ids).Where("next_poll_at <= ?", time.Now().Unix()).
		Where("status IN ?", []string{model.AssetStatusPending, model.AssetStatusProcessing}).
		Where("asset_id IN (?)", model.DB.Model(&model.Asset{}).Select("id")).
		Order("next_poll_at ASC").Order("id ASC").Limit(assetSyncBatchSize).
		Find(&rows).Error; err != nil {
		return err
	}
	byChannel := make(map[int][]*model.AssetChannel, len(targets))
	for i := range rows {
		byChannel[rows[i].ChannelId] = append(byChannel[rows[i].ChannelId], &rows[i])
	}
	for _, channel := range targets {
		pending := byChannel[channel.Id]
		if len(pending) == 0 {
			continue
		}
		adaptor, err := assetrelay.GetAdaptor(channel)
		if err != nil {
			return err
		}
		groupID, err := upstreamAssetSyncGroup(adaptor, channel.Id)
		if err != nil {
			return err
		}
		for _, row := range pending {
			if err = syncAssetChannel(ctx, row, adaptor, groupID); err != nil {
				return err
			}
		}
	}
	return refreshAssetSyncStatus(targets)
}

// Recompute initial availability without revoking assets that are already Active.
func refreshAssetSyncStatus(targets []*model.Channel) error {
	ids := make([]int, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.Id)
	}
	if len(ids) == 0 {
		return errors.New("asset sync requires at least one channel")
	}
	failed := model.DB.Model(&model.AssetChannel{}).Select("asset_id").
		Where("channel_id IN ?", ids).Where("status = ?", model.AssetStatusFailed)

	active := model.DB.Model(&model.AssetChannel{}).Select("asset_id").Where("channel_id IN ?", ids).
		Where("status = ?", model.AssetStatusActive).Where("upstream_asset_id <> ?", "").
		Group("asset_id").Having("COUNT(*) = ?", len(ids))

	return model.DB.Transaction(func(tx *gorm.DB) error {
		failure := datatypes.JSON(`{"Code":"AssetChannelSyncFailed","Message":"Asset synchronization failed on a required channel"}`)
		err := tx.Model(&model.Asset{}).Where("id IN (?)", failed).
			Where("status <> ?", model.AssetStatusFailed).
			Where("status <> ?", model.AssetStatusActive).
			Updates(map[string]any{
				"status":      model.AssetStatusFailed,
				"update_time": time.Now(),
				"error":       failure,
			}).Error
		if err != nil {
			return err
		}
		err = tx.Model(&model.Asset{}).Where("id IN (?)", active).
			Where("status <> ?", model.AssetStatusActive).
			Where("id NOT IN (?)", failed).
			Updates(map[string]any{
				"status":      model.AssetStatusActive,
				"error":       nil,
				"update_time": time.Now(),
			}).Error
		if err != nil {
			return err
		}

		return tx.Model(&model.Asset{}).Where("id NOT IN (?)", active).
			Where("status <> ?", model.AssetStatusProcessing).
			Where("status <> ?", model.AssetStatusActive).
			Where("id NOT IN (?)", failed).
			Updates(map[string]any{
				"status":      model.AssetStatusProcessing,
				"update_time": time.Now(),
				"error":       nil,
			}).Error
	})
}

func syncAssetChannel(ctx context.Context, row *model.AssetChannel, adaptor assetrelay.Adaptor, groupID string) error {
	if row.UpstreamAssetId == "" {
		var local model.Asset
		if err := model.DB.First(&local, "id = ?", row.AssetId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if row.Status == model.AssetStatusProcessing {
			return saveAssetSyncFailure(row, "Upstream creation result is unknown; reconciliation is pending", false)
		}
		var moderation dto.AssetModeration
		if local.Moderation != nil {
			if err := common.UnmarshalJsonStr(*local.Moderation, &moderation); err != nil {
				return saveAssetSyncFailure(row, "Invalid asset moderation configuration", true)
			}
		}
		claimed := model.DB.Model(&model.AssetChannel{}).Where("id = ?", row.Id).
			Where("status = ?", model.AssetStatusPending).
			Where("upstream_asset_id = ?", "").
			Update("status", model.AssetStatusProcessing)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected == 0 {
			return nil
		}
		row.Status = model.AssetStatusProcessing
		assetRequest := dto.CreateAssetRequest{
			AssetType:  local.AssetType,
			Moderation: &moderation,
			URL:        local.URL,
			Name:       local.Id,
			GroupId:    groupID,
		}
		created, err := adaptor.CreateAssets(&assetRequest)
		if err != nil || created == nil || created.Id == "" {
			return saveAssetSyncFailure(row, "Upstream creation result is unknown; reconciliation is pending", false)
		}
		row.UpstreamAssetId = created.Id
		return model.DB.Model(&model.AssetChannel{}).Where("id = ?", row.Id).
			Updates(map[string]any{
				"next_poll_at":      time.Now().Add(assetSyncPollInterval).Unix(),
				"status":            model.AssetStatusProcessing,
				"upstream_asset_id": row.UpstreamAssetId,
				"retry_count":       0,
				"error":             nil,
			}).Error
	}
	remote, err := adaptor.GetAsset(&dto.GetAssetRequest{Id: row.UpstreamAssetId})
	if err != nil {
		return saveAssetSyncFailure(row, "Unable to retrieve a valid upstream asset", false)
	}
	switch remote.Status {
	case model.AssetStatusActive, model.AssetStatusProcessing:
		return model.DB.Model(&model.AssetChannel{}).Where("id = ?", row.Id).
			Updates(map[string]any{
				"next_poll_at": time.Now().Add(assetSyncPollInterval).Unix(),
				"status":       remote.Status,
				"retry_count":  0,
				"error":        nil,
			}).Error
	case model.AssetStatusFailed:
		failure := datatypes.JSON(`{"Code":"UpstreamAssetFailed","Message":"Upstream asset processing or moderation failed"}`)
		if remote.Error != nil {
			failure = datatypes.JSON(*remote.Error)
		}
		return model.DB.Model(&model.AssetChannel{}).Where("id = ?", row.Id).
			Updates(map[string]any{
				"status":       model.AssetStatusFailed,
				"error":        failure,
				"next_poll_at": 0,
			}).Error
	default:
		return saveAssetSyncFailure(row, "Upstream returned an unsupported asset status", false)
	}
}

func upstreamAssetSyncGroup(adaptor assetrelay.Adaptor, channelID int) (string, error) {
	var mapping model.AssetGroupChannel
	err := model.DB.Where("channel_id = ?", channelID).First(&mapping).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if mapping.UpstreamGroupId != "" {
		return mapping.UpstreamGroupId, nil
	}
	request := dto.CreateAssetGroupRequest{
		Name:      generateAssetScopedID("666api-assets"),
		GroupType: "AIGC",
	}
	created, err := adaptor.CreateAssetGroup(&request)
	if err != nil {
		return "", err
	}
	mapping.ChannelId = channelID
	mapping.UpstreamGroupId = created.Id
	err = model.DB.Create(&mapping).Error
	return created.Id, err
}

func saveAssetSyncFailure(row *model.AssetChannel, message string, terminal bool) error {
	retries := row.RetryCount + 1
	status := row.Status
	if row.UpstreamAssetId == "" {
		status = model.AssetStatusPending
	}
	if terminal || retries >= assetSyncMaxRetries {
		status = model.AssetStatusFailed
	}
	encoded, err := common.Marshal(map[string]string{"Code": "AssetSyncFailed", "Message": message})
	if err != nil {
		return err
	}
	next := time.Now().Add(min(15*time.Second*time.Duration(1<<min(retries-1, 5)), 5*time.Minute)).Unix()
	if status == model.AssetStatusFailed {
		next = 0
	}
	return model.DB.Model(&model.AssetChannel{}).Where("id = ?", row.Id).
		Updates(map[string]any{
			"error":        datatypes.JSON(encoded),
			"retry_count":  retries,
			"status":       status,
			"next_poll_at": next,
		}).Error
}
