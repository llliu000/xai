package service

import (
	"github.com/QuantumNous/new-api/video/channel"
	"github.com/QuantumNous/new-api/video/channel/ark"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/gin-gonic/gin"
)

type Ark struct {
	registry *channel.Registry[ark.Adaptor]
}

func NewArk() *Ark {
	return &Ark{registry: channel.DefaultArkRegistry()}
}

func (a *Ark) CreateTask(ctx *gin.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	adaptor, err := a.registry.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.CreateTask(ctx, req)
}

func (a *Ark) GetTask(ctx *gin.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	adaptor, err := a.registry.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.GetTask(ctx, req)
}
