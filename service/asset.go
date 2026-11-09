package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetrelay "github.com/QuantumNous/new-api/relay/asset"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultAssetProjectName = "default"
)

func GetSeedanceChannelOptions() ([]dto.SeedanceChannelOption, error) {
	var channels []model.Channel
	if err := model.DB.Select("id", "name", "status", "setting").
		Where("type = ?", constant.ChannelTypeTaskPlugin).
		Order("id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	options := make([]dto.SeedanceChannelOption, 0, len(channels))
	for _, channel := range channels {
		if channel.GetSetting().TaskPluginKey == "doubao" {
			options = append(options, dto.SeedanceChannelOption{Id: channel.Id, Name: channel.Name, Status: channel.Status})
		}
	}
	return options, nil
}

func UpdateUserSeedanceChannels(user *model.User, req *dto.UpdateUserSeedanceChannelsRequest) (*dto.UserSeedanceChannels, error) {
	var settings kitdto.UserSetting
	if user.Setting != "" {
		if err := common.UnmarshalJsonStr(user.Setting, &settings); err != nil {
			return nil, err
		}
	}
	if req.SeedanceInnerChannelID != nil {
		settings.SeedanceInnerChannelID = *req.SeedanceInnerChannelID
	}
	if req.SeedanceOuterChannelID != nil {
		settings.SeedanceOuterChannelID = *req.SeedanceOuterChannelID
	}
	if err := model.UpdateUserSetting(user.Id, settings); err != nil {
		return nil, err
	}
	return &dto.UserSeedanceChannels{
		SeedanceInnerChannelID: settings.SeedanceInnerChannelID,
		SeedanceOuterChannelID: settings.SeedanceOuterChannelID,
	}, nil
}

func CreateAssetGroup(ctx *gin.Context, req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	group := &model.AssetGroup{
		ProjectName: normalizeAssetProjectName(stringValue(req.ProjectName)),
		Id:          generateAssetScopedID("group"),
		Description: stringValue(req.Description),
		UserId:      ctx.GetInt("id"),
		Region:      getRegion(ctx),
		GroupType:   req.GroupType,
		CreateTime:  time.Now(),
		UpdateTime:  time.Now(),
		Name:        req.Name,
	}
	err := model.DB.Create(group).Error
	return &dto.CreateAssetGroupResponse{Id: group.Id}, err
}

func ListAssetGroups(ctx *gin.Context, req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error) {
	pageNumber, pageSize := normalizeAssetPage(req.PageNumber, req.PageSize)
	sortColumn, sortOrder, err := normalizeAssetGroupSort(req.SortBy, req.SortOrder)
	if err != nil {
		return nil, err
	}
	query := model.DB.Model(&model.AssetGroup{}).
		Where("user_id = ?", ctx.GetInt("id")).
		Where("region = ?", getRegion(ctx))
	if projectName := strings.TrimSpace(stringValue(req.ProjectName)); projectName != "" {
		query = query.Where("project_name = ?", projectName)
	} else {
		query = query.Where("project_name = ?", defaultAssetProjectName)
	}
	if req.Filter != nil {
		if len(req.Filter.GroupIds) > 0 {
			query = query.Where("id in ?", req.Filter.GroupIds)
		}
		if req.Filter.Name != nil && *req.Filter.Name != "" {
			query = query.Where("name = ?", strings.TrimSpace(*req.Filter.Name))
		}
		if req.Filter.GroupType != nil && *req.Filter.GroupType != "" {
			query = query.Where("group_type = ?", strings.TrimSpace(*req.Filter.GroupType))
		}
	}
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return nil, err
	}
	groups := make([]model.AssetGroup, 0)
	if err = query.Order(sortColumn + " " + sortOrder).Limit(pageSize).Offset((pageNumber - 1) * pageSize).Find(&groups).Error; err != nil {
		return nil, err
	}
	items := make([]dto.GetAssetGroupResponse, 0, len(groups))
	for i := range groups {
		items = append(items, dto.GetAssetGroupResponse{
			Id:          groups[i].Id,
			Name:        groups[i].Name,
			Description: groups[i].Description,
			GroupType:   groups[i].GroupType,
			ProjectName: groups[i].ProjectName,
			CreateTime:  groups[i].CreateTime.UTC().Format(time.RFC3339),
			UpdateTime:  groups[i].UpdateTime.UTC().Format(time.RFC3339),
		})
	}
	return &dto.ListAssetGroupsResponse{
		Items:      items,
		TotalCount: total,
		PageNumber: pageNumber,
		PageSize:   pageSize,
	}, nil
}

func GetAssetGroup(ctx *gin.Context, req *dto.GetAssetGroupRequest) (*dto.GetAssetGroupResponse, error) {
	group, err := getAssetGroupById(ctx.GetInt("id"), getRegion(ctx), req.Id)
	if err != nil {
		return nil, err
	}
	return &dto.GetAssetGroupResponse{
		CreateTime:  group.CreateTime.UTC().Format(time.RFC3339),
		UpdateTime:  group.UpdateTime.UTC().Format(time.RFC3339),
		Description: group.Description,
		ProjectName: group.ProjectName,
		GroupType:   group.GroupType,
		Name:        group.Name,
		Id:          group.Id,
	}, nil
}

func UpdateAssetGroup(ctx *gin.Context, req *dto.UpdateAssetGroupRequest) (*dto.UpdateAssetGroupResponse, error) {
	group, err := getAssetGroupById(ctx.GetInt("id"), getRegion(ctx), req.Id)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"update_time": time.Now()}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.ProjectName != nil {
		updates["project_name"] = normalizeAssetProjectName(*req.ProjectName)
	}
	err = model.DB.Model(group).Updates(updates).Error
	return &dto.UpdateAssetGroupResponse{Id: group.Id}, err
}

func DeleteAssetGroup(ctx *gin.Context, req *dto.DeleteAssetGroupRequest) (*dto.DeleteAssetGroupResponse, error) {
	userId, region := ctx.GetInt("id"), getRegion(ctx)
	group, err := getAssetGroupById(userId, region, req.Id)
	if err != nil {
		return nil, err
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		assets := tx.Model(&model.Asset{}).Select("id").Where("user_id = ?", userId).
			Where("region = ?", region).Where("group_id = ?", group.Id)
		if err = tx.Where("asset_id IN (?)", assets).Where("region = ?", region).
			Delete(&model.AssetChannel{}).Error; err != nil {
			return err
		}
		if err = tx.Where("user_id = ?", userId).Where("region = ?", region).
			Where("group_id = ?", group.Id).Delete(&model.Asset{}).Error; err != nil {
			return err
		}
		if err = tx.Where("user_id = ?", userId).Where("region = ?", region).
			Where("id = ?", group.Id).Delete(&model.AssetGroup{}).Error; err != nil {
			return err
		}
		if group.GroupType != "LivenessFace" {
			return nil
		}
		if err = tx.Where("group_id = ?", group.Id).Where("region = ?", region).
			Delete(&model.AssetGroupChannel{}).Error; err != nil {
			return err
		}
		return nil
	})
	return &dto.DeleteAssetGroupResponse{}, err
}

func CreateAsset(ctx *gin.Context, req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	userId, region := ctx.GetInt("id"), getRegion(ctx)
	group, err := getAssetGroupById(userId, region, req.GroupId)
	if err != nil {
		return nil, err
	}
	var chs []*model.Channel
	if group.GroupType == "LivenessFace" {
		query := model.DB.Model(&model.AssetGroupChannel{}).Select("channel_id").
			Where("region = ?", region).Where("group_id = ?", req.GroupId)
		if err = model.DB.Where("id IN (?)", query).Find(&chs).Error; err != nil {
			return nil, err
		}
	} else {
		var channels []*model.Channel
		if err = model.DB.Where("type = ?", constant.ChannelTypeTaskPlugin).
			Order("id ASC").Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, channel := range channels {
			setting := channel.GetSetting()
			if setting.TaskPluginKey == "doubao" && setting.Category != "" &&
				model.NormalizeAssetRegion(setting.Region) == region {
				chs = append(chs, channel)
			}
		}
	}
	if len(chs) == 0 {
		return nil, errors.New("no targets found")
	}

	asset := model.Asset{
		ProjectName: normalizeAssetProjectName(stringValue(req.ProjectName)),
		Id:          generateAssetScopedID("asset"),
		Status:      model.AssetStatusProcessing,
		AssetType:   req.AssetType,
		Region:      group.Region,
		GroupId:     req.GroupId,
		CreateTime:  time.Now(),
		UpdateTime:  time.Now(),
		Name:        req.Name,
		URL:         req.URL,
		UserId:      userId,
	}
	if req.Moderation != nil {
		asset.Moderation = new(datatypes.NewJSONType(*req.Moderation))
	}
	acs := make([]model.AssetChannel, 0, len(chs))
	for _, ch := range chs {
		acs = append(acs, model.AssetChannel{
			Status:    model.AssetStatusPending,
			Region:    asset.Region,
			UserId:    asset.UserId,
			AssetId:   asset.Id,
			ChannelId: ch.Id,
			Manual:    false,
		})
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err = tx.Create(&asset).Error; err != nil {
			return err
		}
		return tx.Create(&acs).Error
	})
	if err != nil {
		return nil, err
	}
	_, _, err = EnqueueSystemTask(assetSyncTaskType, nil)
	return &dto.CreateAssetResponse{Id: asset.Id}, err
}

func ListAssets(ctx *gin.Context, req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error) {
	userId, region := ctx.GetInt("id"), getRegion(ctx)
	pageNumber, pageSize := normalizeAssetPage(req.PageNumber, req.PageSize)
	sortColumn, sortOrder, err := normalizeAssetSort(req.SortBy, req.SortOrder)
	if err != nil {
		return nil, err
	}
	query := model.DB.Model(&model.Asset{}).Where("user_id = ?", userId).Where("region = ?", region)
	if projectName := strings.TrimSpace(stringValue(req.ProjectName)); projectName != "" {
		query = query.Where("project_name = ?", projectName)
	} else {
		query = query.Where("project_name = ?", defaultAssetProjectName)
	}
	if req.Filter != nil {
		if groupIds := normalizeNonEmptyStrings(req.Filter.GroupIds); len(groupIds) > 0 {
			query = query.Where("group_id IN ?", groupIds)
		}
		if req.Filter.GroupType != nil && *req.Filter.GroupType != "" {
			groupType := strings.TrimSpace(*req.Filter.GroupType)
			query = query.Where("group_id IN (?)", model.DB.Model(&model.AssetGroup{}).Select("id").
				Where("user_id = ?", userId).Where("region = ?", region).Where("group_type = ?", groupType))
		}
		if statuses := normalizeNonEmptyStrings(req.Filter.Statuses); len(statuses) > 0 {
			query = query.Where("status IN ?", statuses)
		}
		if req.Filter.Name != nil && *req.Filter.Name != "" {
			query = query.Where("name = ?", strings.TrimSpace(*req.Filter.Name))
		}
	}
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return nil, err
	}
	assets := make([]model.Asset, 0)
	if err = query.Order(sortColumn + " " + sortOrder).Limit(pageSize).Offset((pageNumber - 1) * pageSize).Find(&assets).Error; err != nil {
		return nil, err
	}
	items := make([]dto.GetAssetResponse, 0, len(assets))
	for i := range assets {
		response := dto.GetAssetResponse{
			Id:          assets[i].Id,
			GroupId:     assets[i].GroupId,
			URL:         assets[i].URL,
			Name:        assets[i].Name,
			AssetType:   assets[i].AssetType,
			ProjectName: assets[i].ProjectName,
			Status:      assets[i].Status,
			CreateTime:  assets[i].CreateTime.UTC().Format(time.RFC3339),
			UpdateTime:  assets[i].UpdateTime.UTC().Format(time.RFC3339),
		}
		if assets[i].Error != nil {
			response.Error = new(assets[i].Error.Data())
		}
		items = append(items, response)
	}
	return &dto.ListAssetsResponse{
		Items:      items,
		TotalCount: total,
		PageNumber: pageNumber,
		PageSize:   pageSize,
	}, nil
}

func GetAsset(ctx *gin.Context, req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	userId, region := ctx.GetInt("id"), getRegion(ctx)
	asset, err := getAssetById(userId, region, req.Id)
	if err != nil {
		return nil, err
	}
	response := dto.GetAssetResponse{
		CreateTime:  asset.CreateTime.UTC().Format(time.RFC3339),
		UpdateTime:  asset.UpdateTime.UTC().Format(time.RFC3339),
		ProjectName: asset.ProjectName,
		AssetType:   asset.AssetType,
		GroupId:     asset.GroupId,
		Status:      asset.Status,
		Name:        asset.Name,
		URL:         asset.URL,
		Id:          asset.Id,
	}
	if asset.Error != nil {
		response.Error = new(asset.Error.Data())
	}
	return &response, nil
}

func getAssetById(userId int, region, id string) (*model.Asset, error) {
	var asset model.Asset
	if err := model.DB.Where("user_id = ?", userId).Where("region = ?", region).Where("id = ?", id).First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("asset %s not found", id)
		}
		return nil, err
	}
	return &asset, nil
}

func UpdateAsset(ctx *gin.Context, req *dto.UpdateAssetRequest) (*dto.UpdateAssetResponse, error) {
	userId, region := ctx.GetInt("id"), getRegion(ctx)
	asset, err := getAssetById(userId, region, req.Id)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"update_time": time.Now()}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.ProjectName != nil {
		updates["project_name"] = normalizeAssetProjectName(*req.ProjectName)
	}
	if err = model.DB.Model(asset).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &dto.UpdateAssetResponse{Id: asset.Id}, nil
}

func DeleteAsset(ctx *gin.Context, req *dto.DeleteAssetRequest) (*dto.DeleteAssetResponse, error) {
	userId, region := ctx.GetInt("id"), getRegion(ctx)
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		assets := tx.Model(&model.Asset{}).Select("id").Where("user_id = ?", userId).
			Where("region = ?", region).Where("id = ?", req.Id)
		if err := tx.Where("asset_id IN (?)", assets).Where("region = ?", region).
			Delete(&model.AssetChannel{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", userId).Where("region = ?", region).
			Where("id = ?", req.Id).Delete(&model.Asset{}).Error
	})
	return &dto.DeleteAssetResponse{}, err
}

func CreateVisualValidateSession(c *gin.Context, req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error) {
	channel, err := getUserSeedanceChannel(c)
	if err != nil {
		return nil, err
	}
	adaptor, err := assetrelay.GetAdaptor(channel)
	if err != nil {
		return nil, err
	}
	return adaptor.CreateVisualValidateSession(req)
}

func GetVisualValidateResult(c *gin.Context, req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error) {
	channel, err := getUserSeedanceChannel(c)
	if err != nil {
		return nil, err
	}
	adaptor, err := assetrelay.GetAdaptor(channel)
	if err != nil {
		return nil, err
	}
	result, err := adaptor.GetVisualValidateResult(req)
	if err != nil {
		return nil, err
	}
	if result.GroupId == "" {
		return result, nil
	}
	var count int64
	if err = model.DB.Model(&model.AssetGroup{}).
		Where("region = ?", getRegion(c)).
		Where("id = ?", result.GroupId).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return result, nil
	}
	// 创建本地资源并返回
	group := model.AssetGroup{
		UserId:      c.GetInt("id"),
		Id:          result.GroupId,
		GroupType:   "LivenessFace",
		Name:        "real-human",
		Description: "real-human",
		Region:      getRegion(c),
		CreateTime:  time.Now(),
		UpdateTime:  time.Now(),
		ProjectName: "default",
	}
	groupChannel := model.AssetGroupChannel{
		UpstreamGroupId: result.GroupId,
		Region:          getRegion(c),
		ChannelId:       channel.Id,
		GroupId:         group.Id,
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err = tx.Create(&group).Error; err != nil {
			return err
		}
		return tx.Create(&groupChannel).Error
	})
	return result, err
}

func ResolveAssetReferences(info *relaycommon.RelayInfo, body map[string]any) error {
	content, _ := body["content"].([]any)
	for _, value := range content {
		item, _ := value.(map[string]any)
		kind, _ := item["type"].(string)
		switch kind {
		case "image_url", "video_url", "audio_url":
		default:
			continue
		}
		media, _ := item[kind].(map[string]any)
		mediaURL, _ := media["url"].(string)
		assetID, local := strings.CutPrefix(mediaURL, "asset://")
		if !local {
			continue
		}
		upstreamID, err := model.GetUpstreamAssetID(info.ChannelId, assetID)
		if err != nil {
			return fmt.Errorf("%s not active", assetID)
		}

		if !strings.HasPrefix(assetID, "http") {
			media["url"] = "asset://" + upstreamID
		} else {
			media["url"] = assetID
		}
	}
	return nil
}

func getAssetGroupById(userId int, region, id string) (*model.AssetGroup, error) {
	var group model.AssetGroup
	err := model.DB.Where("user_id = ?", userId).
		Where("region = ?", region).
		Where("id = ?", id).
		First(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("asset group %s not found", id)
	}
	return &group, err
}

// 真人认证必须要绑定渠道才能使用
func getUserSeedanceChannel(ctx *gin.Context) (*model.Channel, error) {
	region := getRegion(ctx)
	// 获取绑定的渠道
	settings, _ := common.GetContextKeyType[kitdto.UserSetting](ctx, constant.ContextKeyUserSetting)
	var channelId int
	switch region {
	case model.AssetRegionDomestic:
		channelId = settings.SeedanceInnerChannelID
	case model.AssetRegionOverseas:
		channelId = settings.SeedanceOuterChannelID
	}
	if channelId <= 0 {
		return nil, errors.New("channelId is invalid")
	}
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		return nil, err
	}
	if model.NormalizeAssetRegion(channel.GetSetting().Region) != region {
		return nil, errors.New("invalid region")
	}
	return channel, nil
}

func getRegion(ctx *gin.Context) string {
	region := ctx.Query("Region")
	return model.NormalizeAssetRegion(region)
}

func normalizeAssetProjectName(projectName string) string {
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return defaultAssetProjectName
	}
	return projectName
}

func normalizeAssetPage(pageNumber int, pageSize int) (int, int) {
	if pageNumber < 1 {
		pageNumber = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return pageNumber, pageSize
}

func normalizeAssetGroupSort(sortBy string, sortOrder string) (string, string, error) {
	var column string
	switch strings.TrimSpace(sortBy) {
	case "", "CreateTime":
		column = "create_time"
	case "UpdateTime":
		column = "update_time"
	default:
		return "", "", fmt.Errorf("unsupported SortBy: %s", sortBy)
	}

	switch strings.TrimSpace(sortOrder) {
	case "", "Desc":
		return column, "desc", nil
	case "Asc":
		return column, "asc", nil
	default:
		return "", "", fmt.Errorf("unsupported SortOrder: %s", sortOrder)
	}
}

func normalizeAssetSort(sortBy string, sortOrder string) (string, string, error) {
	var column string
	switch strings.TrimSpace(sortBy) {
	case "", "CreateTime":
		column = "create_time"
	case "UpdateTime":
		column = "update_time"
	case "GroupId":
		column = "group_id"
	default:
		return "", "", fmt.Errorf("unsupported SortBy: %s", sortBy)
	}

	switch strings.TrimSpace(sortOrder) {
	case "", "Desc":
		return column, "desc", nil
	case "Asc":
		return column, "asc", nil
	default:
		return "", "", fmt.Errorf("unsupported SortOrder: %s", sortOrder)
	}
}

func normalizeNonEmptyStrings(values []string) []string {
	results := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			results = append(results, value)
		}
	}
	return results
}

func generateAssetScopedID(prefix string) string {
	key, err := common.GenerateRandomCharsKey(5)
	if err != nil {
		key = common.GetRandomString(5)
	}
	datetime := time.Now().Format("20060102150405")
	return fmt.Sprintf("%s-%s-%s", prefix, datetime, key)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
