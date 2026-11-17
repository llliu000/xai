package service

import (
	"github.com/QuantumNous/new-api/video/channel"
	"github.com/QuantumNous/new-api/video/channel/ark"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/gin-gonic/gin"
)

type Ark struct {
	registry *channel.Registry[ark.Adaptor]
	asset    *channel.Registry[ark.Asset]
}

func NewArk() *Ark {
	return &Ark{
		registry: channel.DefaultArkRegistry(),
		asset:    channel.DefaultArkAssetRegistry(),
	}
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

func (a *Ark) CreateAssetGroup(ctx *gin.Context, req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	adaptor, err := a.asset.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.CreateAssetGroup(ctx, req)
}

func (a *Ark) ListAssetGroups(ctx *gin.Context, req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error) {
	adaptor, err := a.asset.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.ListAssetGroups(ctx, req)
}

func (a *Ark) CreateAsset(ctx *gin.Context, req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	adaptor, err := a.asset.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.CreateAssets(ctx, req)
}

func (a *Ark) ListAssets(ctx *gin.Context, req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error) {
	adaptor, err := a.asset.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.ListAssets(ctx, req)
}

func (a *Ark) GetAsset(ctx *gin.Context, req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	adaptor, err := a.asset.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.GetAsset(ctx, req)
}

func (a *Ark) CreateVisualValidateSession(ctx *gin.Context, req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error) {
	adaptor, err := a.asset.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.CreateVisualValidateSession(ctx, req)
}

func (a *Ark) GetVisualValidateResult(ctx *gin.Context, req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error) {
	adaptor, err := a.asset.GetAdaptor(ctx)
	if err != nil {
		return nil, err
	}
	return adaptor.GetVisualValidateResult(ctx, req)
}
