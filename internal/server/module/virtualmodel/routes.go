package virtualmodel

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"

	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers virtual-model management routes under the given
// /api/v1 group. Auth middleware is attached per-route via WithMiddleware so
// this registration does not mutate the parent group's middleware chain —
// other modules sharing /api/v1 are unaffected by registration order.
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router, authMiddleware := rt.V1, rt.UserAuth
	router.GET("/vmodel/available-models", h.ListAvailableModels,
		swagger.WithTags("vmodel"),
		swagger.WithDescription("List virtual models registered in the in-process registries, grouped by protocol"),
		swagger.WithResponseModel(AvailableModelsResponse{}),
		swagger.WithMiddleware(authMiddleware),
	)
}
