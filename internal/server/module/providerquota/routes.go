package providerquota

import (
	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers the provider-quota API routes with swagger
// documentation.
//
// These endpoints existed before this file did, but were mounted straight on
// the gin router, so they worked at runtime and were invisible in
// openapi.json — which meant no generated client (Python, TypeScript, or
// anything else) could reach them. Declaring them here is what makes
// "how much quota does each of my accounts have left" callable from the SDK.
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	router.GET("/provider-quota", handler.ListQuota,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("List cached quota for every provider that has a quota fetcher"),
		swagger.WithResponseModel(ListQuotaResponse{}),
	)

	router.POST("/provider-quota/batch", handler.BatchGetQuota,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Fetch quota for a specific set of providers in one call"),
		swagger.WithRequestModel(BatchGetQuotaRequest{}),
		swagger.WithResponseModel(BatchGetQuotaResponse{}),
	)

	router.GET("/provider-quota/summary", handler.Summary,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Aggregate quota summary across all providers"),
		swagger.WithResponseModel(quota.Summary{}),
	)

	router.GET("/provider-quota/:uuid", handler.GetQuota,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Quota for one provider, served from cache when fresh"),
		swagger.WithResponseModel(quota.ProviderUsage{}),
	)

	router.POST("/provider-quota/refresh", handler.RefreshAll,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Force a refresh of every provider's quota from upstream"),
		swagger.WithResponseModel(ListQuotaResponse{}),
	)

	router.POST("/provider-quota/:uuid/refresh", handler.RefreshProvider,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Force a refresh of one provider's quota from upstream"),
		swagger.WithResponseModel(quota.ProviderUsage{}),
	)
}
