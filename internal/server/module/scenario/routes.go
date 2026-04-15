package scenario

import "github.com/tingly-dev/tingly-box/pkg/swagger"

// RegisterRoutes registers all scenario routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// GET /scenarios - Get all scenario configurations
	router.GET("/scenarios", handler.GetScenarios,
		swagger.WithDescription("Get all scenario configurations"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenariosResponse{}),
	)

	// GET /scenario/:scenario - Get configuration for a specific scenario
	router.GET("/scenario/:scenario", handler.GetScenarioConfig,
		swagger.WithDescription("Get configuration for a specific scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioResponse{}),
	)

	// POST /scenario/:scenario - Create or update scenario configuration
	router.POST("/scenario/:scenario", handler.SetScenarioConfig,
		swagger.WithDescription("Create or update scenario configuration"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ScenarioUpdateRequest{}),
		swagger.WithResponseModel(ScenarioUpdateResponse{}),
	)

	// GET /scenario/:scenario/flag/:flag - Get a specific flag value for a scenario
	router.GET("/scenario/:scenario/flag/:flag", handler.GetScenarioFlag,
		swagger.WithDescription("Get a specific flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// PUT /scenario/:scenario/flag/:flag - Set a specific flag value for a scenario
	router.PUT("/scenario/:scenario/flag/:flag", handler.SetScenarioFlag,
		swagger.WithDescription("Set a specific flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ScenarioFlagUpdateRequest{}),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// GET /scenario/:scenario/string-flag/:flag - Get a specific string flag value for a scenario
	router.GET("/scenario/:scenario/string-flag/:flag", handler.GetScenarioStringFlag,
		swagger.WithDescription("Get a specific string flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// PUT /scenario/:scenario/string-flag/:flag - Set a specific string flag value for a scenario
	router.PUT("/scenario/:scenario/string-flag/:flag", handler.SetScenarioStringFlag,
		swagger.WithDescription("Set a specific string flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ScenarioStringFlagUpdateRequest{}),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// --- Profile endpoints ---

	// GET /scenario/:scenario/profiles - List profiles for a scenario
	router.GET("/scenario/:scenario/profiles", handler.GetProfiles,
		swagger.WithDescription("List profiles for a scenario"),
		swagger.WithTags("scenarios"),
	)

	// POST /scenario/:scenario/profiles - Create a new profile
	router.POST("/scenario/:scenario/profiles", handler.CreateProfile,
		swagger.WithDescription("Create a new profile for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ProfileCreateRequest{}),
	)

	// PUT /scenario/:scenario/profiles/:id - Update a profile
	router.PUT("/scenario/:scenario/profiles/:id", handler.UpdateProfile,
		swagger.WithDescription("Update a profile name or mode"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ProfileUpdateRequest{}),
	)

	// DELETE /scenario/:scenario/profiles/:id - Delete a profile
	router.DELETE("/scenario/:scenario/profiles/:id", handler.DeleteProfile,
		swagger.WithDescription("Delete a profile"),
		swagger.WithTags("scenarios"),
	)
}
