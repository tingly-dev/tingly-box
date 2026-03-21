package providertemplate

import "github.com/tingly-dev/tingly-box/pkg/swagger"

// RegisterRoutes registers all provider template routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// GET /provider-templates - Get all provider templates
	router.GET("/provider-templates", handler.GetProviderTemplates,
		swagger.WithDescription("Get all provider templates"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(TemplateResponse{}),
	)

	// GET /provider-templates/:id - Get a specific provider template by ID
	router.GET("/provider-templates/:id", handler.GetProviderTemplate,
		swagger.WithDescription("Get a specific provider template by ID"),
		swagger.WithTags("providers"),
	)

	// POST /provider-templates/refresh - Refresh provider templates from GitHub
	router.POST("/provider-templates/refresh", handler.RefreshProviderTemplates,
		swagger.WithDescription("Refresh provider templates from GitHub"),
		swagger.WithTags("providers"),
	)

	// GET /provider-templates/version - Get current provider template registry version
	router.GET("/provider-templates/version", handler.GetProviderTemplateVersion,
		swagger.WithDescription("Get current provider template registry version"),
		swagger.WithTags("providers"),
	)
}
