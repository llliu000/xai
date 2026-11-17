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

func DefaultArkRegistry() *Registry[ark.Adapter] {
	return &Registry[ark.Adapter]{
		mapping: map[string]ark.Adapter{
			common.Mock: &mockArk.Adapter{},
			common.Vidu: &vidu.Adapter{},
		},
	}
}

func DefaultWanRegistry() *Registry[wan.Adapter] {
	return &Registry[wan.Adapter]{
		mapping: map[string]wan.Adapter{
			common.Mock: &mockWan.Adapter{},
		},
	}
}

func DefaultArkAssetRegistry() *Registry[ark.Asset] {
	return &Registry[ark.Asset]{
		mapping: map[string]ark.Asset{
			common.Mock: &mockArk.Adapter{},
		},
	}
}

func (r *Registry[T]) GetAdapter(ctx context.Context) (T, error) {
	channelName := common.GetRelayContext(ctx).ChannelName
	adapter, ok := r.mapping[channelName]
	if !ok {
		return adapter, errors.New("channel not found")
	}
	return adapter, nil
}
