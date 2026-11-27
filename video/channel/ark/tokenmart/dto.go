package tokenmart

type ResultResponse struct {
	Task struct {
		Id              string   `json:"id"`
		Status          string   `json:"status"`
		Model           string   `json:"model"`
		DurationSeconds int      `json:"duration_seconds"`
		Outputs         []string `json:"outputs"`
		Error           *string  `json:"error"`
		CreatedAt       any      `json:"created_at"`
		CompletedAt     any      `json:"completed_at"`
		Usage           struct {
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Metadata map[string]any `json:"metadata"`
	} `json:"task"`
}

type BaseAssetResponse[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}
