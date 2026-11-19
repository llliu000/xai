package handler

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/QuantumNous/new-api/video/service"
	"github.com/gin-gonic/gin"
)

type Ark struct {
	arkService *service.Ark
}

func NewArk() *Ark {
	return &Ark{arkService: service.NewArk()}
}

func (a *Ark) GetTask(ctx *gin.Context) {
	var req dto.ArkGetTaskRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := a.arkService.GetTask(ctx, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (a *Ark) CreateTask(ctx *gin.Context) {
	var req dto.ArkCreateTaskRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := a.arkService.CreateTask(ctx, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (a *Ark) GetEnhanceTask(ctx *gin.Context) {
	var req dto.ArkGetTaskRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := a.arkService.GetEnhanceTask(ctx, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (a *Ark) CreateEnhanceTask(ctx *gin.Context) {
	var req dto.ArkCreateTaskRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := a.arkService.CreateEnhanceTask(ctx, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (a *Ark) Action(c *gin.Context) {
	var (
		result dto.AssetBaseResult
		err    error
	)
	result.ResponseMetadata.RequestId = c.GetString(common.RequestIdKey)
	result.ResponseMetadata.Version = c.Query("Version")
	result.ResponseMetadata.Action = c.Query("Action")
	result.ResponseMetadata.Region = "cn-beijing"
	result.ResponseMetadata.Service = "ark"

	switch c.Query("Action") {
	case "CreateAssetGroup":
		result.Result, err = a.CreateAssetGroup(c)
	case "CreateAsset":
		result.Result, err = a.CreateAsset(c)
	case "ListAssetGroups":
		result.Result, err = a.ListAssetGroups(c)
	case "ListAssets":
		result.Result, err = a.ListAssets(c)
	case "GetAsset":
		result.Result, err = a.GetAsset(c)
	case "CreateVisualValidateSession":
		result.Result, err = a.CreateVisualValidateSession(c)
	case "GetVisualValidateResult":
		result.Result, err = a.GetVisualValidateResult(c)
	default:
		c.String(http.StatusNotFound, "Not Found")
		return
	}
	if err != nil {
		result.Result = map[string]any{"Code": "unknown", "Error": err.Error()}
	}
	c.JSON(http.StatusOK, result)
}

func (a *Ark) CreateAssetGroup(c *gin.Context) (*dto.CreateAssetGroupResponse, error) {
	var req dto.CreateAssetGroupRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return a.arkService.CreateAssetGroup(c, &req)
}

func (a *Ark) CreateAsset(c *gin.Context) (*dto.CreateAssetResponse, error) {
	var req dto.CreateAssetRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return a.arkService.CreateAsset(c, &req)
}

func (a *Ark) ListAssetGroups(c *gin.Context) (*dto.ListAssetGroupsResponse, error) {
	var req dto.ListAssetGroupsRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return a.arkService.ListAssetGroups(c, &req)
}

func (a *Ark) ListAssets(c *gin.Context) (*dto.ListAssetsResponse, error) {
	var req dto.ListAssetsRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return a.arkService.ListAssets(c, &req)
}

func (a *Ark) GetAsset(c *gin.Context) (*dto.GetAssetResponse, error) {
	var req dto.GetAssetRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return a.arkService.GetAsset(c, &req)
}

func (a *Ark) CreateVisualValidateSession(c *gin.Context) (*dto.CreateVisualValidateSessionResponse, error) {
	var req dto.CreateVisualValidateSessionRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return a.arkService.CreateVisualValidateSession(c, &req)
}

func (a *Ark) GetVisualValidateResult(c *gin.Context) (*dto.GetVisualValidateResultResponse, error) {
	var req dto.GetVisualValidateResultRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return a.arkService.GetVisualValidateResult(c, &req)
}
