package mock

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/video/dto"
	"github.com/google/uuid"
)

type Adaptor struct{}

func (a *Adaptor) Name() string {
	return "mock"
}

func (a *Adaptor) CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	return &dto.ArkCreateTaskResponse{ID: uuid.NewString()}, nil
}

func (a *Adaptor) GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	return &dto.ArkGetTaskResponse{
		ID:     req.TaskID,
		Model:  "mock",
		Status: dto.ArkTaskStatusSucceeded,
		Content: dto.ArkOutput{
			VideoURL:     "https://mock.com",
			LastFrameURL: "https://mock.com",
		},
		Usage: dto.ArkUsage{
			CompletionTokens: 10000,
			TotalTokens:      10000,
		},
		CreatedAt:     time.Now().Unix(),
		UpdatedAt:     time.Now().Unix(),
		Resolution:    new("1080p"),
		Ratio:         new("16:9"),
		OutputFormat:  new("mp4"),
		Duration:      new(int64(5)),
		GenerateAudio: new(true),
		Draft:         new(false),
		Seed:          new(int64),
	}, nil
}

func (a *Adaptor) CreateAssetGroup(req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	return &dto.CreateAssetGroupResponse{Id: uuid.NewString()}, nil
}

func (a *Adaptor) CreateAssets(req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	return &dto.CreateAssetResponse{Id: req.URL}, nil
}

func (a *Adaptor) GetAsset(req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	return &dto.GetAssetResponse{Status: "Active"}, nil
}

func (a *Adaptor) ListAssets(req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error) {
	return &dto.ListAssetsResponse{}, nil
}

func (a *Adaptor) ListAssetGroups(req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error) {
	return &dto.ListAssetGroupsResponse{}, nil
}

func (*Adaptor) CreateVisualValidateSession(*dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error) {
	return nil, errors.New("channel is not supported")
}

func (*Adaptor) GetVisualValidateResult(*dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error) {
	return nil, errors.New("channel is not supported")
}
