package mock

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/video/dto"
	"github.com/google/uuid"
)

type Adapter struct{}

func (a *Adapter) Name() string {
	return "mock"
}

func (a *Adapter) CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	return &dto.ArkCreateTaskResponse{ID: uuid.NewString()}, nil
}

func (a *Adapter) GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
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
