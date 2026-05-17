package probe

import (
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers all probe-module endpoints on the given route group.
func RegisterRoutes(router *swagger.RouteGroup, h *Handler) {
	router.POST("/probe", h.HandleProbeV2,
		swagger.WithDescription("Probe V2 - Unified probe endpoint for testing rules, providers, and unsaved provider config"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(probe.ProbeV2Request{}),
		swagger.WithResponseModel(V2Response{}),
	)

	router.POST("/probe/lightweight", h.HandleLightweightProbe,
		swagger.WithDescription("Lightweight probe for optional key validation using OPTIONS and models endpoint"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(probe.LightweightProbeRequest{}),
		swagger.WithResponseModel(LightweightResponse{}),
	)
}
