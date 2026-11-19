package model

import (
	"time"

	"github.com/QuantumNous/new-api/video/dto"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type VideoEnhance struct {
	ID                 int64                                        `json:"id" gorm:"primaryKey"`
	VideoTaskID        string                                       `json:"video_task_id" gorm:"type:varchar(191);index"`
	VideoTaskParams    datatypes.JSONType[dto.ArkCreateTaskRequest] `json:"video_task_params"`
	VideoRunningResult *datatypes.JSONType[dto.ArkGetTaskResponse]  `json:"video_running_result"`
	VideoResolution    string                                       `json:"video_resolution" gorm:"type:varchar(32)"`
	EnhanceResolution  string                                       `json:"enhance_resolution" gorm:"type:varchar(32)"`
	EnhanceTaskID      *string                                      `json:"enhance_task_id" gorm:"type:varchar(191);index"`
	RetryCount         int                                          `json:"retry_count" gorm:"type:int"`
	Reason             *string                                      `json:"reason,omitempty" gorm:"type:varchar(1000)"`
	CreatedAt          time.Time                                    `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          time.Time                                    `json:"updated_at" gorm:"autoUpdateTime"`
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

func (ve *VideoEnhance) UpdateVideoRunningResult() error {
	return DB.Model(&VideoEnhance{}).Where("video_task_id = ?", ve.VideoTaskID).
		Update("video_running_result", ve.VideoRunningResult).Error
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
