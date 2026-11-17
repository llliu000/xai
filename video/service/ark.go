package service

import (
	"github.com/QuantumNous/new-api/video/channel"
	"github.com/QuantumNous/new-api/video/channel/ark"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/gin-gonic/gin"
)

type Ark struct {
	registry *channel.Registry[ark.Adapter]
	asset    *channel.Registry[ark.Asset]
}

func NewArk() *Ark {
	return &Ark{
		registry: channel.DefaultArkRegistry(),
		asset:    channel.DefaultArkAssetRegistry(),
	}
}

func (a *Ark) CreateTask(ctx *gin.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	adapter, err := a.registry.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.CreateTask(ctx, req)
}

func (a *Ark) GetTask(ctx *gin.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	adapter, err := a.registry.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.GetTask(ctx, req)
}

func (a *Ark) CreateAssetGroup(ctx *gin.Context, req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.CreateAssetGroup(ctx, req)
}

func (a *Ark) ListAssetGroups(ctx *gin.Context, req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.ListAssetGroups(ctx, req)
}

func (a *Ark) CreateAsset(ctx *gin.Context, req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.CreateAssets(ctx, req)
}

func (a *Ark) ListAssets(ctx *gin.Context, req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.ListAssets(ctx, req)
}

func (a *Ark) GetAsset(ctx *gin.Context, req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.GetAsset(ctx, req)
}

func (a *Ark) CreateVisualValidateSession(ctx *gin.Context, req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.CreateVisualValidateSession(ctx, req)
}

func (a *Ark) GetVisualValidateResult(ctx *gin.Context, req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.GetVisualValidateResult(ctx, req)
}
