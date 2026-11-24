package relay

import (
	"context"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const VideoGenerationPath = "/v1/video/generations"
const VideoGenerationMetadataKey = "video_generation_metadata"
const VideoGenerationMaxBodyBytes = 256 << 10

// SeedanceVideoGenerationLimits reports the limits used by the unified video request.
func SeedanceVideoGenerationLimits(name string) (seconds, images int, edit bool) {
	name = strings.ToLower(name)
	if !strings.HasPrefix(name, "doubao-seedance-2-") && !strings.HasPrefix(name, "dreamina-seedance-2-") {
		return 0, 0, false
	}
	if !strings.Contains(name, "seedance-2-5-") {
		return 15, 9, false
	}
	if strings.Contains(name, "-flash-") {
		return 30, 10, true
	}
	return 30, 30, true
}

var videoModelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,191}$`)
var arkPromptParameter = regexp.MustCompile(`(?i)(?:^|\s)--(?:duration|frames|resolution|ratio|seed|watermark|camera_fixed|dur|rs|rt|wm|cf)\b`)

type videoGenerationRequest struct {
	Model           string            `json:"model"`
	Mode            string            `json:"mode"`
	Prompt          string            `json:"prompt"`
	NegativePrompt  *string           `json:"negative_prompt,omitempty"`
	DurationSeconds *float64          `json:"duration_seconds,omitempty"`
	Resolution      *string           `json:"resolution,omitempty"`
	AspectRatio     *string           `json:"aspect_ratio,omitempty"`
	References      []videoReference  `json:"references,omitempty"`
	GenerateAudio   *bool             `json:"generate_audio,omitempty"`
	Seed            *int              `json:"seed,omitempty"`
	PromptExtend    *bool             `json:"prompt_extend,omitempty"`
	Watermark       *bool             `json:"watermark,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type videoReference struct {
	Role  string `json:"role"`
	Input struct {
		URL    *string `json:"url,omitempty"`
		FileID *string `json:"file_id,omitempty"`
	} `json:"input"`
}

func prepareVideoGeneration(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	badRequest := func(code, message string) *dto.TaskError {
		err := service.TaskErrorWrapperLocal(errors.New(message), code, http.StatusBadRequest)
		err.NoRetry = true
		return err
	}
	unsupported := func(message string) *dto.TaskError {
		err := service.TaskErrorWrapperLocal(errors.New(message), "unsupported_parameter", http.StatusUnprocessableEntity)
		err.NoRetry = true
		return err
	}
	mediaType, _, _ := mime.ParseMediaType(c.ContentType())
	if mediaType != "application/json" {
		err := service.TaskErrorWrapperLocal(errors.New("Video generation requires application/json"), "unsupported_media_type", http.StatusUnsupportedMediaType)
		err.NoRetry = true
		return err
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return badRequest("invalid_request", "Unable to read video request")
	}
	if storage.Size() > VideoGenerationMaxBodyBytes {
		err := service.TaskErrorWrapperLocal(errors.New("Video request exceeds 256 KiB"), "payload_too_large", http.StatusRequestEntityTooLarge)
		err.NoRetry = true
		return err
	}
	raw, err := storage.Bytes()
	if err != nil {
		return badRequest("invalid_request", "Unable to read video request")
	}
	var fields map[string]common.RawMessage
	var request videoGenerationRequest
	if common.Unmarshal(raw, &fields) != nil || common.Unmarshal(raw, &request) != nil ||
		!videoModelID.MatchString(request.Model) || strings.TrimSpace(request.Prompt) == "" || len(request.Prompt) > 64<<10 {
		return badRequest("invalid_request", "Invalid video request, model and prompt are required")
	}
	for field, value := range fields {
		switch field {
		case "model", "mode", "prompt", "negative_prompt", "duration_seconds", "resolution", "aspect_ratio", "references", "generate_audio", "seed", "prompt_extend", "watermark", "metadata":
			if string(value) != "null" {
				continue
			}
			return badRequest("invalid_request", "Video fields must be omitted or contain a value")
		default:
			return badRequest("unknown_parameter", "A video request field is not recognized")
		}
	}
	if request.DurationSeconds != nil {
		seconds := *request.DurationSeconds
		if seconds < 1 || seconds > relaycommon.MaxTaskDurationSeconds {
			return badRequest("invalid_request", "duration_seconds is out of range")
		}
		if math.Trunc(seconds) != seconds {
			return unsupported("The selected video model requires a whole number of seconds")
		}
	}
	if len(request.Metadata) > 16 {
		return badRequest("invalid_request", "Too many metadata fields")
	}
	for key := range request.Metadata {
		if n := utf8.RuneCountInString(key); n == 0 || n > 64 || slices.Contains([]string{"key", "api_key", "authorization", "password", "token", "secret"}, strings.ToLower(key)) {
			return badRequest("invalid_request", "Invalid metadata field")
		}
	}
	if len(request.References) > 33 {
		return badRequest("invalid_request", "Too many video references")
	}
	var first, last, images, videos, sources int
	for _, ref := range request.References {
		if (ref.Input.URL == nil) == (ref.Input.FileID == nil) {
			return badRequest("invalid_request", "A media input requires exactly one of url or file_id")
		}
		switch ref.Role {
		case "first_frame":
			first++
		case "last_frame":
			last++
		case "reference_image":
			images++
		case "reference_video":
			videos++
		case "source_video":
			sources++
		default:
			return badRequest("invalid_request", "Invalid video reference role")
		}
	}
	count := len(request.References)
	validReferences := false
	switch request.Mode {
	case "text_to_video":
		validReferences = count == 0
	case "image_to_video":
		validReferences = count == 1 && first == 1
	case "first_last_frame":
		validReferences = count == 2 && first == 1 && last == 1
	case "reference_to_video":
		validReferences = count > 0 && count == images+videos
	case "video_edit":
		validReferences = sources == 1 && count == images+sources
	}
	if !validReferences {
		return badRequest("invalid_parameter_combination", "Video references do not match the requested mode")
	}
	plugin, _ := common.GetContextKeyType[jsplugin.PinnedPlugin](c, jsplugin.ContextKeyPinnedPlugin)
	if plugin.Plugin == nil || plugin.Plugin.Meta.Key != "doubao" {
		return unsupported("The selected channel does not support the unified video request")
	}
	modelName := strings.ToLower(info.UpstreamModelName)
	if !strings.Contains(modelName, "seedance-2-") {
		modelName = strings.ToLower(request.Model)
	}
	maxSeconds, maxImages, is25 := SeedanceVideoGenerationLimits(modelName)
	if maxSeconds == 0 {
		return unsupported("The unified video request requires a configured Seedance 2.x model")
	}
	if request.DurationSeconds != nil && (*request.DurationSeconds < 4 || *request.DurationSeconds > float64(maxSeconds)) {
		return unsupported(fmt.Sprintf("For this model duration_seconds must be between 4 and %d", maxSeconds))
	}
	if images > maxImages || videos > 3 {
		return unsupported("Too many references for the selected video model")
	}
	if request.NegativePrompt != nil || request.Seed != nil || request.PromptExtend != nil && *request.PromptExtend {
		return unsupported("The selected video model does not support negative_prompt, seed or prompt_extend")
	}
	if arkPromptParameter.MatchString(request.Prompt) {
		return badRequest("invalid_request", "Set video options in the request fields instead of the prompt")
	}
	params := map[string]any{"generate_audio": false, "watermark": true}
	if is25 {
		params["output_format"] = "mp4"
	}
	if request.GenerateAudio != nil {
		params["generate_audio"] = *request.GenerateAudio
	}
	if request.Watermark != nil {
		params["watermark"] = *request.Watermark
	}
	if request.Resolution != nil {
		allowed := []string{"480p", "720p"}
		if !strings.Contains(modelName, "-mini-") && !strings.Contains(modelName, "-fast-") && !strings.Contains(modelName, "-flash-") {
			allowed = append(allowed, "1080p", "4k")
		}
		if !slices.Contains(allowed, *request.Resolution) {
			return unsupported("The selected model does not support this resolution")
		}
		params["resolution"] = *request.Resolution
	}
	if request.AspectRatio != nil {
		ratio := *request.AspectRatio
		if !slices.Contains([]string{"16:9", "9:16", "21:9", "4:3", "3:4", "1:1", "adaptive"}, ratio) ||
			is25 && (first > 0 || sources > 0) && ratio != "adaptive" {
			return unsupported("The selected video mode does not support this aspect_ratio")
		}
		params["ratio"] = ratio
	}
	if request.Mode == "video_edit" {
		if !is25 || request.DurationSeconds != nil {
			return unsupported("Video edit requires Seedance 2.5 and uses the source video duration")
		}
		params["omni_reference_task_type"] = "edit"
		params["duration"], params["ratio"] = -1, "adaptive"
	} else if is25 && request.Mode == "reference_to_video" {
		params["omni_reference_task_type"] = "reference"
	}
	for _, ref := range request.References {
		if ref.Input.FileID != nil {
			return unsupported("Uploaded files are not available on this endpoint yet")
		}
	}
	if count > 0 {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		content := make([]map[string]any, 0, count)
		checked := make(map[string]string, count)
		for _, ref := range request.References {
			referenceURL := *ref.Input.URL
			resolved, ok := checked[referenceURL]
			if !ok {
				resolved, err = service.ResolvePublicVideoMediaURL(ctx, referenceURL)
				if err != nil {
					return badRequest("invalid_request", "A reference must be a reachable public HTTPS URL")
				}
				checked[referenceURL] = resolved
			}
			kind, role := "image_url", ref.Role
			if role == "reference_video" || role == "source_video" {
				kind, role = "video_url", "reference_video"
			}
			content = append(content, map[string]any{"type": kind, kind: map[string]string{"url": resolved}, "role": role})
		}
		params["content"] = content
	}
	if request.DurationSeconds != nil {
		params["duration"] = int(*request.DurationSeconds)
	}
	// The legacy task driver receives only fields explicitly mapped to Ark's
	// wire format. The public metadata is kept with the task, away from pricing.
	c.Set("task_request", map[string]any{"model": request.Model, "prompt": request.Prompt, "metadata": params})
	c.Set(VideoGenerationMetadataKey, request.Metadata)
	info.Action = request.Mode
	return nil
}
