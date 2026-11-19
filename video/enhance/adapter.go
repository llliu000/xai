package enhance

import (
	"context"
	"fmt"
)

type Adapter interface {
	CreateTask(ctx context.Context, req CreateTaskRequest) (*CreateTaskResponse, error)
	GetTask(ctx context.Context, taskID string) (*GetTaskResponse, error)
}

func GetConfig(enhanceResolution string) (*ResolutionRatio, error) {
	resolutionRatio, ok := TargetResolutionMapping[enhanceResolution]
	if !ok {
		return nil, fmt.Errorf("not support enhance resolution %s", enhanceResolution)
	}
	return &resolutionRatio, nil
}

var TargetResolutionMapping = map[string]ResolutionRatio{
	"720p": {
		Origin: "480p",
		Ratio:  2.5,
	},
	"1080p": {
		Origin: "480p",
		Ratio:  5,
	},
	"4k": {
		Origin: "1080p",
		Ratio:  10,
	},
}

type ResolutionRatio struct {
	Origin string
	Ratio  float64
}

type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
)

type CreateTaskRequest struct {
	VideoURL   string
	Resolution *string
}
type CreateTaskResponse struct {
	ID string
}

type GetTaskResponse struct {
	Status   TaskStatus
	VideoURL string
	TaskID   string
}
