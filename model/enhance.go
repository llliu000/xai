package model

import (
	"time"

	"github.com/QuantumNous/new-api/video/dto"
	enhanceDto "github.com/QuantumNous/new-api/video/enhance/dto"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type VideoEnhance struct {
	ID                int64                                        `json:"id" gorm:"primaryKey"`
	VideoTaskID       string                                       `json:"video_task_id" gorm:"type:varchar(191);index"`
	VideoTaskParams   datatypes.JSONType[dto.ArkCreateTaskRequest] `json:"video_task_params"`
	VideoResult       *datatypes.JSONType[dto.ArkGetTaskResponse]  `json:"video_result"`
	Status            enhanceDto.TaskStatus                        `json:"status" gorm:"type:varchar(16);not null;default:queued"`
	VideoResolution   string                                       `json:"video_resolution" gorm:"type:varchar(32)"`
	EnhanceResolution string                                       `json:"enhance_resolution" gorm:"type:varchar(32)"`
	EnhanceTaskID     *string                                      `json:"enhance_task_id" gorm:"type:varchar(191);index"`
	RetryCount        int                                          `json:"retry_count" gorm:"type:int"`
	Reason            *string                                      `json:"reason,omitempty" gorm:"type:varchar(1000)"`
	CreatedAt         time.Time                                    `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         time.Time                                    `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ve *VideoEnhance) Create() error {
	return DB.Create(ve).Error
}

func (ve *VideoEnhance) UpdateEnhanceTaskID() error {
	if ve.EnhanceTaskID == nil {
		return nil
	}
	return DB.Model(&VideoEnhance{}).Where("video_task_id = ?", ve.VideoTaskID).
		Update("enhance_task_id", ve.EnhanceTaskID).Error
}

func (ve *VideoEnhance) UpdateVideoResult() error {
	result := DB.Model(&VideoEnhance{}).Where("video_task_id = ? AND status NOT IN ?", ve.VideoTaskID,
		[]enhanceDto.TaskStatus{enhanceDto.TaskStatusSucceeded, enhanceDto.TaskStatusFailed}).
		Updates(map[string]any{"status": ve.Status, "video_result": ve.VideoResult})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 { // 已被其他请求更新为终态，读取实际保存的结果。
		return DB.Where("video_task_id = ?", ve.VideoTaskID).First(ve).Error
	}
	return nil
}

func (ve *VideoEnhance) FindByVideoTaskId(videoTaskId string) error {
	return DB.Where("video_task_id = ?", videoTaskId).Find(ve).Error
}

func (ve *VideoEnhance) UpdateFailed(reason string) error {
	return DB.Model(&VideoEnhance{}).Where("video_task_id = ?", ve.VideoTaskID).
		Updates(map[string]any{
			"retry_count": gorm.Expr("retry_count + 1"),
			"reason":      reason,
		}).Error
}
