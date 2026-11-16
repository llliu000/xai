package dto

type WanCreateTaskRequest struct{}

type WanCreateTaskResponse struct{}

type WanGetTaskRequest struct {
	TaskId string `uri:"task_id"`
}

type WanGetTaskResponse struct{}
