package vidu

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/video/common"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/QuantumNous/new-api/video/utils"
)

const BaseUrl = "https://api.mixrouter.io"

type Adapter struct {
}

func (a *Adapter) Name() string {
	return "vidu"
}

func (a *Adapter) CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	request := CreateTaskRequest{ArkCreateTaskRequest: req, Moderation: "disabled"}
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks", BaseUrl)
	apiKey := common.GetRelayContext(ctx).ApiKey
	httpClient := utils.NewHttpClient(url, apiKey)

	var response dto.ArkCreateTaskResponse
	err := httpClient.Post(ctx, request, &response)
	return &response, err
}

func (a *Adapter) GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", BaseUrl, req.TaskID)
	apiKey := common.GetRelayContext(ctx).ApiKey
	httpClient := utils.NewHttpClient(url, apiKey)

	var response dto.ArkGetTaskResponse
	err := httpClient.Get(ctx, &response)
	return &response, err
}

func (a *Adapter) CreateAssetGroup(ctx context.Context, req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	var result AssetBaseResult[dto.CreateAssetGroupResponse]
	err := a.doCall(ctx, "CreateAssetGroup", req, &result)
	return &result.Result, err
}

func (a *Adapter) CreateAssets(ctx context.Context, req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	request := CreateAsset{CreateAssetRequest: req, Moderation: "disabled"}
	var result AssetBaseResult[dto.CreateAssetResponse]
	err := a.doCall(ctx, "CreateAsset", request, &result)
	return &result.Result, err
}

func (a *Adapter) GetAsset(ctx context.Context, req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	var result AssetBaseResult[dto.GetAssetResponse]
	err := a.doCall(ctx, "GetAsset", req, &result)
	return &result.Result, err
}

func (a *Adapter) ListAssets(ctx context.Context, req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error) {
	var result AssetBaseResult[dto.ListAssetsResponse]
	err := a.doCall(ctx, "ListAssets", req, &result)
	return &result.Result, err
}

func (a *Adapter) ListAssetGroups(ctx context.Context, req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error) {
	var result AssetBaseResult[dto.ListAssetGroupsResponse]
	err := a.doCall(ctx, "ListAssetGroups", req, &result)
	return &result.Result, err
}

func (a *Adapter) CreateVisualValidateSession(ctx context.Context, req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error) {
	var result AssetBaseResult[dto.CreateVisualValidateSessionResponse]
	err := a.doCall(ctx, "CreateVisualValidateSession", req, &result)
	return &result.Result, err
}

func (a *Adapter) GetVisualValidateResult(ctx context.Context, req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error) {
	var result AssetBaseResult[dto.GetVisualValidateResultResponse]
	err := a.doCall(ctx, "GetVisualValidateResult", req, &result)
	return &result.Result, err
}

func (a *Adapter) doCall(ctx context.Context, action string, request, response any) error {
	url := fmt.Sprintf("%s?Action=%s&Version=2024-01-01", BaseUrl, action)
	apiKey := common.GetRelayContext(ctx).ApiKey
	httpClient := utils.NewHttpClient(url, apiKey)
	return httpClient.Post(ctx, request, response)
}
