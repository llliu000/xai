package ark

import (
	"context"

	"github.com/QuantumNous/new-api/video/channel/ark/mock"
	"github.com/QuantumNous/new-api/video/channel/ark/vidu"
	"github.com/QuantumNous/new-api/video/common"
	"github.com/QuantumNous/new-api/video/dto"
)

var Mapping = map[string]Adaptor{
	common.Mock: &mock.Adaptor{},
	common.Vidu: &vidu.Adaptor{},
}

type Adaptor interface {
	CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error)
	GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error)
}
