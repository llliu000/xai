package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func SetAssetRouter(router *gin.Engine) {
	assetController := &controller.AssetController{}
	assetAdminRouter := router.Group("/api")
	assetAdminRouter.Use(middleware.RouteTag("api"), middleware.AccessTokenAudit(), middleware.BodyStorageCleanup(),
		middleware.GlobalAPIRateLimit(), middleware.DisableCache(), middleware.AdminAuth())
	handlePermissionRoute(assetAdminRouter, http.MethodGet, "/channel/seedance-options", authz.ChannelRead, assetController.GetSeedanceChannelOptions)
	handlePermissionRoute(assetAdminRouter, http.MethodPost, "/channel/:id/assets/sync", authz.ChannelWrite, assetController.ManualAsset)
	handlePermissionRoute(assetAdminRouter, http.MethodGet, "/channel/:id/assets/sync", authz.ChannelRead, assetController.GetAssetSyncProgress)
	// User management uses administrator roles; declare its existing PAT scope separately.
	middleware.DeclareAccessTokenPermissionRoute(http.MethodPut, "/api/user/:id/seedance-channels", authz.Permission{Resource: "user", Action: authz.ActionWrite})
	assetAdminRouter.PUT("/user/:id/seedance-channels", assetController.UpdateUserSeedanceChannels)

	// doubao https://www.volcengine.com/docs/82379/1521309?lang=zh
	assetV3Router := router.Group("/api/v3/asset")
	assetV3Router.Use(middleware.RouteTag("relay"))
	assetV3Router.Use(middleware.GlobalAPIRateLimit())
	assetV3Router.Use(middleware.TokenAuth())
	{
		assetV3Router.POST("", assetController.Action)
		assetV3Router.POST("/", assetController.Action)
	}
}
