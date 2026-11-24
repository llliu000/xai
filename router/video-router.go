package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	videoSharedRouter := router.Group("/v1")
	videoSharedRouter.Use(middleware.RouteTag("relay"))
	videoSharedRouter.Use(middleware.TokenAuth())
	videoSharedRouter.Use(middleware.SystemPerformanceCheck())
	videoSharedRouter.POST(
		"/video/generations",
		middleware.PinTaskPluginEndpoint(),
		middleware.TaskPluginEndpointOnly(middleware.ModelRequestRateLimit()),
		middleware.PrepareTaskPluginEndpoint(),
		middleware.Distribute(),
		middleware.RelayIdempotency(),
		func(c *gin.Context) {
			controller.RelayTaskPluginEndpoint(c, controller.RelayTask)
		},
	)
	videoSharedRouter.GET("/video/generations/:task_id", controller.GetVideoGenerationTask)
	videoSharedRouter.GET("/upscale/models", controller.ListUpscaleModels)
	videoSharedRouter.POST("/upscale/tasks", middleware.ModelRequestRateLimit(), middleware.RelayIdempotency(), middleware.Distribute(), controller.RelayTask)
	videoSharedRouter.GET("/upscale/tasks/:task_id", controller.GetUpscaleTask)
	videoSharedRouter.GET("/upscale/tasks/:task_id/result", controller.GetUpscaleResult)

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}
}
