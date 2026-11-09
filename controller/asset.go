package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

const assetAPIVersion = "2024-01-01"

type AssetController struct{}

func (a *AssetController) GetSeedanceChannelOptions(c *gin.Context) {
	options, err := service.GetSeedanceChannelOptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, options)
}

func (a *AssetController) UpdateUserSeedanceChannels(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var req dto.UpdateUserSeedanceChannelsRequest
	if err := common.DecodeJsonWithValidation(c.Request.Body, &req); err != nil ||
		(req.SeedanceInnerChannelID == nil && req.SeedanceOuterChannelID == nil) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !canManageTargetRole(c.GetInt("role"), user.Role) {
		common.ApiErrorT(c, "No permission to update users of same or higher permission level")
		return
	}
	result, err := service.UpdateUserSeedanceChannels(user, &req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, userID, "user.update", map[string]any{
		"seedance_inner_channel_id": result.SeedanceInnerChannelID,
		"seedance_outer_channel_id": result.SeedanceOuterChannelID,
	})
	common.ApiSuccessT(c, "Settings updated", result)
}

func (a *AssetController) Action(c *gin.Context) {
	action := c.Query("Action")
	version := c.Query("Version")
	if action == "" || version != assetAPIVersion {
		c.String(http.StatusNotFound, "Not Found")
		return
	}
	var err error
	var result dto.AssetBaseResult
	result.ResponseMetadata.RequestId = c.GetString(common.RequestIdKey)
	region := model.NormalizeAssetRegion(c.Query("Region"))
	result.ResponseMetadata.Version = version
	result.ResponseMetadata.Region = region
	result.ResponseMetadata.Action = action
	result.ResponseMetadata.Service = "ark"
	if region != model.AssetRegionDomestic && region != model.AssetRegionOverseas {
		c.JSON(http.StatusBadRequest, map[string]any{
			"Error": "Region must be cn-beijing or ap-southeast-1",
			"Code":  "InvalidParameter",
		})
		return
	}

	switch action {
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
	case "GetAssetGroup":
		result.Result, err = a.GetAssetGroup(c)
	case "UpdateAssetGroup":
		result.Result, err = a.UpdateAssetGroup(c)
	case "UpdateAsset":
		result.Result, err = a.UpdateAsset(c)
	case "DeleteAsset":
		result.Result, err = a.DeleteAsset(c)
	case "DeleteAssetGroup":
		result.Result, err = a.DeleteAssetGroup(c)
	case "CreateVisualValidateSession":
		result.Result, err = a.CreateVisualValidateSession(c)
	case "GetVisualValidateResult":
		result.Result, err = a.GetVisualValidateResult(c)
	default:
		c.String(http.StatusNotFound, "Not Found")
		return
	}
	if err != nil {
		if errs, ok := errors.AsType[validator.ValidationErrors](err); ok {
			err = errors.New(TranslateValidationErrors(errs))
		}
		result.Result = map[string]any{"Code": "unknown", "Error": err.Error()}
	}
	c.JSON(http.StatusOK, result)
}

func (a *AssetController) CreateAssetGroup(c *gin.Context) (*dto.CreateAssetGroupResponse, error) {
	var req dto.CreateAssetGroupRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.CreateAssetGroup(c, &req)
}

func (a *AssetController) CreateAsset(c *gin.Context) (*dto.CreateAssetResponse, error) {
	var req dto.CreateAssetRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.CreateAsset(c, &req)
}

func (a *AssetController) ListAssetGroups(c *gin.Context) (*dto.ListAssetGroupsResponse, error) {
	var req dto.ListAssetGroupsRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.ListAssetGroups(c, &req)
}

func (a *AssetController) ListAssets(c *gin.Context) (*dto.ListAssetsResponse, error) {
	var req dto.ListAssetsRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.ListAssets(c, &req)
}

func (a *AssetController) GetAsset(c *gin.Context) (*dto.GetAssetResponse, error) {
	var req dto.GetAssetRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.GetAsset(c, &req)
}

func (a *AssetController) GetAssetGroup(c *gin.Context) (*dto.GetAssetGroupResponse, error) {
	var req dto.GetAssetGroupRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.GetAssetGroup(c, &req)
}

func (a *AssetController) UpdateAssetGroup(c *gin.Context) (*dto.UpdateAssetGroupResponse, error) {
	var req dto.UpdateAssetGroupRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.UpdateAssetGroup(c, &req)
}

func (a *AssetController) UpdateAsset(c *gin.Context) (*dto.UpdateAssetResponse, error) {
	var req dto.UpdateAssetRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.UpdateAsset(c, &req)
}

func (a *AssetController) DeleteAsset(c *gin.Context) (*dto.DeleteAssetResponse, error) {
	var req dto.DeleteAssetRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.DeleteAsset(c, &req)
}

func (a *AssetController) DeleteAssetGroup(c *gin.Context) (*dto.DeleteAssetGroupResponse, error) {
	var req dto.DeleteAssetGroupRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.DeleteAssetGroup(c, &req)
}

func (a *AssetController) CreateVisualValidateSession(c *gin.Context) (*dto.CreateVisualValidateSessionResponse, error) {
	var req dto.CreateVisualValidateSessionRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.CreateVisualValidateSession(c, &req)
}

func (a *AssetController) GetVisualValidateResult(c *gin.Context) (*dto.GetVisualValidateResultResponse, error) {
	var req dto.GetVisualValidateResultRequest
	if err := c.BindJSON(&req); nil != err {
		return nil, err
	}
	return service.GetVisualValidateResult(c, &req)
}

func (a *AssetController) ManualAsset(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.ApiErrorT(c, "Invalid channel ID")
		return
	}
	task, err := service.SyncAssetsToChannel(c.Request.Context(), channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"channel_id": channelID, "task_id": task.TaskID, "status": task.Status})
}

func (a *AssetController) GetAssetSyncProgress(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.ApiErrorT(c, "Invalid channel ID")
		return
	}
	progress, err := service.GetChannelAssetSyncProgress(c.Request.Context(), channelID, c.Query("task_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, progress)
}

func TranslateValidationErrors(errs validator.ValidationErrors) string {
	messages := make([]string, 0, len(errs))
	for _, e := range errs {
		field := e.Field()
		switch e.Tag() {
		case "required":
			messages = append(messages,
				fmt.Sprintf("%s is required", field))
		case "oneof":
			values := strings.ReplaceAll(e.Param(), " ", ", ")
			messages = append(messages,
				fmt.Sprintf("%s must be one of [%s]", field, values))
		default:
			messages = append(messages, e.Error())
		}
	}

	return strings.Join(messages, "; ")
}
