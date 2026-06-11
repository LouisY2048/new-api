package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetOpenApiRouter(router *gin.Engine) {
	openRouter := router.Group("/open/v1")
	openRouter.Use(middleware.RouteTag("openapi"))
	openRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	openRouter.Use(middleware.BodyStorageCleanup())
	openRouter.Use(middleware.OpenAuth())
	{
		openRouter.POST("/apikey/issue", controller.IssueApiKey)
		openRouter.POST("/token/usage", controller.OpenGetTokenUsage)
	}
}
