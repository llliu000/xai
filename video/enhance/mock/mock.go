package mock

import (
	"context"

	"github.com/QuantumNous/new-api/video/enhance/dto"
	"github.com/google/uuid"
)

type Adapter struct {
}

func (a *Adapter) CreateTask(ctx context.Context, req dto.CreateTaskRequest) (*dto.CreateTaskResponse, error) {
	return &dto.CreateTaskResponse{ID: uuid.NewString()}, nil
}

func (a *Adapter) GetTask(ctx context.Context, taskID string) (*dto.GetTaskResponse, error) {
	return &dto.GetTaskResponse{
		Status:   dto.TaskStatusSucceeded,
		VideoURL: "https://www.pexels.com/zh-cn/download/video/27375628/",
		TaskID:   taskID,
	}, nil
}
