package mock

import (
	"context"

	"github.com/QuantumNous/new-api/video/enhance"
	"github.com/google/uuid"
)

type Adapter struct {
}

func (a *Adapter) CreateTask(ctx context.Context, req enhance.CreateTaskRequest) (*enhance.CreateTaskResponse, error) {
	return &enhance.CreateTaskResponse{ID: uuid.NewString()}, nil
}

func (a *Adapter) GetTask(ctx context.Context, taskID string) (*enhance.GetTaskResponse, error) {
	return &enhance.GetTaskResponse{
		Status:   enhance.TaskStatusSucceeded,
		VideoURL: "https://www.baidu.com",
		TaskID:   taskID,
	}, nil
}
