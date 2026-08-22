package provider

import (
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes wires all provider endpoints onto the given route group.
func RegisterRoutes(api *swagger.RouteGroup, h *Handler) {
	api.GET("/providers", h.GetProviders,
		swagger.WithDescription("Get all configured providers with masked tokens"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProvidersResponse{}),
	)

	api.GET("/providers/:uuid", h.GetProvider,
		swagger.WithDescription("Get specific provider details with masked token"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProviderResponse{}),
	)

	api.POST("/providers", h.CreateProvider,
		swagger.WithDescription("Create a new provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithQuery("force", "bool", "Force to add without checking"),
		swagger.WithRequestModel(CreateProviderRequest{}),
		swagger.WithResponseModel(CreateProviderResponse{}),
	)

	api.PUT("/providers/:uuid", h.UpdateProvider,
		swagger.WithDescription("Update existing provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithRequestModel(UpdateProviderRequest{}),
		swagger.WithResponseModel(UpdateProviderResponse{}),
	)

	api.POST("/providers/:uuid/toggle", h.ToggleProvider,
		swagger.WithDescription("Toggle provider enabled/disabled status"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ToggleProviderResponse{}),
	)

	api.DELETE("/providers/:uuid", h.DeleteProvider,
		swagger.WithDescription("Delete a provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(DeleteProviderResponse{}),
	)

	// GET /provider/flags/registry - Catalog of supported provider/model-level
	// flags, mirroring GET /rule/flags/registry so the frontend renders the
	// provider Plugins UI registry-driven.
	api.GET("/provider/flags/registry", h.GetFlagRegistry,
		swagger.WithDescription("Get catalog of supported provider/model-level flags"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(FlagRegistryResponse{}),
	)

	api.GET("/provider-models/:uuid", h.GetProviderModelsByUUID,
		swagger.WithDescription("Get all provider models"),
		swagger.WithTags("models"),
		swagger.WithResponseModel(ProviderModelsResponse{}),
	)

	api.POST("/provider-models/:uuid", h.UpdateProviderModelsByUUID,
		swagger.WithDescription("Fetch models for a specific provider"),
		swagger.WithTags("models"),
		swagger.WithQuery("force_upstream", "bool", "Attempt the upstream models endpoint even when normally skipped"),
		swagger.WithResponseModel(ProviderModelsResponse{}),
	)

	// POST /provider-import - Import providers from base64/JSONL encoded
	// export data. Renamed from the legacy /rule/import path now that
	// dataio export/import is provider-only (this endpoint no longer
	// touches rules).
	api.POST("/provider-import", h.ImportProviders,
		swagger.WithDescription("Import providers from base64/JSONL encoded export data"),
		swagger.WithTags("providers"),
		swagger.WithRequestModel(ImportProvidersRequest{}),
		swagger.WithResponseModel(ImportProvidersResponse{}),
	)

	// GET /provider-export - Export a single provider (uuid required) as
	// base64/JSONL encoded data. Registered alongside the import route for
	// path symmetry.
	api.GET("/provider-export", h.ExportProvider,
		swagger.WithDescription("Export a single provider as base64/JSONL encoded data"),
		swagger.WithTags("providers"),
		swagger.WithQueryRequired("uuid", "string", "UUID of the provider to export"),
		swagger.WithQuery("format", "string", "Export format: base64 (default) or jsonl"),
		swagger.WithResponseModel(ExportProviderResponse{}),
	)
}
