package ark

import (
	"context"

	"github.com/QuantumNous/new-api/video/dto"
)

type Adaptor interface {
	CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error)
	GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error)
}
