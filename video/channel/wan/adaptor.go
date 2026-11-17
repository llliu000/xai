package wan

import (
	"context"

	"github.com/QuantumNous/new-api/video/dto"
)

type Adapter interface {
	CreateTask(ctx context.Context, req dto.WanCreateTaskRequest) (*dto.WanCreateTaskResponse, error)
	GetTask(ctx context.Context, req dto.WanGetTaskRequest) (*dto.WanGetTaskResponse, error)
}
