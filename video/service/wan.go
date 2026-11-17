package service

import (
	"github.com/QuantumNous/new-api/video/channel"
	"github.com/QuantumNous/new-api/video/channel/wan"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/gin-gonic/gin"
)

type Wan struct {
	registry *channel.Registry[wan.Adaptor]
}

func NewWan() *Wan {
	return &Wan{registry: channel.DefaultWanRegistry()}
}

func (w *Wan) CreateTask(ctx *gin.Context, req dto.WanCreateTaskRequest) (*dto.WanCreateTaskResponse, error) {
	adaptor, err := w.registry.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.CreateTask(ctx, req)
}

func (w *Wan) GetTask(ctx *gin.Context, req dto.WanGetTaskRequest) (*dto.WanGetTaskResponse, error) {
	adaptor, err := w.registry.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.GetTask(ctx, req)
}
