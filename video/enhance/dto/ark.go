package dto

type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
)

type ResolutionRatio struct {
	Origin string
	Ratio  float64
}

type CreateTaskRequest struct {
	VideoURL    string
	ClientToken *string
	Resolution  *string
}
type CreateTaskResponse struct {
	ID string
}

type GetTaskResponse struct {
	Status   TaskStatus
	VideoURL string
	TaskID   string
}
