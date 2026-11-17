package common

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	RelayContextKey = "relay-context-key"
)

const (
	Mock = "mock"
	Vidu = "vidu"
)

type RelayContext struct {
	ChannelName string
	ApiKey      string
}

func SetRelayContext(ctx *gin.Context) {
	channelName := ctx.Param("channel-name")
	apiKey := ctx.GetHeader("Authorization")
	apiKey, _ = strings.CutPrefix(apiKey, "Bearer ")

	relayContext := RelayContext{
		ChannelName: channelName,
		ApiKey:      apiKey,
	}
	relayCtx := context.WithValue(
		ctx.Request.Context(),
		RelayContextKey,
		relayContext,
	)

	ctx.Request = ctx.Request.WithContext(relayCtx)
}

func GetRelayContext(ctx context.Context) RelayContext {
	relayContext, _ := ctx.Value(RelayContextKey).(RelayContext)
	return relayContext
}
