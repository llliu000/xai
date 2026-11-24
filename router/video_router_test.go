package router

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel/task/mediakit"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpscaleTasksRespectOwnershipSettlementAndResultExpiry(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Task{}, &model.UsageRecord{}, &model.RelayIdempotency{}, &model.Log{}, &model.SystemTask{}))
	priorPrices := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"`+constant.MediaKitModel+`":0.02}`))
	priorFactory, priorAddress := service.GetTaskAdaptorFunc, system_setting.TaskPublicAddress
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(platform) }
	system_setting.TaskPublicAddress = "https://media.example.org"
	t.Cleanup(func() {
		service.GetTaskAdaptorFunc, system_setting.TaskPublicAddress = priorFactory, priorAddress
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(priorPrices))
	})
	const ownerKey, foreignKey = "upscaleownerkey", "upscaleforeignkey"
	for _, account := range []struct {
		id  int
		key string
	}{{91, ownerKey}, {92, foreignKey}} {
		require.NoError(t, model.DB.Create(&model.User{Id: account.id, Username: account.key, AffCode: account.key, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 20000}).Error)
		require.NoError(t, (&model.Token{Id: account.id, UserId: account.id, Key: account.key, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 20000, ModelLimitsEnabled: true, ModelLimits: constant.MediaKitModel}).Insert())
	}
	provider := newFakeUpstream(t)
	provider.respond = func(w http.ResponseWriter, _ int, request capturedRequest) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"task_id":"amk-failed","status":"failed"}`)
	}
	service.InitHttpClient()
	channel := &model.Channel{Type: constant.ChannelTypeCustom, Status: common.ChannelStatusEnabled, Key: "mediakit-private-key", Name: "MediaKit", BaseURL: new(provider.URL), Models: constant.MediaKitModel, Group: "default"}
	require.NoError(t, channel.Insert())
	engine := gin.New()
	engine.Use(middleware.RequestId(), middleware.BodyStorageCleanup())
	SetVideoRouter(engine)
	SetTaskRouter(engine)
	call := func(method, path, key, idempotency, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer sk-"+key)
		if method == http.MethodPost {
			request.Header.Set("Content-Type", "application/json")
			if idempotency != "" {
				request.Header.Set("Idempotency-Key", idempotency)
			}
		}
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, request)
		return res
	}
	res := call(http.MethodGet, "/v1/upscale/models", ownerKey, "", "")
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	assert.Contains(t, res.Body.String(), `"status":"available"`)
	assert.Contains(t, res.Body.String(), `"unit_price":"0.02000000"`)
	const createPath = constant.UpscaleTaskPath
	const unsafeBody = `{"model":"` + constant.MediaKitModel + `","media_type":"video","input":{"url":"https://127.0.0.1/source.mp4"},"target":{"scale":2},"output":{"format":"mp4"}}`
	res = call(http.MethodPost, createPath, ownerKey, "", unsafeBody)
	assert.Equal(t, http.StatusBadRequest, res.Code, res.Body.String())
	res = call(http.MethodPost, createPath, ownerKey, "reject-private-media", unsafeBody)
	assert.Equal(t, http.StatusUnprocessableEntity, res.Code, res.Body.String())
	require.Empty(t, provider.captured())

	ctx := context.Background()
	const quota = 10000
	task := &model.Task{TaskID: "task_upscale_failed", Platform: constant.MediaKitPlatform, UserId: 91, Group: "default", ChannelId: channel.Id,
		Quota: quota, Status: model.TaskStatusQueued, Action: constant.TaskActionUpscaleVideo, Properties: model.Properties{OriginModelName: constant.MediaKitModel},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "amk-failed", TokenId: 91, BillingContext: &model.TaskBillingContext{PerCallBilling: true},
			Execution: &model.TaskExecutionSnapshot{RequestPath: createPath, RequestID: "request_upscale_refund"}}}
	require.NoError(t, task.Insert())
	usage := model.UsageRecord{AccountID: 91, TokenID: 91, RequestID: "request_upscale_refund", TaskID: &task.TaskID, RecordID: "bill_request_upscale_refund", Version: 1,
		Model: constant.MediaKitModel, Capability: "upscale_video", Status: "pending", RequestStatus: "running", Currency: "USD",
		Metadata: model.JSONValue(`{}`), Quantities: model.JSONValue(`[]`), PriceVersion: "test"}
	require.NoError(t, model.DB.Create(&usage).Error)
	require.NoError(t, model.RecordTaskUsageCharge(ctx, task, model.TaskUsageCharge{Quota: quota, QuotaPerUSD: common.QuotaPerUnit, PriceVersion: "test"}))
	res = call(http.MethodGet, createPath+"/"+task.TaskID+"/result", ownerKey, "", "")
	assert.Equal(t, http.StatusConflict, res.Code, res.Body.String())
	res = call(http.MethodGet, createPath+"/"+task.TaskID, foreignKey, "", "")
	assert.Equal(t, http.StatusNotFound, res.Code, res.Body.String())
	require.NoError(t, service.UpdateVideoTasks(ctx, task.Platform, map[int][]string{channel.Id: {task.GetUpstreamTaskID()}}, map[string]*model.Task{task.GetUpstreamTaskID(): task}))
	res = call(http.MethodGet, createPath+"/"+task.TaskID, ownerKey, "", "")
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	assert.Contains(t, res.Body.String(), `"status":"failed"`)
	assert.Contains(t, res.Body.String(), `"status":"refunded"`)
	seen := provider.captured()
	require.Len(t, seen, 1)
	assert.Equal(t, "/api/v1/tasks/amk-failed", seen[0].URL)
	assert.Equal(t, "Bearer mediakit-private-key", seen[0].Header.Get("Authorization"))

	success := *task
	success.ID, success.TaskID, success.Status = 0, "task_upscale_success", model.TaskStatusSuccess
	success.PrivateData.ResultURL = "https://private-provider.example/result.mp4"
	state := mediakit.State{ExpiresAt: common.GetTimestamp() + 1000, Result: &service.MP4Media{Width: 1280, Height: 720, Duration: 2, FPS: 24, Codec: "h264", Size: 1024, SHA256: strings.Repeat("a", 64)}}
	var err error
	success.PrivateData.PluginState, err = common.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, success.Insert())
	resultPath := createPath + "/" + success.TaskID + "/result"
	res = call(http.MethodGet, resultPath, ownerKey, "", "")
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	assert.NotContains(t, res.Body.String(), "private-provider.example")
	assert.Contains(t, res.Body.String(), `"width":1280`)
	assert.Contains(t, res.Body.String(), `"sha256":"`+state.Result.SHA256+`"`)
	var payload struct {
		Results []struct{ URL string } `json:"results"`
	}
	require.NoError(t, common.Unmarshal(res.Body.Bytes(), &payload))
	require.Len(t, payload.Results, 1)
	state.ExpiresAt = common.GetTimestamp() - 1
	success.PrivateData.PluginState, err = common.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, success.Update())
	res = call(http.MethodGet, resultPath, ownerKey, "", "")
	assert.Equal(t, http.StatusGone, res.Code, res.Body.String())
	res = call(http.MethodGet, payload.Results[0].URL, ownerKey, "", "")
	assert.Contains(t, []int{http.StatusGone, http.StatusNotFound}, res.Code, res.Body.String())
	require.Len(t, provider.captured(), 1)
}

func TestUnifiedVideoGenerationRunsAndReportsSettlement(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Task{}, &model.UsageRecord{}, &model.RelayIdempotency{}, &model.Log{}, &model.UserSubscription{}, &model.SystemTask{}))
	const videoModel = "doubao-seedance-2-0-260128"
	previousPrice := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"`+videoModel+`":0.02}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrice)) })
	const inputKey, foreignKey = "unifiedvideokey", "othervideokey"
	for _, owner := range []struct{ name, key string }{{"submitter", inputKey}, {"other", foreignKey}} {
		user := model.User{Username: owner.name, AffCode: owner.name, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 10_000_000}
		require.NoError(t, model.DB.Create(&user).Error)
		require.NoError(t, (&model.Token{UserId: user.Id, Key: owner.key, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, ModelLimitsEnabled: true, ModelLimits: videoModel}).Insert())
	}
	upstream := newFakeUpstream(t)
	upstreamUpdatedAt := common.GetTimestamp()
	upstream.respond = func(w http.ResponseWriter, _ int, req capturedRequest) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"id":"provider-private-task"}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":"provider-private-task","status":"succeeded","updated_at":%d,"duration":5,"framespersecond":24,"generate_audio":false,"resolution":"720p","content":{"video_url":"https://provider.example/result.mp4"},"usage":{"total_tokens":3000,"completion_tokens":3000}}`, upstreamUpdatedAt)
	}
	channel := model.Channel{Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Key: "provider-key", Name: "test video", BaseURL: new(upstream.URL), Models: videoModel, Group: "default"}
	require.NoError(t, channel.Insert())
	service.InitHttpClient()
	previousFactory := service.GetTaskAdaptorFunc
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(platform) }
	previousQueryLimit := constant.TaskQueryLimit
	constant.TaskQueryLimit = 10
	previousAddress := system_setting.TaskPublicAddress
	system_setting.TaskPublicAddress = "https://media.example.org"
	t.Cleanup(func() {
		service.GetTaskAdaptorFunc = previousFactory
		constant.TaskQueryLimit = previousQueryLimit
		system_setting.TaskPublicAddress = previousAddress
	})
	engine := gin.New()
	engine.Use(middleware.RequestId(), middleware.BodyStorageCleanup())
	SetVideoRouter(engine)
	call := func(method, path, key, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer sk-"+key)
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", fmt.Sprintf("%x", sha256.Sum256([]byte(body))))
		}
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res
	}
	const path = relay.VideoGenerationPath
	for _, invalid := range []string{
		`{"mode":"text_to_video","duration_seconds":65536}`,
		`{"mode":"first_last_frame","references":[]}`,
		`{"mode":"image_to_video","references":[{"role":"first_frame","input":{"url":"https://127.0.0.1/image.png"}}]}`,
	} {
		body := `{"model":"` + videoModel + `","prompt":"test",` + invalid[1:]
		res := call(http.MethodPost, path, inputKey, body)
		assert.Equal(t, http.StatusBadRequest, res.Code, res.Body.String())
		assert.Contains(t, res.Body.String(), `"dispatch_state":"not_sent"`)
	}
	require.Empty(t, upstream.captured())
	const body = `{"model":"` + videoModel + `","mode":"text_to_video","prompt":"test","duration_seconds":5.0,"resolution":"720p","aspect_ratio":"16:9","generate_audio":false,"watermark":false,"metadata":{"task_id":"horo-task"}}`
	res := call(http.MethodPost, path, inputKey, body)
	require.Equal(t, http.StatusAccepted, res.Code, res.Body.String())
	var receipt struct {
		TaskID    string            `json:"task_id"`
		RequestID string            `json:"request_id"`
		Status    string            `json:"status"`
		Metadata  map[string]string `json:"metadata"`
	}
	require.NoError(t, common.Unmarshal(res.Body.Bytes(), &receipt))
	require.NotEmpty(t, receipt.TaskID)
	assert.Equal(t, res.Header().Get("X-Request-Id"), receipt.RequestID)
	assert.Equal(t, "queued", receipt.Status)
	assert.Equal(t, "horo-task", receipt.Metadata["task_id"])
	assert.Contains(t, res.Body.String(), `"status":"pending"`)
	assert.Equal(t, res.Body.String(), call(http.MethodPost, path, inputKey, body).Body.String())
	requests := upstream.captured()
	require.Len(t, requests, 1)
	assert.Equal(t, "/api/v3/contents/generations/tasks", requests[0].URL)
	assert.JSONEq(t, `{"model":"`+videoModel+`","content":[{"type":"text","text":"test"}],"duration":5,"resolution":"720p","ratio":"16:9","generate_audio":false,"watermark":false}`, requests[0].Body)
	assert.Equal(t, http.StatusNotFound, call(http.MethodGet, path+"/"+receipt.TaskID, foreignKey, "").Code)
	submitter, err := model.GetTokenByKey(inputKey, true)
	require.NoError(t, err)
	const siblingKey = "siblingvideokey"
	require.NoError(t, (&model.Token{UserId: submitter.UserId, Key: siblingKey, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, ModelLimitsEnabled: true, ModelLimits: videoModel}).Insert())
	assert.Equal(t, http.StatusNotFound, call(http.MethodGet, path+"/"+receipt.TaskID, siblingKey, "").Code)
	service.RunTaskPollingOnce(context.Background(), nil)
	stored, found, err := model.GetByTaskId(submitter.UserId, receipt.TaskID)
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, stored.SaveVideoMediaResult(context.Background(), &model.VideoMediaResult{
		Width: 1280, Height: 720, Duration: 5, FPS: 24, Codec: "h264", Size: 1024, SHA256: strings.Repeat("a", 64),
	}))
	res = call(http.MethodGet, path+"/"+receipt.TaskID, inputKey, "")
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var task struct {
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
		ExpiresAt int64  `json:"result_expires_at"`
		Billing   struct {
			Status   string `json:"status"`
			RecordID string `json:"record_id"`
			Amount   string `json:"amount"`
		} `json:"billing"`
		Results []struct {
			URL       string  `json:"url"`
			Duration  float64 `json:"duration_seconds"`
			HasAudio  *bool   `json:"has_audio"`
			ExpiresAt int64   `json:"expires_at"`
		} `json:"results"`
	}
	require.NoError(t, common.Unmarshal(res.Body.Bytes(), &task))
	assert.Equal(t, receipt.RequestID, task.RequestID)
	assert.Equal(t, "succeeded", task.Status)
	assert.Contains(t, []string{"settled", "partially_refunded"}, task.Billing.Status)
	assert.Equal(t, "bill_"+receipt.RequestID, task.Billing.RecordID)
	assert.NotEmpty(t, task.Billing.Amount)
	require.Len(t, task.Results, 1)
	assert.Equal(t, time.Unix(upstreamUpdatedAt, 0).Add(24*time.Hour).Unix(), task.ExpiresAt)
	assert.Equal(t, task.ExpiresAt, task.Results[0].ExpiresAt)
	assert.Equal(t, new(false), task.Results[0].HasAudio)
	assert.Equal(t, 5.0, task.Results[0].Duration)
	assert.Contains(t, res.Body.String(), `"width":1280`)
	assert.Contains(t, res.Body.String(), `"size_bytes":1024`)
	assert.Contains(t, res.Body.String(), strings.Repeat("a", 64))
	assert.Contains(t, task.Results[0].URL, "https://media.example.org/v1/tasks/"+receipt.TaskID+"/artifacts/video/content")
	assert.NotContains(t, res.Body.String(), "provider-private-task")
	assert.NotContains(t, res.Body.String(), "provider.example")
	require.Len(t, upstream.captured(), 2)
}

func TestGetOpenAIVideoRouteRendersJimengTask(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousDB := model.DB
	previousDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousSQLitePath := common.SQLitePath
	previousMasterNode := common.IsMasterNode
	previousRedisEnabled := common.RedisEnabled
	common.SQLitePath = t.TempDir() + "/router-video.db"
	common.IsMasterNode = false
	common.RedisEnabled = false
	t.Setenv("SQL_DSN", "")
	require.NoError(t, model.InitDB())
	database := model.DB
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Task{}))
	t.Cleanup(func() {
		sqlDB, closeErr := database.DB()
		require.NoError(t, closeErr)
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.SetDatabaseTypes(previousDatabaseType, previousLogDatabaseType)
		common.SQLitePath = previousSQLitePath
		common.IsMasterNode = previousMasterNode
		common.RedisEnabled = previousRedisEnabled
	})

	require.NoError(t, database.Create(&model.User{
		Id:          91,
		Username:    "jimeng-fetch-user",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Quota:       100,
		Group:       "default",
		AuthVersion: 1,
	}).Error)
	require.NoError(t, database.Create(&model.Token{
		Id:             1,
		UserId:         91,
		Key:            "jimengfetch",
		Status:         common.TokenStatusEnabled,
		Name:           "jimeng fetch",
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)
	require.NoError(t, database.Create(&model.Channel{
		Id:     17,
		Type:   constant.ChannelTypeJimeng,
		Key:    "unused",
		Status: common.ChannelStatusEnabled,
		Name:   "jimeng fetch",
		Models: "jimeng_vgfm_t2v_l20",
		Group:  "default",
	}).Error)

	task := &model.Task{
		CreatedAt: 1710000000,
		UpdatedAt: 1710000060,
		TaskID:    "task_jimeng_public",
		Platform:  constant.TaskPlatform("jimeng"),
		UserId:    91,
		Group:     "default",
		ChannelId: 17,
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		PrivateData: model.TaskPrivateData{
			ResultURL: "data:video/mp4;base64,ZGF0YQ==",
		},
	}
	task.SetData(map[string]any{
		"code": 10000,
		"data": map[string]any{
			"status":    "done",
			"task_id":   "jimeng-private-1",
			"video_url": "https://cdn.example/video.mp4",
		},
		"message": "success",
	})
	require.NoError(t, database.Create(task).Error)

	engine := gin.New()
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	request := httptest.NewRequest(http.MethodGet, "/v1/videos/task_jimeng_public", nil)
	request.Header.Set("Authorization", "Bearer sk-jimengfetch")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		ID          string `json:"id"`
		Object      string `json:"object"`
		Status      string `json:"status"`
		Progress    int    `json:"progress"`
		CreatedAt   int64  `json:"created_at"`
		CompletedAt int64  `json:"completed_at"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "task_jimeng_public", response.ID)
	assert.Equal(t, "video", response.Object)
	assert.Equal(t, "completed", response.Status)
	assert.Equal(t, 100, response.Progress)
	assert.Equal(t, int64(1710000000), response.CreatedAt)
	assert.Equal(t, int64(1710000060), response.CompletedAt)
	assert.NotContains(t, recorder.Body.String(), "cdn.example")
	assert.NotContains(t, recorder.Body.String(), "jimeng-private-1")

	for _, testCase := range []struct {
		name          string
		authorization string
		query         string
		wantStatus    int
	}{
		{name: "missing credential rejected", wantStatus: http.StatusUnauthorized},
		{name: "access rejected", query: "?access=not-a-video-credential", wantStatus: http.StatusUnauthorized},
		{name: "bearer accepted", authorization: "Bearer sk-jimengfetch", wantStatus: http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodGet,
				"/v1/videos/task_jimeng_public/content"+testCase.query,
				nil,
			)
			if testCase.authorization != "" {
				request.Header.Set("Authorization", testCase.authorization)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, testCase.wantStatus, recorder.Code, recorder.Body.String())
			if testCase.wantStatus == http.StatusOK {
				assert.Equal(t, "data", recorder.Body.String())
			}
		})
	}
}

func TestGetOpenAIVideoRouteHandsNativeXAITaskToPluginQuery(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Task{}))
	require.NoError(t, model.DB.Create(&model.User{Id: 92, Username: "xai-fetch-user", Status: common.UserStatusEnabled, Group: "default", Quota: 100}).Error)
	require.NoError(t, model.DB.Create(&model.Token{UserId: 92, Key: "xaifetch", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	for taskID, requestPath := range map[string]string{"task_xai_native": "/v1/videos/generations", "task_xai_pinned": "/xai/v1/videos/generations", "task_xai_openai": "/v1/videos"} {
		task := &model.Task{
			CreatedAt:  1710000000,
			UpdatedAt:  1710000060,
			TaskID:     taskID,
			Platform:   constant.TaskPlatform("xai"),
			UserId:     92,
			Group:      "default",
			Status:     model.TaskStatusSuccess,
			Progress:   "100%",
			Properties: model.Properties{OriginModelName: "grok-imagine-video"},
			PrivateData: model.TaskPrivateData{
				UpstreamTaskID: "xai-private-1",
				Execution: &model.TaskExecutionSnapshot{
					RequestPath: requestPath,
					TaskPlugin:  &model.TaskPluginSnapshot{Key: "xai"},
				},
			},
		}
		task.SetData(map[string]any{
			"request_id": "xai-private-1",
			"status":     "done",
			"model":      "grok-imagine-video",
			"video":      map[string]any{"url": "https://cdn.example/xai.mp4", "duration": 8},
		})
		require.NoError(t, model.DB.Create(task).Error)
	}

	engine := gin.New()
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	source, err := plugins.Source("xai")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	_, err = registry.RegisterFactory(source, jsplugin.Options{Key: "xai"})
	require.NoError(t, err)
	require.NoError(t, registry.SetGenerationPreparer(newPluginGenerationBuilder(engine.Routes(), nil, productionPluginRouteHandlers).prepare))
	require.Empty(t, registry.RoutingErrors())
	previousRegistry := defaultPluginDispatcher.registry
	defaultPluginDispatcher.registry = registry
	t.Cleanup(func() { defaultPluginDispatcher.registry = previousRegistry })

	for _, testCase := range []struct {
		taskID     string
		wantStatus string
		wantObject any
	}{
		// Submitted on the xAI route: xAI's own query response.
		{taskID: "task_xai_native", wantStatus: "done"},
		// The same under /xai.
		{taskID: "task_xai_pinned", wantStatus: "done"},
		// Submitted through POST /v1/videos: the OpenAI video object.
		{taskID: "task_xai_openai", wantStatus: "completed", wantObject: "video"},
	} {
		t.Run(testCase.taskID, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/v1/videos/"+testCase.taskID, nil)
			request.Header.Set("Authorization", "Bearer sk-xaifetch")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var response map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, testCase.wantStatus, response["status"])
			assert.Equal(t, testCase.wantObject, response["object"])
			assert.NotContains(t, recorder.Body.String(), "xai-private-1")
		})
	}
}
