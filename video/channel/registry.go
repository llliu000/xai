package channel

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/video/channel/ark"
	mockArk "github.com/QuantumNous/new-api/video/channel/ark/mock"
	"github.com/QuantumNous/new-api/video/channel/ark/vidu"
	"github.com/QuantumNous/new-api/video/channel/wan"
	mockWan "github.com/QuantumNous/new-api/video/channel/wan/mock"
	"github.com/QuantumNous/new-api/video/common"
)

type Registry[T any] struct {
	mapping map[string]T
}

func DefaultArkRegistry() *Registry[ark.Adaptor] {
	return &Registry[ark.Adaptor]{
		mapping: map[string]ark.Adaptor{
			common.Mock: &mockArk.Adaptor{},
			common.Vidu: &vidu.Adaptor{},
		},
	}
}

func DefaultWanRegistry() *Registry[wan.Adaptor] {
	return &Registry[wan.Adaptor]{
		mapping: map[string]wan.Adaptor{
			common.Mock: &mockWan.Adaptor{},
		},
	}
}

func (r *Registry[T]) GetAdaptor(ctx context.Context) (T, error) {
	channelName := common.GetRelayContext(ctx).ChannelName
	adaptor, ok := r.mapping[channelName]
	if !ok {
		return adaptor, errors.New("channel not found")
	}
	return adaptor, nil
}
