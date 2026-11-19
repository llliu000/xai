package ark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/video/enhance"
)

type Adapter struct {
	ToolVersion string `json:"tool_version"`
	BaseURL     string `json:"base_url"`
	ApiKey      string `json:"api_key"`
}

var _ enhance.Adapter = (*Adapter)(nil)

func (a *Adapter) CreateTask(ctx context.Context, req enhance.CreateTaskRequest) (*enhance.CreateTaskResponse, error) {
	request := createTaskRequest{
		VideoURL:   req.VideoURL,
		Resolution: req.Resolution,
		//MediaOutputDestination: new("tos://"),
	}
	var uri string
	switch a.ToolVersion {
	case "fast":
		uri = "/api/v1/tools/enhance-video-fast"
	case "standard", "professional":
		request.ToolVersion = new(a.ToolVersion)
		uri = "/api/v1/tools/enhance-video"
		request.Scene = new("aigc")
	case "generative":
		uri = "/api/v1/tools/enhance-video-generative"
	default:
		uri = "/api/v1/tools/enhance-video"
	}
	var response createTaskResponse
	if err := a.request(ctx, http.MethodPost, uri, request, &response); err != nil {
		return nil, err
	}
	if err := response.validate(); err != nil {
		return nil, err
	}
	return &enhance.CreateTaskResponse{ID: response.TaskID}, nil
}

func (a *Adapter) GetTask(ctx context.Context, taskID string) (*enhance.GetTaskResponse, error) {
	var response getTaskResponse
	uri := fmt.Sprintf("/api/v1/tasks/%s", taskID)
	if err := a.request(ctx, http.MethodGet, uri, struct{}{}, &response); err != nil {
		return nil, err
	}
	if err := response.validate(); err != nil {
		return nil, err
	}

	task := &enhance.GetTaskResponse{TaskID: response.TaskID}
	switch response.Status {
	case "running":
		task.Status = enhance.TaskStatusRunning
	case "completed":
		task.Status = enhance.TaskStatusSucceeded
		task.VideoURL = response.Result.VideoURL
	case "failed":
		task.Status = enhance.TaskStatusFailed
	default:
		task.Status = enhance.TaskStatusQueued
	}
	return task, nil
}

func (a *Adapter) request(ctx context.Context, method, path string, request, response any) error {
	baseUrl, err := url.JoinPath(a.BaseURL, path)
	if err != nil {
		return err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(body)
	req, err := http.NewRequestWithContext(ctx, method, baseUrl, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.ApiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, responseBody)
	}
	err = json.Unmarshal(responseBody, response)
	return err
}
