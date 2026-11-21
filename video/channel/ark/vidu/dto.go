package vidu

import "github.com/QuantumNous/new-api/video/dto"

type CreateTaskRequest struct {
	dto.ArkCreateTaskRequest
	Moderation string `json:"moderation"`
}

type AssetBaseResult[T any] struct {
	ResponseMetadata struct {
		RequestId string `json:"RequestId"`
		Action    string `json:"Action"`
		Version   string `json:"Version"`
		Service   string `json:"Service"`
		Region    string `json:"Region"`
		Error     struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"ResponseMetadata"`
	Result T `json:"Result"`
}

type CreateAsset struct {
	*dto.CreateAssetRequest
	Moderation string `json:"moderation"`
}
