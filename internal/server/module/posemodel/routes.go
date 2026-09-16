package posemodel

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes wires the authenticated management endpoints.
func RegisterRoutes(apiV1 *swagger.RouteGroup, h *Handler) {
	apiV1.GET("/pose/model", h.GetStatus,
		swagger.WithTags("pose"),
		swagger.WithDescription("Report whether the pose estimator's runtime and model are downloaded and ready to serve"),
		swagger.WithResponseModel(StatusResponse{}),
	)
	apiV1.POST("/pose/model/ensure", h.Ensure,
		swagger.WithTags("pose"),
		swagger.WithDescription("Download the pose estimator's runtime and model into the config directory if they are missing, verifying each against its pinned checksum"),
		swagger.WithResponseModel(StatusResponse{}),
	)
}

// RegisterFileRoute mounts the unauthenticated file route on the engine,
// beside the SPA's own static assets.
func RegisterFileRoute(engine *gin.Engine, h *Handler) {
	engine.GET(strings.TrimSuffix(BaseURL, "/")+"/:name", h.ServeFile)
}
