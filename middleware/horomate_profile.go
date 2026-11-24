package middleware

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func HoroMateRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.GetString("token_key"), model.ManagedRelayKeyPrefix) {
			return
		}
		path := c.FullPath()
		var fields string
		switch path {
		case "/v1/responses":
			fields = "model input instructions stream max_output_tokens temperature top_p tools tool_choice parallel_tool_calls reasoning text store include truncation metadata service_tier prompt_cache_key safety_identifier stream_options"
		case "/v1/chat/completions":
			fields = "model messages stream stream_options max_tokens max_completion_tokens temperature top_p n stop presence_penalty frequency_penalty tools tool_choice parallel_tool_calls response_format reasoning_effort seed user metadata store service_tier"
		case "/v1/messages":
			if version := c.GetHeader("anthropic-version"); version != "2023-06-01" || c.GetHeader("anthropic-beta") != "" {
				abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Messages requires anthropic-version 2023-06-01 without beta options", "not_sent")
				return
			}
			fields = "model messages max_tokens system stream temperature top_p top_k stop_sequences tools tool_choice thinking output_config metadata service_tier"
		case "/v1/images/generations", "/v1/images/edits":
			fields = "model prompt size quality n output_format response_format moderation background output_compression user image"
			if path == "/v1/images/edits" {
				fields += " mask"
			}
		default:
			return
		}
		mediaType, _, _ := mime.ParseMediaType(c.ContentType())
		if mediaType == "multipart/form-data" && path == "/v1/images/edits" {
			form, err := common.ParseMultipartFormReusable(c)
			if err != nil {
				abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Invalid image edit form", "not_sent")
				return
			}
			defer form.RemoveAll()
			if len(form.File["image[]"]) == 0 || len(form.Value["prompt"]) != 1 || strings.TrimSpace(form.Value["prompt"][0]) == "" {
				abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Image edit requires image[] files", "not_sent")
				return
			}
			for name, values := range form.Value {
				if !strings.Contains(" "+fields+" ", " "+name+" ") || name == "image" || name == "mask" || len(values) != 1 || name == "n" && values[0] != "1" {
					abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Invalid image edit field", "not_sent")
					return
				}
			}
			for name, files := range form.File {
				if name != "image[]" && (name != "mask" || len(files) != 1) {
					abortRelayIdempotency(c, http.StatusBadRequest, "unknown_parameter", "Unknown image edit file part", "not_sent")
					return
				}
			}
			if format := form.Value["response_format"]; len(format) > 0 && format[0] != "b64_json" && format[0] != "url" {
				abortRelayIdempotency(c, http.StatusUnprocessableEntity, "unsupported_parameter", "Invalid image response format", "not_sent")
				return
			}
			c.Request.MultipartForm = form
			c.Set("horomate_image_format", c.Request.MultipartForm.Value["response_format"])
			c.Next()
			return
		}
		if mediaType != "application/json" {
			abortRelayIdempotency(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "Expected application/json", "not_sent")
			return
		}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Unable to read the request body", "not_sent")
			return
		}
		reader, err := storage.NewReader()
		if err != nil {
			abortRelayIdempotency(c, http.StatusInternalServerError, "internal_error", "Unable to read the request body", "not_sent")
			return
		}
		var input map[string]common.RawMessage
		err = common.DecodeJsonBody(reader, &input)
		_ = reader.Close()
		if err != nil || input == nil {
			abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Expected a JSON object", "not_sent")
			return
		}
		for name, value := range input {
			if strings.ContainsRune(name, ' ') || !strings.Contains(" "+fields+" ", " "+name+" ") {
				abortRelayIdempotency(c, http.StatusBadRequest, "unknown_parameter", "A request field is outside the HoroMate profile", "not_sent")
				return
			}
			if bytes.Equal(value, []byte("null")) {
				abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Omit optional fields that have no value", "not_sent")
				return
			}
			if name == "n" && string(value) != "1" || name == "store" && string(value) != "false" {
				abortRelayIdempotency(c, http.StatusUnprocessableEntity, "unsupported_parameter", "Only n=1 and store=false are supported", "not_sent")
				return
			}
		}
		if input["max_tokens"] != nil && input["max_completion_tokens"] != nil {
			abortRelayIdempotency(c, http.StatusUnprocessableEntity, "invalid_parameter_combination", "Specify only one token limit", "not_sent")
			return
		}
		if strings.HasPrefix(path, "/v1/images/") {
			var prompt, format string
			if common.Unmarshal(input["prompt"], &prompt) != nil || strings.TrimSpace(prompt) == "" {
				abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "An image prompt is required", "not_sent")
				return
			}
			if path == "/v1/images/edits" {
				var images []string
				if common.Unmarshal(input["image"], &images) != nil || len(images) == 0 {
					abortRelayIdempotency(c, http.StatusBadRequest, "invalid_request", "Image edit requires an image array", "not_sent")
					return
				}
			}
			if value := input["response_format"]; value != nil && (common.Unmarshal(value, &format) != nil || format != "b64_json" && format != "url") {
				abortRelayIdempotency(c, http.StatusUnprocessableEntity, "unsupported_parameter", "Invalid image response format", "not_sent")
				return
			}
			c.Set("horomate_image_format", []string{format})
		}
		// Apply the profile defaults before the downstream decoder reads the body.
		changed := false
		if (path == "/v1/responses" || path == "/v1/chat/completions") && input["store"] == nil {
			input["store"] = common.RawMessage("false")
			changed = true
		}
		if strings.HasPrefix(path, "/v1/images/") && input["response_format"] == nil {
			input["response_format"] = common.RawMessage(`"b64_json"`)
			changed = true
		}
		if changed {
			body, err := common.Marshal(input)
			if err != nil {
				abortRelayIdempotency(c, http.StatusInternalServerError, "internal_error", "Unable to normalize request", "not_sent")
				return
			}
			common.CleanupBodyStorage(c)
			c.Set(common.KeyRequestBody, nil)
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			c.Request.ContentLength = int64(len(body))
		}
	}
}
