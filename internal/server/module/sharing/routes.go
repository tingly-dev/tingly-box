package sharing

import "github.com/tingly-dev/tingly-box/swagger"

// RegisterRoutes wires all token management endpoints onto the given group.
func RegisterRoutes(group *swagger.RouteGroup, h *Handler) {
	group.POST("/tokens", h.Create,
		swagger.WithTags("tokens"),
		swagger.WithDescription("Create a shared API token"),
		swagger.WithRequestModel(TokenCreateRequest{}),
		swagger.WithResponseModel(TokenCreateResponse{}),
	)
	group.GET("/tokens", h.List,
		swagger.WithTags("tokens"),
		swagger.WithDescription("List shared API tokens"),
		swagger.WithQueryModel(TokenListQuery{}),
		swagger.WithResponseModel(TokenListResponse{}),
	)
	group.GET("/tokens/:token_id", h.Get,
		swagger.WithTags("tokens"),
		swagger.WithDescription("Get a shared API token"),
		swagger.WithResponseModel(APITokenInfo{}),
	)
	group.DELETE("/tokens/:token_id", h.Delete,
		swagger.WithTags("tokens"),
		swagger.WithDescription("Delete a shared API token"),
	)
	group.PUT("/tokens/:token_id/enable", h.Enable,
		swagger.WithTags("tokens"),
		swagger.WithDescription("Enable a shared API token"),
	)
	group.PUT("/tokens/:token_id/disable", h.Disable,
		swagger.WithTags("tokens"),
		swagger.WithDescription("Disable a shared API token"),
	)
	group.POST("/tokens/:token_id/regenerate", h.Regenerate,
		swagger.WithTags("tokens"),
		swagger.WithDescription("Regenerate a shared API token"),
		swagger.WithResponseModel(TokenCreateResponse{}),
	)
}
