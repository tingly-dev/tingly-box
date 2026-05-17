package probe

import (
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers all probe-module endpoints on the given route group.
func RegisterRoutes(router *swagger.RouteGroup, h *Handler) {
	router.POST("/probe", h.HandleE2EProbe,
		swagger.WithDescription("End-to-end probe - SDK-level test for rules, providers, and unsaved provider config"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(probe.E2ERequest{}),
		swagger.WithResponseModel(E2EResponse{}),
	)

	router.POST("/probe/lightweight", h.HandleLightweightProbe,
		swagger.WithDescription("Lightweight probe for optional key validation using OPTIONS and models endpoint"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(probe.LightweightProbeRequest{}),
		swagger.WithResponseModel(LightweightResponse{}),
	)
}
