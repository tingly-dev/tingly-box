package imagegen

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers the imagegen control-plane routes.
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	// GET /imagegen/info - Report read-only imagegen scenario info (output directory, ...)
	router.GET("/imagegen/info", h.GetInfo,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Get read-only info about the imagegen scenario, including the local output directory"),
		swagger.WithResponseModel(ImageGenInfoResponse{}))
}
