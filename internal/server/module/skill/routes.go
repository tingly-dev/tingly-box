package skill

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers all skill management routes with swagger documentation
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V2
	// GET /skill-locations - Get all skill locations
	router.GET("/skill-locations", h.GetSkillLocations,
		swagger.WithDescription("Get all skill locations"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(SkillLocationsResponse{}),
	)

	// POST /skill-locations - Add a new skill location
	router.POST("/skill-locations", h.AddSkillLocation,
		swagger.WithDescription("Add a new skill location"),
		swagger.WithTags("skills"),
		swagger.WithRequestModel(AddSkillLocationRequest{}),
		swagger.WithResponseModel(AddSkillLocationResponse{}),
	)

	// GET /skill-locations/:id - Get a specific skill location
	router.GET("/skill-locations/:id", h.GetSkillLocation,
		swagger.WithDescription("Get a specific skill location"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(SkillLocationResponse{}),
	)

	// DELETE /skill-locations/:id - Remove a skill location
	router.DELETE("/skill-locations/:id", h.RemoveSkillLocation,
		swagger.WithDescription("Remove a skill location"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(RemoveSkillLocationResponse{}),
	)

	// POST /skill-locations/:id/refresh - Refresh/scan a skill location for updated skills
	router.POST("/skill-locations/:id/refresh", h.RefreshSkillLocation,
		swagger.WithDescription("Refresh/scan a skill location for updated skills"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(RefreshSkillLocationResponse{}),
	)

	// POST /skill-locations/scan - Scan all IDE locations for skills (comprehensive scan)
	router.POST("/skill-locations/scan", h.ScanIdes,
		swagger.WithDescription("Scan all IDE locations and return discovered skills"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(ScanIdesResponse{}),
	)

	// GET /skill-locations/discover - Discover IDEs with skills in home directory
	router.GET("/skill-locations/discover", h.DiscoverIdes,
		swagger.WithDescription("Discover IDEs with skills in home directory"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(DiscoverIdesResponse{}),
	)

	// POST /skill-locations/import - Import discovered skill locations
	router.POST("/skill-locations/import", h.ImportSkillLocations,
		swagger.WithDescription("Import discovered skill locations"),
		swagger.WithTags("skills"),
		swagger.WithRequestModel(ImportSkillLocationsRequest{}),
		swagger.WithResponseModel(ImportSkillLocationsResponse{}),
	)

	// GET /skill-content - Get skill file content
	router.GET("/skill-content", h.GetSkillContent,
		swagger.WithDescription("Get skill file content"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(SkillContentResponse{}),
	)
}
