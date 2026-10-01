package providerquota

import (
	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers the provider-quota API routes with swagger
// documentation, mirroring how every other module under internal/server/module
// registers into apiV1. Registered unconditionally (regardless of whether a
// quota manager is actually configured) so the routes — and their response
// models — always appear in openapi.json and every generated client;
// Handler.available() answers 503 at request time when there is no manager.
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	router.GET("/provider-quota", h.ListQuota,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("List cached quota for every provider that has a quota fetcher"),
		swagger.WithResponseModel(ListQuotaResponse{}),
	)

	router.POST("/provider-quota/batch", h.BatchGetQuota,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Fetch quota for a specific set of providers in one call"),
		swagger.WithRequestModel(BatchGetQuotaRequest{}),
		swagger.WithResponseModel(BatchGetQuotaResponse{}),
	)

	router.GET("/provider-quota/summary", h.Summary,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Aggregate quota summary across all providers"),
		swagger.WithResponseModel(quota.Summary{}),
	)

	router.GET("/provider-quota/history", h.QuotaHistory,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("List today's quota samples and past days' extrema for dashboard trends"),
		swagger.WithRequestModel(HistoryRequest{}),
		swagger.WithResponseModel(ListQuotaResponse{}),
	)

	router.GET("/provider-quota/:uuid", h.GetQuota,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Quota for one provider, served from cache when fresh"),
		swagger.WithResponseModel(quota.ProviderUsage{}),
	)

	router.POST("/provider-quota/refresh", h.RefreshAll,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Force a refresh of every provider's quota from upstream"),
		swagger.WithResponseModel(ListQuotaResponse{}),
	)

	router.POST("/provider-quota/:uuid/refresh", h.RefreshProvider,
		swagger.WithTags("provider-quota"),
		swagger.WithDescription("Force a refresh of one provider's quota from upstream"),
		swagger.WithResponseModel(quota.ProviderUsage{}),
	)
}
