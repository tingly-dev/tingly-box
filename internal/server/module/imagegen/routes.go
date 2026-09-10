package imagegen

import "github.com/tingly-dev/tingly-box/swagger"

// RegisterRoutes registers the imagegen control-plane routes.
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// GET /imagegen/info - Report read-only imagegen scenario info (output directory, ...)
	router.GET("/imagegen/info", handler.GetInfo,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Get read-only info about the imagegen scenario, including the local output directory"),
		swagger.WithResponseModel(ImageGenInfoResponse{}))
}
