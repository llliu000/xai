package model

import (
	"github.com/QuantumNous/new-api/video/dto"
	"gorm.io/datatypes"
)

type VideoEnhance struct {
	ID                int64                                        `json:"id"`
	OriginTaskID      string                                       `json:"origin_task_id"`
	OriginParams      datatypes.JSONType[dto.ArkCreateTaskRequest] `json:"origin_params"`
	OriginResolution  string                                       `json:"origin_resolution"`
	EnhanceResolution string                                       `json:"enhance_resolution"`
	RunningResult     *datatypes.JSONType[dto.ArkGetTaskResponse]  `json:"running_result"`
	EnhanceTaskID     string                                       `json:"enhance_task_id"`
	EnhanceParams     string                                       `json:"enhance_params"`
}
