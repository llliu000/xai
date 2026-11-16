package video

import (
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/video/common"
	"github.com/QuantumNous/new-api/video/handler"
	"github.com/gin-gonic/gin"
)

func NewVideoServer() error {
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.ContextWithFallback = true

	channelGroup := engine.Group("/channels/:channel-name")
	channelGroup.Use(func(ctx *gin.Context) {
		if key := ctx.GetHeader("Authorization"); key == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{})
			return
		}
		channelName := ctx.Param("channel-name")
		if channelName == "" {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{})
			return
		}
		if !slices.Contains(common.Channels, channelName) {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{})
			return
		}
		common.SetRelayContext(ctx)
	})
	// ark渠道适配
	arkRouter := channelGroup.Group("/api/v3/contents/generations/tasks")
	{
		arkHandler := handler.NewArk()
		arkRouter.POST("", arkHandler.CreateTask)
		arkRouter.GET("/:task_id", arkHandler.GetTask)
	}

	// wan渠道适配
	wanRouter := channelGroup.Group("/api/v1")
	{
		wanHandler := handler.NewWan()
		wanRouter.GET("/tasks/:task_id", wanHandler.GetTask)
		wanRouter.POST("/services/aigc/video-generation/video-synthesis", wanHandler.CreateTask)
	}
	err := http.ListenAndServe(":8080", engine)
	return err
}
