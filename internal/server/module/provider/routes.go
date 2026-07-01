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

	api.GET("/provider-models/:uuid", h.GetProviderModelsByUUID,
		swagger.WithDescription("Get all provider models"),
		swagger.WithTags("models"),
		swagger.WithResponseModel(ProviderModelsResponse{}),
	)

	api.POST("/provider-models/:uuid", h.UpdateProviderModelsByUUID,
		swagger.WithDescription("Fetch models for a specific provider"),
		swagger.WithTags("models"),
		swagger.WithResponseModel(ProviderModelsResponse{}),
	)
}
