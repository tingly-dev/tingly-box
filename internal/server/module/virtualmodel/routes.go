package virtualmodel

import (
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers virtual-model management routes under /api/v1.
func RegisterRoutes(router *swagger.RouteGroup, authMiddleware gin.HandlerFunc, handler *Handler) {
	router.Router.Use(authMiddleware)

	router.GET("/vmodel/available-models", handler.ListAvailableModels,
		swagger.WithTags("vmodel"),
		swagger.WithDescription("List virtual models registered in the in-process registries, grouped by protocol"),
		swagger.WithResponseModel(AvailableModelsResponse{}),
	)
}
