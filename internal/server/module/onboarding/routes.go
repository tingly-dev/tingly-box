package onboarding

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes wires the onboarding endpoints onto a swagger route group.
// The route group is expected to already carry the user auth middleware.
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	router.POST("/onboarding/extract", h.Extract,
		swagger.WithDescription("Extract provider candidates from arbitrary text input"),
		swagger.WithTags("onboarding"),
		swagger.WithRequestModel(ExtractRequest{}),
		swagger.WithResponseModel(ExtractResponse{}),
	)
}
