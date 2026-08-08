package plugin

import (
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes wires the plugin endpoints onto the given route group.
func RegisterRoutes(api *swagger.RouteGroup, h *Handler) {
	// Register (or update) external plugin code as an upstream, optionally
	// binding a rule — an idempotent upsert-by-name.
	api.POST("/plugins", h.RegisterPlugin,
		swagger.WithDescription("Register (or update) external plugin code as an upstream, optionally binding a rule"),
		swagger.WithTags("plugins"),
		swagger.WithRequestModel(RegisterPluginRequest{}),
		swagger.WithResponseModel(RegisterPluginResponse{}),
	)

	api.GET("/plugins", h.ListPlugins,
		swagger.WithDescription("List registered plugin providers"),
		swagger.WithTags("plugins"),
		swagger.WithResponseModel(PluginsResponse{}),
	)
}
