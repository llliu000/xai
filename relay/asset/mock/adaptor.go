package mock

import (
	"errors"

	"github.com/QuantumNous/new-api/dto"
	"github.com/google/uuid"
)

type Adaptor struct {
}

func (a *Adaptor) ReviewSkip() bool { return false }

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
