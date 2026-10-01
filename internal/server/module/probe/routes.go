package probe

import (
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers all probe-module endpoints on the given route group.
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V2
	router.POST("/probe", h.HandleE2EProbe,
		swagger.WithDescription("End-to-end probe - SDK-level test for rules, providers, and unsaved provider config"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(probe.E2ERequest{}),
		swagger.WithResponseModel(E2EResponse{}),
	)

	router.POST("/probe/curl", h.HandleCurlProbe,
		swagger.WithDescription("Generate the curl command equivalent to a probe request without executing it (secrets as placeholder env vars)"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(probe.E2ERequest{}),
		swagger.WithResponseModel(CurlResponse{}),
	)

	router.POST("/probe/lightweight", h.HandleLightweightProbe,
		swagger.WithDescription("Lightweight probe for optional key validation using OPTIONS and models endpoint"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(probe.LightweightProbeRequest{}),
		swagger.WithResponseModel(LightweightResponse{}),
	)
}
