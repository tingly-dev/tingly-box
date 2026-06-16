package onboarding

import "github.com/tingly-dev/tingly-box/swagger"

// RegisterRoutes wires the onboarding endpoints onto a swagger route group.
// The route group is expected to already carry the user auth middleware.
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	router.POST("/onboarding/extract", handler.Extract,
		swagger.WithDescription("Extract provider candidates from arbitrary text input"),
		swagger.WithTags("onboarding"),
		swagger.WithRequestModel(ExtractRequest{}),
		swagger.WithResponseModel(ExtractResponse{}),
	)
}
