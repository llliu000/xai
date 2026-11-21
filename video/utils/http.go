package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type HttpClient struct {
	Url    string
	ApiKey string
}

func NewHttpClient(url, apiKey string) *HttpClient {
	return &HttpClient{url, apiKey}
}

func (hc *HttpClient) Post(ctx context.Context, request, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	bodyReader := bytes.NewReader(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", hc.Url, bodyReader)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+hc.ApiKey)
	httpReq.Header.Set("Accept", "application/json")
	httpResp, err := http.DefaultClient.Do(httpReq.WithContext(ctx))
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("http status code: %d, %s", httpResp.StatusCode, responseBody)
	}
	err = json.Unmarshal(responseBody, response)
	return err
}

func (hc *HttpClient) Get(ctx context.Context, response any) error {
	httpReq, err := http.NewRequest("GET", hc.Url, nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+hc.ApiKey)
	httpReq.Header.Set("Accept", "application/json")
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("http status code: %d, %s", httpResp.StatusCode, responseBody)
	}
	err = json.Unmarshal(responseBody, response)
	return err
}
