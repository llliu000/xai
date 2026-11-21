package vidu

import (
	"context"
	"fmt"
	"strconv"

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

	var resTask GetTaskResponse
	if err := httpClient.Get(ctx, &resTask); err != nil {
		return nil, err
	}

	response := dto.ArkGetTaskResponse{
		Status:        dto.ArkTaskStatus(resTask.Status),
		GenerateAudio: &resTask.GenerateAudio,
		OutputFormat:  &resTask.OutputFormat,
		ServiceTier:   &resTask.ServiceTier,
		Resolution:    &resTask.Resolution,
		Draft:         &resTask.Draft,
		Ratio:         &resTask.Ratio,
		Model:         resTask.Model,
		ID:            resTask.Id,
	}
	if resTask.Usage != nil {
		response.Usage.CompletionTokens, _ = strconv.ParseInt(resTask.Usage.CompletionTokens, 10, 64)
		response.Usage.TotalTokens, _ = strconv.ParseInt(resTask.Usage.CompletionTokens, 10, 64)
	}

	if resTask.Error != nil {
		response.Error = &dto.ArkError{Message: resTask.Error.Message, Code: resTask.Error.Code}
	}
	if resTask.Content != nil {
		response.Content.LastFrameURL = resTask.Content.LastFrameUrl
		response.Content.VideoURL = resTask.Content.VideoUrl
	}

	for _, tool := range resTask.Tools {
		response.Tools = append(response.Tools, &dto.ArkTool{Type: tool.Type})
	}

	executionExpiresAfter, _ := strconv.ParseInt(resTask.ExecutionExpiresAfter, 10, 64)
	framesPerSecond, _ := strconv.ParseInt(resTask.FramesPerSecond, 10, 64)
	createdAt, _ := strconv.ParseInt(resTask.CreatedAt, 10, 64)
	updatedAt, _ := strconv.ParseInt(resTask.UpdatedAt, 10, 64)
	duration, _ := strconv.ParseInt(resTask.Duration, 10, 64)
	deed, _ := strconv.ParseInt(resTask.Seed, 10, 64)

	response.ExecutionExpiresAfter = &executionExpiresAfter
	response.FramesPerSecond = &framesPerSecond
	response.CreatedAt = createdAt
	response.UpdatedAt = updatedAt
	response.Duration = &duration
	response.Seed = &deed

	return &response, nil
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
