package wan

import (
	"context"

	"github.com/QuantumNous/new-api/video/channel/wan/mock"
	"github.com/QuantumNous/new-api/video/common"
	"github.com/QuantumNous/new-api/video/dto"
)

var Mapping = map[string]Adaptor{
	common.Mock: &mock.Adaptor{},
}

type Adaptor interface {
	CreateTask(ctx context.Context, req dto.WanCreateTaskRequest) (*dto.WanCreateTaskResponse, error)
	GetTask(ctx context.Context, req dto.WanGetTaskRequest) (*dto.WanGetTaskResponse, error)
}
