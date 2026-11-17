package vidu

import (
	"context"

	"github.com/QuantumNous/new-api/video/dto"
)

type Adapter struct {
}

func (a *Adapter) Name() string {
	return "vidu"
}

func (a *Adapter) CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	return nil, nil
}

func (a *Adapter) GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	return nil, nil
}
