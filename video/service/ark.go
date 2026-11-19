package service

import (
	"log"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/video/channel"
	"github.com/QuantumNous/new-api/video/channel/ark"
	"github.com/QuantumNous/new-api/video/dto"
	"github.com/QuantumNous/new-api/video/enhance"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type Ark struct {
	registry *channel.Registry[ark.Adapter]
	asset    *channel.Registry[ark.Asset]
}

func NewArk() *Ark {
	return &Ark{
		registry: channel.DefaultArkRegistry(),
		asset:    channel.DefaultArkAssetRegistry(),
	}
}

func (a *Ark) CreateTask(ctx *gin.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	adapter, err := a.registry.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.CreateTask(ctx, req)
}

func (a *Ark) GetTask(ctx *gin.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	adapter, err := a.registry.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.GetTask(ctx, req)
}

func (a *Ark) CreateEnhanceTask(ctx *gin.Context, req dto.ArkCreateTaskRequest) (*dto.ArkCreateTaskResponse, error) {
	enhanceResolution := req.Resolution
	if enhanceResolution == nil {
		enhanceResolution = new("720p")
	}
	resolution, err := enhance.GetConfig(*enhanceResolution)
	if err != nil {
		return nil, err
	}
	req.Resolution = &resolution.Origin
	task, err := a.CreateTask(ctx, req)
	if err != nil {
		return nil, err
	}
	var videoEnhance = model.VideoEnhance{
		VideoTaskParams:   datatypes.NewJSONType(req),
		EnhanceResolution: *enhanceResolution,
		VideoResolution:   resolution.Origin,
		VideoTaskID:       task.ID,
	}
	// 保存超分原始数据
	if err = videoEnhance.Create(); err != nil {
		log.Println(err)
	}
	return task, nil
}

func (a *Ark) GetEnhanceTask(ctx *gin.Context, req dto.ArkGetTaskRequest) (*dto.ArkGetTaskResponse, error) {
	task, err := a.GetTask(ctx, req)
	if err != nil {
		return nil, err
	}
	var videoEnhance model.VideoEnhance
	if err = videoEnhance.FindByVideoTaskId(task.ID); err != nil {
		return nil, err
	}
	// 超分出现问题直接返回超分信息
	if videoEnhance.ID == 0 {
		return task, nil
	}
	// 响应参数要操持超分目标分辨率
	task.Resolution = &videoEnhance.EnhanceResolution
	// 超过重试次数直接返回原始视频生成任务信息
	if videoEnhance.RetryCount >= 3 {
		return task, nil
	}

	// 缓存进行中信息供超分的时候直接返回
	if task.Status == dto.ArkTaskStatusRunning {
		videoEnhance.VideoRunningResult = new(datatypes.NewJSONType(*task))
		if err = videoEnhance.UpdateVideoRunningResult(); err != nil {
			return nil, err
		}
	}
	if videoEnhance.VideoRunningResult == nil && task.Status == dto.ArkTaskStatusSucceeded {
		runningResult := dto.ArkGetTaskResponse{
			Status:        dto.ArkTaskStatusRunning,
			GenerateAudio: task.GenerateAudio,
			OutputFormat:  task.OutputFormat,
			ServiceTier:   task.ServiceTier,
			Resolution:    task.Resolution,
			CreatedAt:     task.CreatedAt,
			UpdatedAt:     task.UpdatedAt,
			Priority:      task.Priority,
			Model:         task.Model,
			Ratio:         task.Ratio,
			Draft:         task.Draft,
			ID:            task.ID,
		}
		videoEnhance.VideoRunningResult = new(datatypes.NewJSONType(runningResult))
		if err = videoEnhance.UpdateVideoRunningResult(); err != nil {
			return nil, err
		}
	}

	if task.Status != dto.ArkTaskStatusSucceeded {
		return task, nil
	}
	enhanceAdapter, err := enhance.NewRegistry().GetAdapter("")
	if err != nil {
		return nil, err
	}
	// 超分任务进行中直接返回生成任务进行中
	if videoEnhance.EnhanceTaskID == nil {
		request := enhance.CreateTaskRequest{
			VideoURL:    task.Content.VideoURL,
			Resolution:  task.Resolution,
			ClientToken: new(task.ID), // 火山任务幂等控制，不会重复创建任务，会正常返回任务ID
		}
		createTask, err := enhanceAdapter.CreateTask(ctx, request)
		if err != nil {
			_ = videoEnhance.UpdateFailed(err.Error())
			return nil, err
		}
		videoEnhance.EnhanceTaskID = &createTask.ID
		if err = videoEnhance.UpdateEnhanceTaskID(); err != nil {
			return nil, err
		}
		return new(videoEnhance.VideoRunningResult.Data()), nil
	}

	response, err := enhanceAdapter.GetTask(ctx, *videoEnhance.EnhanceTaskID)
	if err != nil {
		_ = videoEnhance.UpdateFailed(err.Error())
		return nil, err
	}
	switch response.Status {
	case enhance.TaskStatusFailed: // 超分失败直接返回原始任务结果
		return task, nil
	case enhance.TaskStatusSucceeded: // 成功直接替换生成任务url地址返回
		resolution, err := enhance.GetConfig(videoEnhance.EnhanceResolution)
		if err != nil {
			return nil, err
		}
		task.Usage.CompletionTokens = int64(float64(task.Usage.CompletionTokens) * resolution.Ratio)
		task.Usage.TotalTokens = int64(float64(task.Usage.TotalTokens) * resolution.Ratio)
		task.Content.VideoURL = response.VideoURL
		return task, nil
	default: // 队列或者进行中直接返回原始任务的生成中状态
		return new(videoEnhance.VideoRunningResult.Data()), nil
	}
}

func (a *Ark) CreateAssetGroup(ctx *gin.Context, req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.CreateAssetGroup(ctx, req)
}

func (a *Ark) ListAssetGroups(ctx *gin.Context, req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.ListAssetGroups(ctx, req)
}

func (a *Ark) CreateAsset(ctx *gin.Context, req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.CreateAssets(ctx, req)
}

func (a *Ark) ListAssets(ctx *gin.Context, req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.ListAssets(ctx, req)
}

func (a *Ark) GetAsset(ctx *gin.Context, req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.GetAsset(ctx, req)
}

func (a *Ark) CreateVisualValidateSession(ctx *gin.Context, req *dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.CreateVisualValidateSession(ctx, req)
}

func (a *Ark) GetVisualValidateResult(ctx *gin.Context, req *dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error) {
	adapter, err := a.asset.GetAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return adapter.GetVisualValidateResult(ctx, req)
}
