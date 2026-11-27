package enhance

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/video/enhance/ark"
	"github.com/QuantumNous/new-api/video/enhance/dto"
	"github.com/QuantumNous/new-api/video/enhance/mock"
)

var TargetResolutionMapping = map[string]dto.ResolutionRatio{
	"720p": {
		Origin: "480p",
		Ratio:  2.5,
	},
	"1080p": {
		Origin: "720p",
		Ratio:  5,
	},
}

type Adapter interface {
	CreateTask(ctx context.Context, req dto.CreateTaskRequest) (*dto.CreateTaskResponse, error)
	GetTask(ctx context.Context, taskID string) (*dto.GetTaskResponse, error)
}

func GetConfig(enhanceResolution string) (*dto.ResolutionRatio, error) {
	resolutionRatio, ok := TargetResolutionMapping[enhanceResolution]
	if !ok {
		return nil, fmt.Errorf("not support enhance resolution %s", enhanceResolution)
	}
	return &resolutionRatio, nil
}

func GetAdapter(name string) (Adapter, error) {
	switch name {
	case "mock":
		return &mock.Adapter{}, nil
	default:
		var adapter ark.Adapter
		config := common.OptionMap["enhance.config"]
		_ = json.Unmarshal([]byte(config), &adapter)
		return &adapter, nil
	}
}
