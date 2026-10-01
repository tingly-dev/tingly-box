package info

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

var _ module.Module = (*Handler)(nil)

// RegisterRoutes wires the /info/* endpoints onto the route groups.
// Public receives routes that need no authentication (health check).
// V1 receives the authenticated info routes.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	apiAuth, apiV1 := rt.Public, rt.V1
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
