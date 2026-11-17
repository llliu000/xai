package service

import (
	"github.com/QuantumNous/new-api/video/channel"
	"github.com/QuantumNous/new-api/video/channel/wan"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/gin-gonic/gin"
)

type Wan struct {
	registry *channel.Registry[wan.Adapter]
}

func NewWan() *Wan {
	return &Wan{registry: channel.DefaultWanRegistry()}
}

func (w *Wan) CreateTask(ctx *gin.Context, req dto.WanCreateTaskRequest) (*dto.WanCreateTaskResponse, error) {
	adapter, err := w.registry.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.CreateTask(ctx, req)
}

func (w *Wan) GetTask(ctx *gin.Context, req dto.WanGetTaskRequest) (*dto.WanGetTaskResponse, error) {
	adapter, err := w.registry.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.GetTask(ctx, req)
}
