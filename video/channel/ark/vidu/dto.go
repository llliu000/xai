package vidu

import "github.com/QuantumNous/new-api/video/dto"

type CreateTaskRequest struct {
	dto.ArkCreateTaskRequest
	Moderation string `json:"moderation"`
}

type GetTaskResponse struct {
	Id      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content *struct {
		VideoUrl     string `json:"video_url"`
		LastFrameUrl string `json:"last_frame_url"`
	} `json:"content"`
	Usage *struct {
		PromptTokens     string `json:"prompt_tokens"`
		CompletionTokens string `json:"completion_tokens"`
		TotalTokens      string `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
	Tools []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
	Seed                  string `json:"seed"`
	Resolution            string `json:"resolution"`
	Ratio                 string `json:"ratio"`
	Duration              string `json:"duration"`
	FramesPerSecond       string `json:"frames_per_second"`
	ServiceTier           string `json:"service_tier"`
	ExecutionExpiresAfter string `json:"execution_expires_after"`
	GenerateAudio         bool   `json:"generate_audio"`
	Draft                 bool   `json:"draft"`
	OmniReferenceTaskType string `json:"omni_reference_task_type"`
	OutputFormat          string `json:"output_format"`
}

type AssetBaseResult[T any] struct {
	ResponseMetadata struct {
		RequestId string `json:"RequestId"`
		Action    string `json:"Action"`
		Version   string `json:"Version"`
		Service   string `json:"Service"`
		Region    string `json:"Region"`
		Error     struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"ResponseMetadata"`
	Result T `json:"Result"`
}

type CreateAsset struct {
	*dto.CreateAssetRequest
	Moderation string `json:"moderation"`
}
