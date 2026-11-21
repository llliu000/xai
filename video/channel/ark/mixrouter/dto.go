package mixrouter

import "encoding/json"

type CreateTaskRequest struct {
	Prompt   string `json:"prompt"`
	Model    string `json:"model"`
	Asset    bool   `json:"asset"`
	Metadata any    `json:"metadata"`
}

type CreateTaskResponse struct {
	TaskID string `json:"task_id,omitempty"`
}

type GetTaskResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Data json.RawMessage `json:"data"`
	} `json:"data"`
}
