package volc

import (
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
	"github.com/volcengine/volcengine-go-sdk/volcengine/session"
	"github.com/volcengine/volcengine-go-sdk/volcengine/universal"
)

type Adaptor struct {
	HTTPClient  *http.Client `json:"-"`
	BaseUrl     string       `json:"base_url"`
	AccessID    string       `json:"access_id"`
	SecretKey   string       `json:"secret_key"`
	ProjectName string       `json:"project_name"`
	Region      string       `json:"region"`
	Moderation  string       `json:"moderation"`
}

func (a *Adaptor) ReviewSkip() bool {
	return a.Moderation == "Skip"
}

func (a *Adaptor) CreateAssetGroup(req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	var result dto.CreateAssetGroupResponse
	req.ProjectName = &a.ProjectName
	err := a.doCall("CreateAssetGroup", req, &result)
	return &result, err
}

func (a *Adaptor) ListAssetGroups(req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error) {
	var result dto.ListAssetGroupsResponse
	req.ProjectName = &a.ProjectName
	err := a.doCall("ListAssetGroups", req, &result)
	return &result, err
}

func (a *Adaptor) CreateAssets(req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	var result dto.CreateAssetResponse
	req.ProjectName = &a.ProjectName
	err := a.doCall("CreateAsset", req, &result)
	return &result, err
}

func (a *Adaptor) GetAsset(req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	var result dto.GetAssetResponse
	req.ProjectName = &a.ProjectName
	err := a.doCall("GetAsset", req, &result)
	return &result, err
}

func (a *Adaptor) ListAssets(req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error) {
	var result dto.ListAssetsResponse
	req.ProjectName = &a.ProjectName
	err := a.doCall("ListAssets", req, &result)
	return &result, err
}

func (a *Adaptor) CreateVisualValidateSession(req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error) {
	var result dto.CreateVisualValidateSessionResponse
	req.ProjectName = &a.ProjectName
	err := a.doCall("CreateVisualValidateSession", req, &result)
	return &result, err
}

func (a *Adaptor) GetVisualValidateResult(req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error) {
	var result dto.GetVisualValidateResultResponse
	req.ProjectName = &a.ProjectName
	err := a.doCall("GetVisualValidateResult", req, &result)
	return &result, err
}

func (a *Adaptor) doCall(action string, request, response any) error {
	config := volcengine.NewConfig().WithAkSk(a.AccessID, a.SecretKey).
		WithEndpoint(a.BaseUrl).WithRegion(a.Region).WithMaxRetries(0)
	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	config.WithHTTPClient(client)
	sess, err := session.NewSession(config)
	if err != nil {
		return err
	}
	marshal, err := common.Marshal(request)
	if err != nil {
		return err
	}
	params := map[string]any{}
	if err = common.Unmarshal(marshal, &params); nil != err {
		return err
	}
	resp, err := universal.New(sess).DoCall(
		universal.RequestUniversal{
			ServiceName: "ark",
			Action:      action,
			Version:     "2024-01-01",
			HttpMethod:  universal.POST,
			ContentType: universal.ApplicationJSON,
		},
		&params,
	)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("asset upstream returned no response")
	}
	result, ok := (*resp)["Result"]
	if !ok || result == nil {
		return errors.New("asset upstream returned no result")
	}
	respData, err := common.Marshal(result)
	if err != nil {
		return err
	}
	err = common.Unmarshal(respData, response)
	return err
}
