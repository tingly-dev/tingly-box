package info

import "github.com/tingly-dev/tingly-box/swagger"

// RegisterRoutes wires the /info/* endpoints onto the given route groups.
// apiAuth receives routes that need no authentication (health check).
// apiV1 receives the authenticated info routes.
func RegisterRoutes(apiAuth, apiV1 *swagger.RouteGroup, h *Handler) {
	apiAuth.GET("/info/health", h.GetHealthInfo,
		swagger.WithTags("info"),
		swagger.WithResponseModel(HealthInfoResponse{}),
	)

	apiV1.GET("/info/config", h.GetInfoConfig,
		swagger.WithTags("info"),
		swagger.WithDescription("Get config info about this application"),
		swagger.WithResponseModel(ConfigInfoResponse{}),
	)

	apiV1.GET("/info/version", h.GetInfoVersion,
		swagger.WithTags("info"),
		swagger.WithDescription("Get version info about this application"),
		swagger.WithResponseModel(VersionInfoResponse{}),
	)

	apiV1.GET("/info/version/check", h.GetLatestVersion,
		swagger.WithTags("info"),
		swagger.WithDescription("Check if a newer version is available on GitHub"),
		swagger.WithResponseModel(LatestVersionResponse{}),
	)
}
