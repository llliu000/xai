package mock

import (
	"context"

	"github.com/QuantumNous/new-api/video/dto"
)

type Adapter struct{}

func (a *Adapter) CreateTask(ctx context.Context, req dto.WanCreateTaskRequest) (*dto.WanCreateTaskResponse, error) {
	return nil, nil
}

func (a *Adapter) GetTask(ctx context.Context, req dto.WanGetTaskRequest) (*dto.WanGetTaskResponse, error) {
	return nil, nil
}
