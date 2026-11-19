package ark

import "fmt"

type createTaskRequest struct {
	VideoURL               string   `json:"video_url"`
	ToolVersion            *string  `json:"tool_version,omitempty"`
	Scene                  *string  `json:"scene,omitempty"`
	EnhanceStyle           *string  `json:"enhance_style,omitempty"`
	ColorStyle             *string  `json:"color_style,omitempty"`
	SkinSmoothing          *int     `json:"skin_smoothing,omitempty"`
	FaceSharpness          *int     `json:"face_sharpness,omitempty"`
	GlareSuppression       *string  `json:"glare_suppression,omitempty"`
	Resolution             *string  `json:"resolution,omitempty"`
	ResolutionLimit        *int     `json:"resolution_limit,omitempty"`
	BitrateLevel           *string  `json:"bitrate_level,omitempty"`
	Bitrate                *int     `json:"bitrate,omitempty"`
	FPS                    *float64 `json:"fps,omitempty"`
	BitDepth               *int     `json:"bit_depth,omitempty"`
	Codec                  *string  `json:"codec,omitempty"`
	PixFmt                 *string  `json:"pix_fmt,omitempty"`
	DynamicRange           *string  `json:"dynamic_range,omitempty"`
	MediaOutputDestination *string  `json:"media_output_destination,omitempty"`
	ClientToken            *string  `json:"client_token,omitempty"`
	CallbackArgs           *string  `json:"callback_args,omitempty"`
	CallbackURL            *string  `json:"callback_url,omitempty"`
	QueueID                *string  `json:"queue_id,omitempty"`
}

type getTaskRequest struct {
	TaskID string `uri:"task_id" json:"-"`
}

type baseResponse struct {
	Success   bool   `json:"success"`
	RequestID string `json:"request_id"`
	Error     struct {
		Code    string  `json:"code"`
		Type    string  `json:"type"`
		Message string  `json:"message"`
		Param   *string `json:"param,omitempty"`
	} `json:"error"`
}

func (br *baseResponse) validate() error {
	if br.Success {
		return nil
	}
	err := fmt.Errorf("RequestID:%s,Code:%s,Message:%s",
		br.RequestID, br.Error.Code, br.Error.Message)
	return err
}

type createTaskResponse struct {
	baseResponse
	TaskID string `json:"task_id"`
}

type getTaskResponse struct {
	baseResponse
	TaskID   string `json:"task_id"`
	TaskType string `json:"task_type"`
	Status   string `json:"status"`
	Result   struct {
		VideoURL    string  `json:"video_url"`
		Duration    float64 `json:"duration"`
		Resolution  string  `json:"resolution"`
		FPS         float64 `json:"fps"`
		ToolVersion string  `json:"tool_version"`
	} `json:"result"`
	ExpiresAt  *int64 `json:"expires_at,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	FinishedAt *int64 `json:"finished_at,omitempty"`
	QueueID    string `json:"queue_id"`
}
