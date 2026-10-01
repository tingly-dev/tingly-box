package rule

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers all rule routes with swagger documentation
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	// GET /rules - Get all configured rules
	router.GET("/rules", h.GetRules,
		swagger.WithDescription("Get all configured rules"),
		swagger.WithTags("rules"),
		swagger.WithQueryRequired("scenario", "string", "Filter by scenario"),
		swagger.WithResponseModel(RulesResponse{}),
	)

	// GET /rule/:uuid - Get specific rule by UUID
	router.GET("/rule/:uuid", h.GetRule,
		swagger.WithDescription("Get specific rule by UUID"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(RuleResponse{}),
	)

	// POST /rule/:uuid - Create or update a rule configuration
	router.POST("/rule/:uuid", h.UpdateRule,
		swagger.WithDescription("Create or update a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithRequestModel(UpdateRuleRequest{}),
		swagger.WithResponseModel(UpdateRuleResponse{}),
	)

	// POST /rule - Create a new rule
	router.POST("/rule", h.CreateRule,
		swagger.WithDescription("Create or update a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithRequestModel(CreateRuleRequest{}),
		swagger.WithResponseModel(UpdateRuleResponse{}),
	)

	// DELETE /rule/:uuid - Delete a rule configuration
	router.DELETE("/rule/:uuid", h.DeleteRule,
		swagger.WithDescription("Delete a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(DeleteRuleResponse{}),
	)

	// GET /rule/flags/registry - Get catalog of supported rule-level flags
	router.GET("/rule/flags/registry", h.GetFlagRegistry,
		swagger.WithDescription("Get the catalog of supported rule-level flags"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(FlagRegistryResponse{}),
	)
}
