package mock

import (
	"context"

	"github.com/QuantumNous/new-api/video/dto"
)

type Adaptor struct{}

func (a *Adaptor) CreateTask(ctx context.Context, req dto.WanCreateTaskRequest) (*dto.WanCreateTaskResponse, error) {
	return nil, nil
}

func (a *Adaptor) GetTask(ctx context.Context, req dto.WanGetTaskRequest) (*dto.WanGetTaskResponse, error) {
	return nil, nil
}
