package dto

type ArkContentType string

const (
	ArkContentTypeText      ArkContentType = "text"
	ArkContentTypeImage     ArkContentType = "image_url"
	ArkContentTypeAudio     ArkContentType = "audio_url"
	ArkContentTypeVideo     ArkContentType = "video_url"
	ArkContentTypeDraftTask ArkContentType = "draft_task"
)

type ArkTaskStatus string

const (
	ArkTaskStatusSucceeded ArkTaskStatus = "succeeded"
	ArkTaskStatusCancelled ArkTaskStatus = "cancelled"
	ArkTaskStatusFailed    ArkTaskStatus = "failed"
	ArkTaskStatusRunning   ArkTaskStatus = "running"
	ArkTaskStatusQueued    ArkTaskStatus = "queued"
	ArkTaskStatusExpired   ArkTaskStatus = "expired"
)

type ArkCreateTaskRequest struct {
	Model                 string        `json:"model"`
	Content               []*ArkContent `json:"content"`
	SafetyIdentifier      *string       `json:"safety_identifier,omitempty"`
	CallbackURL           *string       `json:"callback_url,omitempty"`
	ReturnLastFrame       *bool         `json:"return_last_frame,omitempty"`
	ServiceTier           *string       `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *int64        `json:"execution_expires_after,omitempty"`
	Priority              *int32        `json:"priority,omitempty"`
	GenerateAudio         *bool         `json:"generate_audio,omitempty"`
	Draft                 *bool         `json:"draft,omitempty"`
	CameraFixed           *bool         `json:"camera_fixed,omitempty"`
	Watermark             *bool         `json:"watermark,omitempty"`
	Seed                  *int64        `json:"seed,omitempty"`
	Resolution            *string       `json:"resolution,omitempty"`
	Ratio                 *string       `json:"ratio,omitempty"`
	OutputFormat          *string       `json:"output_format,omitempty"`
	OmniReferenceTaskType *string       `json:"omni_reference_task_type,omitempty"`
	Duration              *int64        `json:"duration,omitempty"`
	Frames                *int64        `json:"frames,omitempty"`
	Tools                 []*ArkTool    `json:"tools,omitempty"`
}

type ArkContent struct {
	Type      ArkContentType `json:"type"`
	Text      *string        `json:"text,omitempty"`
	ImageURL  *ArkMedia      `json:"image_url,omitempty"`
	AudioURL  *ArkMedia      `json:"audio_url,omitempty"`
	VideoURL  *ArkMedia      `json:"video_url,omitempty"`
	Role      *string        `json:"role,omitempty"`
	DraftTask *ArkDraftTask  `json:"draft_task,omitempty"`
}

type ArkMedia struct {
	URL string `json:"url"`
}

type ArkDraftTask struct {
	ID string `json:"id"`
}

type ArkTool struct {
	Type string `json:"type"`
}

type ArkCreateTaskResponse struct {
	ID               string  `json:"id"`
	SafetyIdentifier *string `json:"safety_identifier,omitempty"`
}

type ArkGetTaskRequest struct {
	TaskID string `uri:"task_id" json:"id"`
}

type ArkGetTaskResponse struct {
	ID                    string        `json:"id"`
	Model                 string        `json:"model"`
	SafetyIdentifier      *string       `json:"safety_identifier,omitempty"`
	Status                ArkTaskStatus `json:"status"`
	Error                 *ArkError     `json:"error,omitempty"`
	Content               ArkOutput     `json:"content"`
	Usage                 ArkUsage      `json:"usage"`
	SubdivisionLevel      *string       `json:"subdivisionlevel,omitempty"`
	FileFormat            *string       `json:"fileformat,omitempty"`
	Frames                *int64        `json:"frames"`
	FramesPerSecond       *int64        `json:"framespersecond"`
	Resolution            *string       `json:"resolution,omitempty"`
	Ratio                 *string       `json:"ratio,omitempty"`
	OutputFormat          *string       `json:"output_format,omitempty"`
	Duration              *int64        `json:"duration,omitempty"`
	CreatedAt             int64         `json:"created_at"`
	UpdatedAt             int64         `json:"updated_at"`
	Seed                  *int64        `json:"seed,omitempty"`
	RevisedPrompt         *string       `json:"revised_prompt,omitempty"`
	ServiceTier           *string       `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *int64        `json:"execution_expires_after,omitempty"`
	Priority              *int32        `json:"priority,omitempty"`
	GenerateAudio         *bool         `json:"generate_audio,omitempty"`
	Draft                 *bool         `json:"draft,omitempty"`
	DraftTaskID           *string       `json:"draft_task_id,omitempty"`
	Tools                 []*ArkTool    `json:"tools,omitempty"`
}

type ArkOutput struct {
	VideoURL     string `json:"video_url"`
	LastFrameURL string `json:"last_frame_url"`
	FileURL      string `json:"file_url"`
}

type ArkError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ArkUsage struct {
	CompletionTokens int64         `json:"completion_tokens"`
	TotalTokens      int64         `json:"total_tokens"`
	ToolUsage        *ArkToolUsage `json:"tool_usage,omitempty"`
}

type ArkToolUsage struct {
	WebSearch int64 `json:"web_search"`
}
