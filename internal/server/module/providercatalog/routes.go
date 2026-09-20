package providercatalog

import "github.com/tingly-dev/tingly-box/swagger"

// deprecatedPathMsg is attached to every /provider-templates alias below:
// kept for backward compatibility with existing callers, but /provider-catalog
// is the route new integrations should use.
const deprecatedPathMsg = "Use /provider-catalog instead; this alias is kept for backward compatibility."

// RegisterRoutes registers all provider catalog routes with swagger documentation.
// Each endpoint is also registered under its old /provider-templates path as a
// deprecated alias to the same handler, so existing callers keep working.
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// GET /provider-catalog - Get all provider catalog entries
	router.GET("/provider-catalog", handler.ListProviderCatalogs,
		swagger.WithDescription("Get all provider catalog entries"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProviderCatalogResponse{}),
	)
	router.GET("/provider-templates", handler.ListProviderCatalogs,
		swagger.WithDescription("Get all provider catalog entries"),
		swagger.WithTags("providers"),
		swagger.WithDeprecated(deprecatedPathMsg),
	)

	// GET /provider-catalog/:id - Get a specific provider catalog entry by ID
	router.GET("/provider-catalog/:id", handler.GetProviderCatalog,
		swagger.WithDescription("Get a specific provider catalog entry by ID"),
		swagger.WithTags("providers"),
	)
	router.GET("/provider-templates/:id", handler.GetProviderCatalog,
		swagger.WithDescription("Get a specific provider catalog entry by ID"),
		swagger.WithTags("providers"),
		swagger.WithDeprecated(deprecatedPathMsg),
	)

	// POST /provider-catalog/refresh - Refresh the provider catalog from GitHub
	router.POST("/provider-catalog/refresh", handler.RefreshProviderCatalogs,
		swagger.WithDescription("Refresh the provider catalog from GitHub"),
		swagger.WithTags("providers"),
	)
	router.POST("/provider-templates/refresh", handler.RefreshProviderCatalogs,
		swagger.WithDescription("Refresh the provider catalog from GitHub"),
		swagger.WithTags("providers"),
		swagger.WithDeprecated(deprecatedPathMsg),
	)

	// GET /provider-catalog/version - Get current provider catalog registry version
	router.GET("/provider-catalog/version", handler.GetProviderCatalogVersion,
		swagger.WithDescription("Get current provider catalog registry version"),
		swagger.WithTags("providers"),
	)
	router.GET("/provider-templates/version", handler.GetProviderCatalogVersion,
		swagger.WithDescription("Get current provider catalog registry version"),
		swagger.WithTags("providers"),
		swagger.WithDeprecated(deprecatedPathMsg),
	)
}
