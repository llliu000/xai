package volc

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/video/common"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/QuantumNous/new-api/video/utils"
)

const (
	InnerBaseUrl = "https://ark.cn-beijing.volces.com"
	OuterBaseUrl = "https://ark.ap-southeast.bytepluses.com"
)

type InnerAdapter struct{}

func (a *InnerAdapter) Name() string {
	return common.VolcInner
}

func (a *InnerAdapter) CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks", InnerBaseUrl)
	apiKey := common.GetRelayContext(ctx).ApiKey
	client := utils.NewHttpClient(url, apiKey)
	var response dto.ArkCreateTaskResponse
	err := client.Post(ctx, req, &response)
	return &response, err
}

func (a *InnerAdapter) GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", InnerBaseUrl, req.TaskID)
	apiKey := common.GetRelayContext(ctx).ApiKey
	client := utils.NewHttpClient(url, apiKey)
	var response dto.ArkGetTaskResponse
	err := client.Get(ctx, &response)
	return &response, err
}

type OuterAdapter struct{}

func (a *OuterAdapter) Name() string {
	return common.VolcOuter
}

func (a *OuterAdapter) CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks", OuterBaseUrl)
	apiKey := common.GetRelayContext(ctx).ApiKey
	client := utils.NewHttpClient(url, apiKey)
	var response dto.ArkCreateTaskResponse
	err := client.Post(ctx, req, &response)
	return &response, err
}

func (a *OuterAdapter) GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", OuterBaseUrl, req.TaskID)
	apiKey := common.GetRelayContext(ctx).ApiKey
	client := utils.NewHttpClient(url, apiKey)
	var response dto.ArkGetTaskResponse
	err := client.Get(ctx, &response)
	return &response, err
}
