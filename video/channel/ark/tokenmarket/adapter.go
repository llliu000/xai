package tokenmarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/video/common"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/QuantumNous/new-api/video/utils"
	"github.com/google/uuid"
)

const BaseUrl = "https://model.service-inference.ai"

type Adapter struct{}

func (a *Adapter) Name() string {
	return common.TokenMarket
}

func (a *Adapter) CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	url := fmt.Sprintf("%s/v2/video/generate", BaseUrl)
	apiKey := common.GetRelayContext(ctx).ApiKey
	httpClient := utils.NewHttpClient(url, apiKey)

	var response ResultResponse
	err := httpClient.Post(ctx, req, &response)
	return &dto.ArkCreateTaskResponse{ID: response.Task.Id}, err
}

func (a *Adapter) GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	url := fmt.Sprintf("%s/v2/video/tasks/%s", BaseUrl, req.TaskID)
	apiKey := common.GetRelayContext(ctx).ApiKey
	httpClient := utils.NewHttpClient(url, apiKey)

	var tr ResultResponse
	if err := httpClient.Get(ctx, &tr); err != nil {
		return nil, err
	}

	taskErr, ok := tr.Task.Metadata["error"].(string)
	delete(tr.Task.Metadata, "error")
	if ok && taskErr != "" {
		tr.Task.Metadata["error"] = map[string]any{
			"message": taskErr,
			"code":    "40001",
		}
	}
	responseBody, err := json.Marshal(tr.Task.Metadata)
	if err != nil {
		return nil, err
	}
	var response dto.ArkGetTaskResponse
	err = json.Unmarshal(responseBody, &response)
	return &response, err
}

func (a *Adapter) CreateAssetGroup(ctx context.Context, req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	return &dto.CreateAssetGroupResponse{Id: uuid.NewString()}, nil
}

func (a *Adapter) CreateAssets(ctx context.Context, req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	url := fmt.Sprintf("%s/v2/sd-max/assets", BaseUrl)
	var response BaseAssetResponse[dto.CreateAssetResponse]
	apiKey := common.GetRelayContext(ctx).ApiKey
	httpClient := utils.NewHttpClient(url, apiKey)
	err := httpClient.Post(ctx, req, &response)
	return &response.Data, err
}

func (a *Adapter) GetAsset(ctx context.Context, req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	url := fmt.Sprintf("%s/v2/sd-max/assets/%s", BaseUrl, req.Id)
	var response BaseAssetResponse[dto.GetAssetResponse]
	apiKey := common.GetRelayContext(ctx).ApiKey
	httpClient := utils.NewHttpClient(url, apiKey)
	err := httpClient.Get(ctx, &response)
	return &response.Data, err
}

func (a *Adapter) ListAssets(ctx context.Context, req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error) {
	return &dto.ListAssetsResponse{}, nil
}

func (a *Adapter) ListAssetGroups(ctx context.Context, req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error) {
	return &dto.ListAssetGroupsResponse{}, nil
}

func (*Adapter) CreateVisualValidateSession(ctx context.Context, req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error) {
	return nil, errors.New("channel is not supported")
}

func (*Adapter) GetVisualValidateResult(ctx context.Context, req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error) {
	return nil, errors.New("channel is not supported")
}
