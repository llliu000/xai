package dto

type ArkCreateTaskRequest struct {
	Model                 string       `json:"model"`
	Content               []ArkContent `json:"content"`
	Resolution            *string      `json:"resolution,omitempty"`
	Ratio                 *string      `json:"ratio,omitempty"`
	Duration              *int         `json:"duration,omitempty"`
	Frames                *int         `json:"frames,omitempty"`
	GenerateAudio         *bool        `json:"generate_audio,omitempty"`
	Watermark             *bool        `json:"watermark,omitempty"`
	OutputFormat          *string      `json:"output_format,omitempty"`
	Seed                  *int         `json:"seed,omitempty"`
	CameraFixed           *bool        `json:"camera_fixed,omitempty"`
	ReturnLastFrame       *bool        `json:"return_last_frame,omitempty"`
	Draft                 *bool        `json:"draft,omitempty"`
	ServiceTier           *string      `json:"service_tier,omitempty"`
	CallbackURL           *string      `json:"callback_url,omitempty"`
	ExecutionExpiresAfter *int         `json:"execution_expires_after,omitempty"`
	Priority              *int         `json:"priority,omitempty"`
	SafetyIdentifier      *string      `json:"safety_identifier,omitempty"`
	Tools                 []ArkTool    `json:"tools,omitempty"`
	OmniReferenceTaskType *string      `json:"omni_reference_task_type,omitempty"`
}
type ArkContent struct {
	Type      string        `json:"type"`
	Text      *string       `json:"text,omitempty"`
	ImageURL  *ArkMedia     `json:"image_url,omitempty"`
	VideoURL  *ArkMedia     `json:"video_url,omitempty"`
	AudioURL  *ArkMedia     `json:"audio_url,omitempty"`
	DraftTask *ArkDraftTask `json:"draft_task,omitempty"`
	Role      *string       `json:"role,omitempty"`
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
	ID string `json:"id"`
}

type ArkGetTaskRequest struct {
	TaskId string `uri:"task_id"`
}

type ArkGetTaskResponse struct {
	ID                    string     `json:"id"`
	Model                 string     `json:"model"`
	Status                string     `json:"status"`
	Content               *ArkOutput `json:"content,omitempty"`
	CreatedAt             int64      `json:"created_at"`
	Draft                 *bool      `json:"draft,omitempty"`
	Duration              *int       `json:"duration,omitempty"`
	Error                 *ArkError  `json:"error,omitempty"`
	ExecutionExpiresAfter *int       `json:"execution_expires_after,omitempty"`
	Frames                *int       `json:"frames,omitempty"`
	FramesPerSecond       *int       `json:"framespersecond,omitempty"`
	GenerateAudio         *bool      `json:"generate_audio,omitempty"`
	OutputFormat          *string    `json:"output_format,omitempty"`
	Priority              *int       `json:"priority,omitempty"`
	Ratio                 *string    `json:"ratio,omitempty"`
	Resolution            *string    `json:"resolution,omitempty"`
	SafetyIdentifier      *string    `json:"safety_identifier,omitempty"`
	Seed                  *int       `json:"seed,omitempty"`
	ServiceTier           *string    `json:"service_tier,omitempty"`
	Tools                 []ArkTool  `json:"tools,omitempty"`
	UpdatedAt             int64      `json:"updated_at"`
	Usage                 *ArkUsage  `json:"usage,omitempty"`
}

type ArkOutput struct {
	LastFrameURL *string `json:"last_frame_url,omitempty"`
	VideoURL     *string `json:"video_url,omitempty"`
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
