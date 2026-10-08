package scenario

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers all scenario routes with swagger documentation
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	// GET /scenario-descriptors - List all registered scenario descriptors (including SupportsProfiles)
	router.GET("/scenario-descriptors", h.GetScenarioDescriptors,
		swagger.WithDescription("List all registered scenario descriptors"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioDescriptorsResponse{}),
	)

	// GET /scenarios - Get all scenario configurations
	router.GET("/scenarios", h.GetScenarios,
		swagger.WithDescription("Get all scenario configurations"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenariosResponse{}),
	)

	// GET /scenario/:scenario - Get configuration for a specific scenario
	router.GET("/scenario/:scenario", h.GetScenarioConfig,
		swagger.WithDescription("Get configuration for a specific scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioResponse{}),
	)

	// POST /scenario/:scenario - Create or update scenario configuration
	router.POST("/scenario/:scenario", h.SetScenarioConfig,
		swagger.WithDescription("Create or update scenario configuration"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ScenarioUpdateRequest{}),
		swagger.WithResponseModel(ScenarioUpdateResponse{}),
	)

	// GET /scenario/:scenario/flag/:flag - Get a specific flag value for a scenario
	router.GET("/scenario/:scenario/flag/:flag", h.GetScenarioFlag,
		swagger.WithDescription("Get a specific flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// PUT /scenario/:scenario/flag/:flag - Set a specific flag value for a scenario
	router.PUT("/scenario/:scenario/flag/:flag", h.SetScenarioFlag,
		swagger.WithDescription("Set a specific flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ScenarioFlagUpdateRequest{}),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// GET /scenario/:scenario/string-flag/:flag - Get a specific string flag value for a scenario
	router.GET("/scenario/:scenario/string-flag/:flag", h.GetScenarioStringFlag,
		swagger.WithDescription("Get a specific string flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// PUT /scenario/:scenario/string-flag/:flag - Set a specific string flag value for a scenario
	router.PUT("/scenario/:scenario/string-flag/:flag", h.SetScenarioStringFlag,
		swagger.WithDescription("Set a specific string flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ScenarioStringFlagUpdateRequest{}),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// GET /scenario/:scenario/int-flag/:flag - Get a specific integer flag value for a scenario
	router.GET("/scenario/:scenario/int-flag/:flag", h.GetScenarioIntFlag,
		swagger.WithDescription("Get a specific integer flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// PUT /scenario/:scenario/int-flag/:flag - Set a specific integer flag value for a scenario
	router.PUT("/scenario/:scenario/int-flag/:flag", h.SetScenarioIntFlag,
		swagger.WithDescription("Set a specific integer flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ScenarioIntFlagUpdateRequest{}),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// --- Profile endpoints ---

	// GET /scenario/:scenario/profiles - List profiles for a scenario
	router.GET("/scenario/:scenario/profiles", h.GetProfiles,
		swagger.WithDescription("List profiles for a scenario"),
		swagger.WithTags("scenarios"),
	)

	// POST /scenario/:scenario/profiles - Create a new profile
	router.POST("/scenario/:scenario/profiles", h.CreateProfile,
		swagger.WithDescription("Create a new profile for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ProfileCreateRequest{}),
	)

	// PUT /scenario/:scenario/profiles/:id - Update a profile
	router.PUT("/scenario/:scenario/profiles/:id", h.UpdateProfile,
		swagger.WithDescription("Update a profile name or mode"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ProfileUpdateRequest{}),
	)

	router.PUT("/scenario/:scenario/claude-code/slots/:slot", h.SetClaudeCodeSlot,
		swagger.WithDescription("Give a Claude Code model slot its own rule (enabled), or hand it back to the main rule"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ClaudeCodeSlotRequest{}),
		swagger.WithResponseModel(ClaudeCodeSlotResponse{}),
	)

	router.GET("/scenario/:scenario/models", h.GetClaudeCodeModels,
		swagger.WithDescription("List the model tiers Claude Code can be asked for under the main routing or a profile, each with its current route"),
		swagger.WithTags("scenarios"),
		swagger.WithQuery("profile", "string", "Claude Code profile id; empty is the main claude_code routing"),
		swagger.WithResponseModel(ClaudeCodeModelsResponse{}),
	)

	router.GET("/scenario/:scenario/profiles/:id/claude-config", h.GetProfileClaudeConfig,
		swagger.WithDescription("Get the effective and inherited Claude Code profile configuration"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ProfileClaudeConfigResponse{}),
	)

	router.PUT("/scenario/:scenario/profiles/:id/claude-config", h.UpdateProfileClaudeConfig,
		swagger.WithDescription("Persist and materialize fine-grained Claude Code profile preferences"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ProfileClaudeConfigRequest{}),
		swagger.WithResponseModel(ProfileClaudeConfigResponse{}),
	)

	router.DELETE("/scenario/:scenario/profiles/:id/claude-config", h.DeleteProfileClaudeConfig,
		swagger.WithDescription("Clear Claude Code profile overrides and restore inheritance"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ProfileClaudeConfigResponse{}),
	)

	// DELETE /scenario/:scenario/profiles/:id - Delete a profile
	router.DELETE("/scenario/:scenario/profiles/:id", h.DeleteProfile,
		swagger.WithDescription("Delete a profile"),
		swagger.WithTags("scenarios"),
	)
}
