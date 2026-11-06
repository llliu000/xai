package asset

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/asset/volc"
	"github.com/QuantumNous/new-api/relay/asset/volc_key"
)

var (
	UpstreamAssetGroupName = "6666-xxxfkafjlasd-123"
	GroupType              = "AIGC"
)

const (
	CategoryVolc    = "volc"
	CategoryVolcKey = "volc_key"
)

type Adaptor interface {
	CreateAssetGroup(req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error)
	ListAssetGroups(req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error)
	CreateAssets(req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error)
	GetAsset(req *dto.GetAssetRequest) (*dto.GetAssetResponse, error)
	ListAssets(req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error)
	CreateVisualValidateSession(req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error)
	GetVisualValidateResult(req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error)
	ReviewSkip() bool
}

func GetAdaptor(ch *model.Channel) (Adaptor, error) {
	setting := ch.GetSetting()
	if setting.CategorySetting == "" {
		setting.CategorySetting = "{}"
	}
	switch setting.Category {
	case CategoryVolc:
		var adaptor volc.Adaptor
		err := json.Unmarshal([]byte(setting.CategorySetting), &adaptor)
		return &adaptor, err
	case CategoryVolcKey:
		var adaptor volc_key.Adaptor
		if err := json.Unmarshal([]byte(setting.CategorySetting), &adaptor); err != nil {
			return nil, err
		}
		if ch.BaseURL != nil {
			adaptor.BaseURL = *ch.BaseURL
		}
		adaptor.ApiKey = ch.Key
		return &adaptor, nil
	}
	return nil, common.NewMessage("Asset library type must be volc or volc_key")
}

func CategoryOptions() []map[string]string {
	return []map[string]string{
		{
			"value": CategoryVolc,
			"label": "AK/SK",
		},
		{
			"value": CategoryVolcKey,
			"label": "API Key",
		},
	}
}
