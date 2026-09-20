package providercatalog

import "github.com/tingly-dev/tingly-box/swagger"

// RegisterRoutes registers all provider catalog routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// GET /provider-catalog - Get all provider catalog entries
	router.GET("/provider-catalog", handler.ListProviderCatalogs,
		swagger.WithDescription("Get all provider catalog entries"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProviderCatalogResponse{}),
	)

	// GET /provider-catalog/:id - Get a specific provider catalog entry by ID
	router.GET("/provider-catalog/:id", handler.GetProviderCatalog,
		swagger.WithDescription("Get a specific provider catalog entry by ID"),
		swagger.WithTags("providers"),
	)

	// POST /provider-catalog/refresh - Refresh the provider catalog from GitHub
	router.POST("/provider-catalog/refresh", handler.RefreshProviderCatalogs,
		swagger.WithDescription("Refresh the provider catalog from GitHub"),
		swagger.WithTags("providers"),
	)

	// GET /provider-catalog/version - Get current provider catalog registry version
	router.GET("/provider-catalog/version", handler.GetProviderCatalogVersion,
		swagger.WithDescription("Get current provider catalog registry version"),
		swagger.WithTags("providers"),
	)
}
