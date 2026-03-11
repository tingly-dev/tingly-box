package skill

import (
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers all skill management routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// GET /skill-locations - Get all skill locations
	router.GET("/skill-locations", handler.GetSkillLocations,
		swagger.WithDescription("Get all skill locations"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(SkillLocationsResponse{}),
	)

	// POST /skill-locations - Add a new skill location
	router.POST("/skill-locations", handler.AddSkillLocation,
		swagger.WithDescription("Add a new skill location"),
		swagger.WithTags("skills"),
		swagger.WithRequestModel(AddSkillLocationRequest{}),
		swagger.WithResponseModel(AddSkillLocationResponse{}),
	)

	// GET /skill-locations/:id - Get a specific skill location
	router.GET("/skill-locations/:id", handler.GetSkillLocation,
		swagger.WithDescription("Get a specific skill location"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(SkillLocationResponse{}),
	)

	// DELETE /skill-locations/:id - Remove a skill location
	router.DELETE("/skill-locations/:id", handler.RemoveSkillLocation,
		swagger.WithDescription("Remove a skill location"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(RemoveSkillLocationResponse{}),
	)

	// POST /skill-locations/:id/refresh - Refresh/scan a skill location for updated skills
	router.POST("/skill-locations/:id/refresh", handler.RefreshSkillLocation,
		swagger.WithDescription("Refresh/scan a skill location for updated skills"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(RefreshSkillLocationResponse{}),
	)

	// POST /skill-locations/scan - Scan all IDE locations for skills (comprehensive scan)
	router.POST("/skill-locations/scan", handler.ScanIdes,
		swagger.WithDescription("Scan all IDE locations and return discovered skills"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(ScanIdesResponse{}),
	)

	// GET /skill-locations/discover - Discover IDEs with skills in home directory
	router.GET("/skill-locations/discover", handler.DiscoverIdes,
		swagger.WithDescription("Discover IDEs with skills in home directory"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(DiscoverIdesResponse{}),
	)

	// POST /skill-locations/import - Import discovered skill locations
	router.POST("/skill-locations/import", handler.ImportSkillLocations,
		swagger.WithDescription("Import discovered skill locations"),
		swagger.WithTags("skills"),
		swagger.WithRequestModel(ImportSkillLocationsRequest{}),
		swagger.WithResponseModel(ImportSkillLocationsResponse{}),
	)

	// GET /skill-content - Get skill file content
	router.GET("/skill-content", handler.GetSkillContent,
		swagger.WithDescription("Get skill file content"),
		swagger.WithTags("skills"),
		swagger.WithResponseModel(SkillContentResponse{}),
	)
}
