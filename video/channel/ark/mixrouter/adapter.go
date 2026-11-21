package mixrouter

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

const BaseUrl = "https://api.mixrouter.io"

type Adapter struct{}

func (a *Adapter) Name() string {
	return "mixrouter"
}

func (a *Adapter) CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	request := CreateTaskRequest{Model: req.Model, Asset: true, Metadata: req}
	for i := range req.Content {
		if req.Content[i].Type == "text" {
			request.Prompt = *req.Content[i].Text
			break
		}
	}

	url := fmt.Sprintf("%s/v1/video/generations", BaseUrl)
	apiKey := common.GetRelayContext(ctx).ApiKey
	httpClient := utils.NewHttpClient(url, apiKey)

	var response CreateTaskResponse
	err := httpClient.Post(ctx, request, &response)
	return &dto.ArkCreateTaskResponse{ID: response.TaskID}, err
}

func (a *Adapter) GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	url := fmt.Sprintf("%s/v1/video/generations/%s", BaseUrl, req.TaskID)
	apiKey := common.GetRelayContext(ctx).ApiKey
	httpClient := utils.NewHttpClient(url, apiKey)

	var tr GetTaskResponse
	if err := httpClient.Get(ctx, &tr); err != nil {
		return nil, err
	}
	if tr.Code != "success" {
		return nil, errors.New(tr.Message)
	}
	var response dto.ArkGetTaskResponse
	err := json.Unmarshal(tr.Data.Data, &response)
	return &response, err
}

func (a *Adapter) CreateAssetGroup(ctx context.Context, req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	return &dto.CreateAssetGroupResponse{Id: uuid.NewString()}, nil
}

func (a *Adapter) CreateAssets(ctx context.Context, req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	return &dto.CreateAssetResponse{Id: req.URL}, nil
}

func (a *Adapter) GetAsset(ctx context.Context, req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	return &dto.GetAssetResponse{Status: "Active"}, nil
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
