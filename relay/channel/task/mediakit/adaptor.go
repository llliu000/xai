package mediakit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/video/enhance/ark"
	"github.com/gin-gonic/gin"
)

const baseURL = "https://mediakit.cn-beijing.volces.com"
const MetadataContextKey = "upscale_task_metadata"

var vendorID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,191}$`)

type Adaptor struct {
	request ark.TaskRequest
	state   State
}

type mediaProbe func(context.Context, string) (service.MP4Media, string, error)

type State struct {
	TargetWidth  int               `json:"target_width"`
	TargetHeight int               `json:"target_height"`
	SourceFPS    float64           `json:"source_fps"`
	SourceLength float64           `json:"source_length"`
	SourceAudio  bool              `json:"source_audio"`
	Result       *service.MP4Media `json:"result,omitempty"`
	ExpiresAt    int64             `json:"expires_at,omitempty"`
}

type createRequest struct {
	Model     string `json:"model"`
	MediaType string `json:"media_type"`
	Input     struct {
		URL    string  `json:"url"`
		FileID *string `json:"file_id,omitempty"`
	} `json:"input"`
	Target struct {
		Scale  *float64 `json:"scale,omitempty"`
		Width  *int     `json:"width,omitempty"`
		Height *int     `json:"height,omitempty"`
		Fit    *string  `json:"fit,omitempty"`
	} `json:"target"`
	Output struct {
		Format        string  `json:"format"`
		Codec         *string `json:"video_codec,omitempty"`
		CRF           *int    `json:"crf,omitempty"`
		Quality       *int    `json:"quality,omitempty"`
		PreserveAudio *bool   `json:"preserve_audio,omitempty"`
	} `json:"output"`
	Enhance  map[string]common.RawMessage `json:"enhance,omitempty"`
	Metadata map[string]string            `json:"metadata,omitempty"`
}

func (*Adaptor) Init(*relaycommon.RelayInfo) {}

func (a *Adaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return a.prepareTask(c, info, service.ProbePublicMP4)
}

func (a *Adaptor) prepareTask(c *gin.Context, info *relaycommon.RelayInfo, probe mediaProbe) *dto.TaskError {
	mediaType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mediaType != "application/json" {
		return requestError(http.StatusUnsupportedMediaType, "unsupported_media_type", "Upscale requires application/json")
	}
	if info.ChannelType != constant.ChannelTypeCustom || info.ApiKey == "" {
		return requestError(http.StatusServiceUnavailable, "unavailable", "A MediaKit channel is not configured")
	}
	var request createRequest
	storage, err := common.GetBodyStorage(c)
	if err != nil || storage.Size() > 64<<10 {
		return requestError(http.StatusRequestEntityTooLarge, "payload_too_large", "Upscale request is too large")
	}
	body, err := storage.NewReader()
	if err != nil {
		return requestError(http.StatusBadRequest, "invalid_request", "Unable to read upscale request")
	}
	defer body.Close()
	if err := common.DecodeStrictJsonBody(body, &request); err != nil {
		return requestError(http.StatusBadRequest, "invalid_request", "Invalid upscale request fields")
	}
	if request.Model != constant.MediaKitModel || info.OriginModelName != constant.MediaKitModel || info.UpstreamModelName != constant.MediaKitModel {
		return requestError(http.StatusUnprocessableEntity, "unsupported_parameter", "Unsupported upscale model")
	}
	price, ok := ratio_setting.GetModelPrice(request.Model, false)
	if !ok || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return requestError(http.StatusServiceUnavailable, "unavailable", "The upscale model has no configured task price")
	}
	if request.MediaType != "video" || request.Input.FileID != nil || request.Output.Format != "mp4" ||
		request.Output.Codec != nil || request.Output.CRF != nil || request.Output.Quality != nil ||
		request.Output.PreserveAudio != nil && !*request.Output.PreserveAudio || len(request.Enhance) > 0 {
		return requestError(http.StatusUnprocessableEntity, "unsupported_parameter", "MediaKit supports HTTPS video input and MP4 output with source audio")
	}
	if len(request.Metadata) > 16 || request.Input.URL == "" {
		return requestError(http.StatusBadRequest, "invalid_request", "Missing video URL or invalid metadata")
	}
	for key := range request.Metadata {
		if n := utf8.RuneCountInString(key); n == 0 || n > 64 || slices.Contains([]string{"key", "api_key", "authorization", "password", "token", "secret"}, strings.ToLower(key)) {
			return requestError(http.StatusBadRequest, "invalid_request", "Invalid metadata field")
		}
	}
	target := request.Target
	if target.Scale != nil {
		if *target.Scale != 2 || target.Width != nil || target.Height != nil || target.Fit != nil {
			return requestError(http.StatusUnprocessableEntity, "invalid_parameter_combination", "Specify scale 2 or an exact width and height with fit contain")
		}
	} else if target.Width == nil || target.Height == nil || target.Fit == nil || *target.Fit != "contain" ||
		*target.Width < 1 || *target.Height < 1 || max(*target.Width, *target.Height) > 3840 {
		return requestError(http.StatusUnprocessableEntity, "unsupported_parameter", "The target requires supported dimensions and fit contain")
	}
	media, inputURL, err := probe(c.Request.Context(), request.Input.URL)
	if err != nil {
		return requestError(http.StatusUnprocessableEntity, "unsupported_media_type", "Unable to read a supported MP4 from the public HTTPS URL")
	}
	if short, long := min(media.Width, media.Height), max(media.Width, media.Height); short < 360 || short > 1080 || long > 1920 ||
		media.Duration <= 0 || media.FPS < 1 || media.FPS > 120 {
		return requestError(http.StatusUnprocessableEntity, "unsupported_parameter", "Source MP4 exceeds the upscale model limits")
	}
	width, height := media.Width*2, media.Height*2
	if target.Scale == nil {
		width, height = *target.Width, *target.Height
	}
	resolution := map[int]string{720: "720p", 1080: "1080p", 1440: "2k", 2160: "4k"}[min(width, height)]
	if resolution == "" || width <= media.Width || height <= media.Height || width%2 != 0 || height%2 != 0 ||
		max(width, height) > 3840 || width*media.Height != height*media.Width {
		return requestError(http.StatusUnprocessableEntity, "unsupported_parameter", "The target must match a supported resolution tier without changing aspect ratio")
	}
	a.request = ark.TaskRequest{VideoURL: inputURL, Resolution: new(resolution), ClientToken: new(info.PublicTaskID)}
	a.state = State{TargetWidth: width, TargetHeight: height, SourceFPS: media.FPS, SourceLength: media.Duration, SourceAudio: media.HasAudio}
	c.Set(MetadataContextKey, request.Metadata)
	info.Action = constant.TaskActionUpscaleVideo
	return nil
}

func requestError(status int, code, message string) *dto.TaskError {
	err := service.TaskErrorWrapperLocal(errors.New(message), code, status)
	err.NoRetry = true
	return err
}

func (*Adaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return taskURL(info.ChannelBaseUrl, ark.GenerativeTaskPath)
}

func taskURL(channelURL, path string) (string, error) {
	if channelURL == "" {
		channelURL = baseURL
	}
	return url.JoinPath(channelURL, path)
}

func (*Adaptor) BuildRequestHeader(_ *gin.Context, request *http.Request, info *relaycommon.RelayInfo) error {
	request.Header.Set("Authorization", "Bearer "+info.ApiKey)
	request.Header.Set("Content-Type", "application/json")
	return nil
}

func (a *Adaptor) BuildRequestBody(*gin.Context, *relaycommon.RelayInfo) (io.Reader, error) {
	payload, err := common.Marshal(a.request)
	return bytes.NewReader(payload), err
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	response, err := channel.DoTaskApiRequest(a, c, info, body)
	if err == nil && response != nil {
		response.Body = http.MaxBytesReader(nil, response.Body, 64<<10)
	}
	return response, err
}

func (a *Adaptor) ParseResponse(_ *gin.Context, response *http.Response, _ *relaycommon.RelayInfo) (*channel.TaskSubmitResponse, *dto.TaskError) {
	var submitted ark.SubmitResponse
	if err := common.DecodeJsonBody(response.Body, &submitted); err != nil || !submitted.Success || !vendorID.MatchString(submitted.TaskID) {
		return nil, requestError(http.StatusBadGateway, "upstream_error", "MediaKit did not confirm task submission")
	}
	state, err := common.Marshal(a.state)
	if err != nil {
		return nil, requestError(http.StatusInternalServerError, "internal_error", "Unable to store upscale task options")
	}
	return &channel.TaskSubmitResponse{UpstreamTaskID: submitted.TaskID, PluginState: state}, nil
}

func (*Adaptor) FetchTask(channelURL, key string, task *model.Task, proxy string) (*http.Response, error) {
	if !vendorID.MatchString(task.GetUpstreamTaskID()) {
		return nil, errors.New("invalid MediaKit task ID")
	}
	endpoint, err := taskURL(channelURL, ark.TaskQueryPath+"/"+task.GetUpstreamTaskID())
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	limited := *client
	limited.Timeout = 15 * time.Second
	limited.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := limited.Do(request)
	if err == nil && response != nil {
		response.Body = http.MaxBytesReader(nil, response.Body, 64<<10)
	}
	return response, err
}

func (*Adaptor) ParseTaskResult(task *model.Task, _ *http.Response, body []byte) (*relaycommon.TaskInfo, error) {
	return parseTaskResult(task, body, service.ProbePublicMP4)
}

func parseTaskResult(task *model.Task, body []byte, probe mediaProbe) (*relaycommon.TaskInfo, error) {
	var response ark.TaskResponse
	if err := common.Unmarshal(body, &response); err != nil || !response.Success || response.TaskID != task.GetUpstreamTaskID() {
		return nil, errors.New("invalid MediaKit task check response")
	}
	result := &relaycommon.TaskInfo{TaskID: task.GetUpstreamTaskID()}
	switch response.Status {
	case "pending", "queued":
		result.Status = model.TaskStatusQueued
	case "running", "processing":
		result.Status = model.TaskStatusInProgress
	case "failed", "cancelled":
		result.Status, result.Reason = model.TaskStatusFailure, "MediaKit could not complete the upscale task"
	case "completed":
		var state State
		if err := common.Unmarshal(task.PrivateData.PluginState, &state); err != nil || state.TargetWidth <= 0 || state.TargetHeight <= 0 {
			return nil, errors.New("missing stored upscale target")
		}
		if response.FinishedAt == nil || *response.FinishedAt <= 0 || *response.FinishedAt > common.GetTimestamp()+60 {
			return nil, errors.New("missing MediaKit completion time")
		}
		state.ExpiresAt = *response.FinishedAt + int64((24*time.Hour)/time.Second)
		if response.ExpiresAt != nil && *response.ExpiresAt > 0 {
			state.ExpiresAt = min(state.ExpiresAt, *response.ExpiresAt)
		}
		if state.ExpiresAt <= common.GetTimestamp() {
			result.Status, result.Reason = model.TaskStatusFailure, "MediaKit output expired before it could be verified"
			return result, nil
		}
		media, finalURL, err := probe(context.Background(), response.Result.VideoURL)
		if err != nil && !errors.Is(err, service.ErrUnsupportedMP4) {
			return nil, errors.New("MediaKit result could not be downloaded for verification")
		}
		if err != nil || media.Width != state.TargetWidth || media.Height != state.TargetHeight ||
			math.Abs(media.FPS-state.SourceFPS) > 0.05 || math.Abs(media.Duration-state.SourceLength) > 0.1 || media.HasAudio != state.SourceAudio {
			result.Status, result.Reason = model.TaskStatusFailure, "MediaKit output did not match the requested video properties"
			return result, nil
		}
		state.Result = &media
		result.Status, result.Url = model.TaskStatusSuccess, finalURL
		result.PluginState, err = common.Marshal(state)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unknown MediaKit task status")
	}
	return result, nil
}

func (*Adaptor) EstimateBilling(*gin.Context, *relaycommon.RelayInfo) map[string]float64 { return nil }
func (*Adaptor) AdjustBillingOnSubmit(*relaycommon.RelayInfo, []byte) map[string]float64 { return nil }
func (*Adaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int          { return 0 }
func (*Adaptor) GetModelList() []string                                                  { return []string{constant.MediaKitModel} }
func (*Adaptor) GetChannelName() string                                                  { return "MediaKit" }
