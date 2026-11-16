package dto

type ArkCreateTaskRequest struct {
}

type ArkCreateTaskResponse struct{}

type ArkGetTaskRequest struct {
	TaskId string `uri:"task_id"`
}

type ArkGetTaskResponse struct{}
