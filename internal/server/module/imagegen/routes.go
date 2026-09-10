package imagegen

import "github.com/tingly-dev/tingly-box/swagger"

// RegisterRoutes registers the imagegen control-plane routes.
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// GET /imagegen/output-dir - Report the generated-image output directory path
	router.GET("/imagegen/output-dir", handler.GetOutputDir,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Get the local path generated/edited images are persisted to"),
		swagger.WithResponseModel(OutputDirResponse{}))
}
