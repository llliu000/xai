package service

import (
	"errors"

	"github.com/QuantumNous/new-api/video/channel/wan"
	"github.com/QuantumNous/new-api/video/common"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/gin-gonic/gin"
)

type Wan struct {
}

func (w *Wan) CreateTask(ctx *gin.Context, req dto.WanCreateTaskRequest) (*dto.WanCreateTaskResponse, error) {
	channelName := common.GetRelayContext(ctx).ChannelName
	adaptor, ok := wan.Mapping[channelName]
	if !ok {
		return nil, errors.New("no adaptor")
	}
	return adaptor.CreateTask(ctx, req)
}

func (w *Wan) GetTask(ctx *gin.Context, req dto.WanGetTaskRequest) (*dto.WanGetTaskResponse, error) {
	channelName := common.GetRelayContext(ctx).ChannelName
	adaptor, ok := wan.Mapping[channelName]
	if !ok {
		return nil, errors.New("no adaptor")
	}
	return adaptor.GetTask(ctx, req)
}
