package mediakit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaKitTaskRequiresExactTargetAndVerifiedResult(t *testing.T) {
	previousPrice := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"`+constant.MediaKitModel+`":0.05}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrice)) })
	const sourceURL = "https://media.example.org/source.mp4"
	const outputURL = "https://media.example.org/enhanced.mp4"
	probe := func(_ context.Context, raw string) (service.MP4Media, string, error) {
		require.Equal(t, sourceURL, raw)
		return service.MP4Media{Width: 640, Height: 360, Duration: 2, FPS: 24, HasAudio: true}, raw, nil
	}
	info := &relaycommon.RelayInfo{OriginModelName: constant.MediaKitModel,
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCustom, ApiKey: "private", UpstreamModelName: constant.MediaKitModel},
	}
	for _, tc := range []struct {
		name, target string
		status       int
	}{
		{"exact scale", `{"scale":2}`, http.StatusAccepted},
		{"exact dimensions", `{"width":1280,"height":720,"fit":"contain"}`, http.StatusAccepted},
		{"rounded scale", `{"scale":1.5}`, http.StatusUnprocessableEntity},
		{"changed aspect ratio", `{"width":1440,"height":720,"fit":"contain"}`, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			payload := fmt.Sprintf(`{"model":%q,"media_type":"video","input":{"url":%q},"target":%s,"output":{"format":"mp4"}}`, constant.MediaKitModel, sourceURL, tc.target)
			c.Request = httptest.NewRequest(http.MethodPost, constant.UpscaleTaskPath, strings.NewReader(payload))
			c.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			adaptor := &Adaptor{}
			apiErr := adaptor.prepareTask(c, info, probe)
			if tc.status != http.StatusAccepted {
				require.NotNil(t, apiErr)
				assert.Equal(t, tc.status, apiErr.StatusCode)
				return
			}
			require.Nil(t, apiErr)
			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			outbound, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"video_url":"`+sourceURL+`","resolution":"720p","client_token":"task_public"}`, string(outbound))
			result, apiErr := adaptor.ParseResponse(c, &http.Response{Body: io.NopCloser(strings.NewReader(`{"success":true,"task_id":"amk-private"}`))}, info)
			require.Nil(t, apiErr)
			task := &model.Task{PrivateData: model.TaskPrivateData{UpstreamTaskID: result.UpstreamTaskID, PluginState: result.PluginState}}
			now := common.GetTimestamp()
			completed := fmt.Appendf(nil, `{"success":true,"task_id":"amk-private","status":"completed","finished_at":%d,"expires_at":%d,"result":{"video_url":%q}}`, now, now+5000, outputURL)
			for _, changeRate := range []bool{false, true} {
				media := service.MP4Media{Width: 1280, Height: 720, FPS: 24, Duration: 2, HasAudio: true, Size: 1024, SHA256: strings.Repeat("a", 64), Codec: "h264"}
				if changeRate {
					media.FPS = 30
				}
				polled, err := parseTaskResult(task, completed, func(_ context.Context, raw string) (service.MP4Media, string, error) {
					require.Equal(t, outputURL, raw)
					return media, outputURL, nil
				})
				require.NoError(t, err)
				if changeRate {
					assert.Equal(t, model.TaskStatusFailure, polled.Status)
					assert.Empty(t, polled.Url)
					continue
				}
				assert.Equal(t, model.TaskStatusSuccess, polled.Status)
				assert.Equal(t, outputURL, polled.Url)
				var state State
				require.NoError(t, common.Unmarshal(polled.PluginState, &state))
				assert.Equal(t, now+5000, state.ExpiresAt)
				assert.Equal(t, &media, state.Result)
			}
		})
	}
}
