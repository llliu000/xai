package ark

import (
	"context"

	"github.com/QuantumNous/new-api/video/dto"
)

type Adaptor interface {
	CreateTask(ctx context.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error)
	GetTask(ctx context.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error)
}

type Asset interface {
	CreateAssetGroup(req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error)
	ListAssetGroups(req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error)
	CreateAssets(req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error)
	GetAsset(req *dto.GetAssetRequest) (*dto.GetAssetResponse, error)
	ListAssets(req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error)
	CreateVisualValidateSession(req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error)
	GetVisualValidateResult(req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error)
}
