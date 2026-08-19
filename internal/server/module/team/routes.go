package team

import "github.com/tingly-dev/tingly-box/swagger"

func RegisterRoutes(group *swagger.RouteGroup, h *Handler) {
	group.GET("/teams", h.List,
		swagger.WithTags("teams"),
		swagger.WithDescription("List teams"),
		swagger.WithResponseModel(ListResponse{}),
	)
	group.POST("/teams", h.Create,
		swagger.WithTags("teams"),
		swagger.WithDescription("Create a team"),
		swagger.WithRequestModel(CreateRequest{}),
		swagger.WithResponseModel(TeamInfo{}),
	)
	group.PUT("/teams/:team_id", h.Update,
		swagger.WithTags("teams"),
		swagger.WithDescription("Update a team"),
		swagger.WithRequestModel(UpdateRequest{}),
		swagger.WithResponseModel(TeamInfo{}),
	)
	group.PUT("/teams/:team_id/enable", h.Enable,
		swagger.WithTags("teams"),
		swagger.WithDescription("Enable a team and its sharing keys"),
		swagger.WithResponseModel(TeamInfo{}),
	)
	group.PUT("/teams/:team_id/disable", h.Disable,
		swagger.WithTags("teams"),
		swagger.WithDescription("Disable a team and all of its sharing keys"),
		swagger.WithResponseModel(TeamInfo{}),
	)
	group.DELETE("/teams/:team_id", h.Delete,
		swagger.WithTags("teams"),
		swagger.WithDescription("Delete an empty, non-default team"),
	)
}
