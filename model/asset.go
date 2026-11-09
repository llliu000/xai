package model

import (
	"time"

	"github.com/QuantumNous/new-api/dto"
	"gorm.io/datatypes"
)

const (
	AssetRegionDomestic = "cn-beijing"
	AssetRegionOverseas = "ap-southeast-1"

	AssetStatusPending    = "Pending"
	AssetStatusProcessing = "Processing"
	AssetStatusActive     = "Active"
	AssetStatusFailed     = "Failed"
)

// AssetGroup 是本地素材分组，不直接使用上游分组 ID。
// 本地分组用于素材分类，上游分组由 AssetGroupChannel 按渠道统一管理。
type AssetGroup struct {
	Id          string    `json:"id" gorm:"type:varchar(191);primaryKey"`
	Region      string    `json:"region" gorm:"type:varchar(32);not null;default:cn-beijing;index"`
	UserId      int       `json:"user_id" gorm:"not null;index"`
	Name        string    `json:"name" gorm:"type:varchar(64);not null;index"`
	Description string    `json:"description" gorm:"type:varchar(300)"`
	GroupType   string    `json:"group_type" gorm:"type:varchar(50);not null;index"` // AIGC、LivenessFace；真人分组需单独完成上游认证。
	ProjectName string    `json:"project_name" gorm:"type:varchar(191);index"`       // 项目名称，如 default
	CreateTime  time.Time `json:"create_time" gorm:"bigint"`
	UpdateTime  time.Time `json:"update_time" gorm:"bigint"`
}

// Asset 保存本地素材身份和可供渠道重新上传的持久原始地址。
// Status 在上传时确定的全部自动同步渠道均 Active 后变为 Active，后续手动补同步不回退此状态。
// 生成任务仍需检查选中渠道上的全部素材映射，不能只依赖此汇总状态。
type Asset struct {
	Id          string                                   `json:"id" gorm:"type:varchar(191);primaryKey"`
	Region      string                                   `json:"region" gorm:"type:varchar(32);not null;default:cn-beijing;index"`
	UserId      int                                      `json:"user_id" gorm:"not null;index"`
	GroupId     string                                   `json:"group_id" gorm:"type:varchar(191);not null;index"`
	URL         string                                   `json:"url" gorm:"type:text;not null"` // 持久原始地址，不保存上游详情返回的临时 URL。
	Name        string                                   `json:"name" gorm:"type:varchar(64);not null;index"`
	AssetType   string                                   `json:"asset_type" gorm:"type:varchar(50);not null;index"` // Image、Video、Audio。
	ProjectName string                                   `json:"project_name" gorm:"type:varchar(191);index"`       // 项目名称，如 default
	Status      string                                   `json:"status" gorm:"type:varchar(50);not null;index"`     // 任务状态，如 Active、Processing、Failed
	Moderation  *datatypes.JSONType[dto.AssetModeration] `json:"moderation" gorm:"type:varchar(200)"`               // 审核配置 {"Strategy": "Skip"}
	Error       *datatypes.JSONType[dto.AssetError]      `json:"error" gorm:"type:jsonb"`                           // 错误信息
	CreateTime  time.Time                                `json:"create_time" gorm:"bigint"`
	UpdateTime  time.Time                                `json:"update_time" gorm:"bigint"`
}

// AssetGroupChannel 保存每个渠道唯一的上游分组 ID，供该渠道的全部素材复用。
// 同一渠道的密钥必须属于相同账号和项目；更换账号或项目应使用新渠道。
type AssetGroupChannel struct {
	Id              int64  `json:"id" gorm:"primaryKey"`
	Region          string `json:"region" gorm:"type:varchar(32);not null;default:cn-beijing;index"`
	ChannelId       int    `json:"channel_id" gorm:"not null;index;uniqueIndex:uk_asset_group_channel"`
	GroupId         string `json:"group_id" gorm:"type:varchar(191);not null;uniqueIndex:uk_asset_group_channel"`
	UpstreamGroupId string `json:"upstream_group_id" gorm:"type:varchar(191);index"`
	CreateTime      int64  `json:"create_time" gorm:"autoCreateTime"`
	UpdateTime      int64  `json:"update_time" gorm:"autoUpdateTime"`
}

// AssetChannel 保存素材在每个渠道的上传、审核结果和上游 ID。
// 上传前先建立 Pending 记录；获得上游 ID 后轮询详情，Active 才能用于生成。
// Manual 区分手动补同步和上传时自动同步，手动补同步不影响本地素材状态。
// 上游分组通过 ChannelId 对应的 AssetGroupChannel 获取。
type AssetChannel struct {
	Id              int64           `json:"id" gorm:"primaryKey;index:idx_asset_channel_manual_status_poll,priority:4"`
	Region          string          `json:"region" gorm:"type:varchar(32);not null;default:cn-beijing;index"`
	UserId          int             `json:"user_id" gorm:"not null;index"`
	AssetId         string          `json:"asset_id" gorm:"type:varchar(191);not null;uniqueIndex:uk_asset_channel,priority:1"`
	ChannelId       int             `json:"channel_id" gorm:"not null;index;uniqueIndex:uk_asset_channel,priority:2"`
	Manual          bool            `json:"manual" gorm:"index:idx_asset_channel_manual_status_poll,priority:1"`                                                                          // true 为手动补同步，false 为上传时自动同步；创建后保持不变。
	UpstreamAssetId string          `json:"upstream_asset_id" gorm:"type:varchar(191);index"`                                                                                             // 上传成功前为空；获得 ID 不代表审核通过。
	Status          string          `json:"status" gorm:"type:varchar(50);not null;index:idx_asset_channel_status_poll,priority:1;index:idx_asset_channel_manual_status_poll,priority:2"` // Pending 为本地待上传，其余状态对应上游结果。
	Error           *datatypes.JSON `json:"error" gorm:"type:jsonb"`                                                                                                                      // 错误信息
	RetryCount      int             `json:"retry_count" gorm:"not null"`                                                                                                                  // 同步请求连续失败次数，成功后清零。
	NextPollAt      int64           `json:"next_poll_at" gorm:"not null;index:idx_asset_channel_status_poll,priority:2;index:idx_asset_channel_manual_status_poll,priority:3"`            // 下次同步的 Unix 秒，0 表示未调度。
	CreateTime      int64           `json:"create_time" gorm:"autoCreateTime"`
	UpdateTime      int64           `json:"update_time" gorm:"autoUpdateTime"`

	Asset Asset `gorm:"foreignKey:AssetId;references:Id"`
}

func NormalizeAssetRegion(region string) string {
	if region == "" {
		return AssetRegionDomestic
	}
	return region
}

func GetUpstreamAssetID(channelID int, assetID string) (string, error) {
	var mapping AssetChannel
	err := DB.Select("upstream_asset_id").
		Where("status = ?", AssetStatusActive).
		Where("channel_id = ?", channelID).
		Where("asset_id = ?", assetID).
		Take(&mapping).Error
	return mapping.UpstreamAssetId, err
}
