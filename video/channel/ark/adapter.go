package ark

import (
	"context"

	"github.com/QuantumNous/new-api/video/dto"
)

type Adapter interface {
	CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error)
	GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error)
}

type Asset interface {
	CreateAssetGroup(ctx context.Context, req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error)
	ListAssetGroups(ctx context.Context, req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error)
	CreateAssets(ctx context.Context, req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error)
	GetAsset(ctx context.Context, req *dto.GetAssetRequest) (*dto.GetAssetResponse, error)
	ListAssets(ctx context.Context, req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error)
	CreateVisualValidateSession(ctx context.Context, req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error)
	GetVisualValidateResult(ctx context.Context, req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error)
}
