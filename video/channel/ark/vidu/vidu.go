package vidu

import (
	"context"

	"github.com/QuantumNous/new-api/video/dto"
)

type Adaptor struct {
}

func (a *Adaptor) Name() string {
	return "vidu"
}

func (a *Adaptor) CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	return nil, nil
}

func (a *Adaptor) GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	return nil, nil
}
