package service

import (
	"errors"

	"github.com/QuantumNous/new-api/video/channel/ark"
	"github.com/QuantumNous/new-api/video/common"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/gin-gonic/gin"
)

type Ark struct {
}

func (a *Ark) CreateTask(ctx *gin.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	channelName := common.GetRelayContext(ctx).ChannelName
	adaptor, ok := ark.Mapping[channelName]
	if !ok {
		return nil, errors.New("no adaptor")
	}
	return adaptor.CreateTask(ctx, req)
}

func (a *Ark) GetTask(ctx *gin.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	channelName := common.GetRelayContext(ctx).ChannelName
	adaptor, ok := ark.Mapping[channelName]
	if !ok {
		return nil, errors.New("no adaptor")
	}
	return adaptor.GetTask(ctx, req)
}
