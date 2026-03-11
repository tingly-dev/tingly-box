package rule

import (
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers all rule routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// GET /rules - Get all configured rules
	router.GET("/rules", handler.GetRules,
		swagger.WithDescription("Get all configured rules"),
		swagger.WithTags("rules"),
		swagger.WithQueryRequired("scenario", "string", "Filter by scenario"),
		swagger.WithResponseModel(RulesResponse{}),
	)

	// GET /rule/:uuid - Get specific rule by UUID
	router.GET("/rule/:uuid", handler.GetRule,
		swagger.WithDescription("Get specific rule by UUID"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(RuleResponse{}),
	)

	// POST /rule/:uuid - Create or update a rule configuration
	router.POST("/rule/:uuid", handler.UpdateRule,
		swagger.WithDescription("Create or update a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithRequestModel(UpdateRuleRequest{}),
		swagger.WithResponseModel(UpdateRuleResponse{}),
	)

	// POST /rule - Create a new rule
	router.POST("/rule", handler.CreateRule,
		swagger.WithDescription("Create or update a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithRequestModel(CreateRuleRequest{}),
		swagger.WithResponseModel(UpdateRuleResponse{}),
	)

	// DELETE /rule/:uuid - Delete a rule configuration
	router.DELETE("/rule/:uuid", handler.DeleteRule,
		swagger.WithDescription("Delete a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(DeleteRuleResponse{}),
	)
}
