package guardrails

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts the guardrails admin API under /api/v1.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	rt.V1.GET("/guardrails/config", h.GetGuardrailsConfig,
		swagger.WithDescription("Get guardrails config content and parsed config"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(guardrailsConfigResponse{}),
	)
	rt.V1.GET("/guardrails/builtins", h.GetGuardrailsBuiltins,
		swagger.WithDescription("Get curated builtin guardrails policies"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(guardrailsBuiltinsResponse{}),
	)
	rt.V1.GET("/guardrails/registry", h.GetGuardrailsRegistry,
		swagger.WithDescription("List downloadable guardrails policies from a remote registry"),
		swagger.WithTags("guardrails"),
		swagger.WithQuery("refresh", "string", "Set to 1 to refresh the registry cache"),
		swagger.WithResponseModel(guardrailsRegistryResponse{}),
	)
	rt.V1.POST("/guardrails/registry/install", h.InstallGuardrailsRegistryPolicy,
		swagger.WithDescription("Download a guardrails policy from a remote registry into the local guardrails directory"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(guardrailsRegistryInstallRequest{}),
		swagger.WithResponseModel(guardrailsRegistryInstallResponse{}),
	)
	rt.V1.GET("/guardrails/credentials", h.GetGuardrailsCredentials,
		swagger.WithDescription("List protected credentials used by guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(protectedCredentialsListResponse{}),
	)
	rt.V1.GET("/guardrails/credential/:id", h.GetGuardrailsCredential,
		swagger.WithDescription("Get a protected credential for the local editor dialog"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(protectedCredentialDetailEnvelope{}),
	)
	rt.V1.POST("/guardrails/credential", h.CreateGuardrailsCredential,
		swagger.WithDescription("Create a protected credential for guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(protectedCredentialCreateRequest{}),
		swagger.WithResponseModel(protectedCredentialMutationResponse{}),
	)
	rt.V1.PUT("/guardrails/credential/:id", h.UpdateGuardrailsCredential,
		swagger.WithDescription("Update a protected credential for guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(protectedCredentialUpdateRequest{}),
		swagger.WithResponseModel(protectedCredentialMutationResponse{}),
	)
	rt.V1.DELETE("/guardrails/credential/:id", h.DeleteGuardrailsCredential,
		swagger.WithDescription("Delete a protected credential for guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(protectedCredentialDeleteResponse{}),
	)
	rt.V1.PUT("/guardrails/config", h.UpdateGuardrailsConfig,
		swagger.WithDescription("Update guardrails config and reload engine"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(guardrailsConfigUpdateRequest{}),
		swagger.WithResponseModel(guardrailsConfigUpdateResponse{}),
	)
	rt.V1.POST("/guardrails/fragment/import", h.ImportGuardrailsFragment,
		swagger.WithDescription("Import one or more guardrails policies into the shared custom fragment"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(guardrailsFragmentImportRequest{}),
		swagger.WithResponseModel(guardrailsFragmentImportResponse{}),
	)
	rt.V1.POST("/guardrails/fragment/export", h.ExportGuardrailsFragments,
		swagger.WithDescription("Export one or more imported guardrails policy fragments"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(guardrailsFragmentExportRequest{}),
		swagger.WithResponseModel(guardrailsFragmentExportResponse{}),
	)
	rt.V1.PUT("/guardrails/policy/:id", h.UpdateGuardrailsPolicy,
		swagger.WithDescription("Update a guardrails policy and reload engine"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(guardrailsPolicyUpdateRequest{}),
		swagger.WithResponseModel(guardrailsPolicyUpdateResponse{}),
	)
	rt.V1.DELETE("/guardrails/policy/:id", h.DeleteGuardrailsPolicy,
		swagger.WithDescription("Delete a guardrails policy and reload engine"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(guardrailsPolicyUpdateResponse{}),
	)
	rt.V1.POST("/guardrails/policy", h.CreateGuardrailsPolicy,
		swagger.WithDescription("Create a new guardrails policy and reload engine"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(guardrailsPolicyCreateRequest{}),
		swagger.WithResponseModel(guardrailsPolicyUpdateResponse{}),
	)
	rt.V1.PUT("/guardrails/group/:id", h.UpdateGuardrailsGroup,
		swagger.WithDescription("Update a guardrails group and reload engine"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(guardrailsGroupUpdateRequest{}),
		swagger.WithResponseModel(guardrailsGroupUpdateResponse{}),
	)
	rt.V1.DELETE("/guardrails/group/:id", h.DeleteGuardrailsGroup,
		swagger.WithDescription("Delete a guardrails group and reload engine"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(guardrailsGroupUpdateResponse{}),
	)
	rt.V1.POST("/guardrails/group", h.CreateGuardrailsGroup,
		swagger.WithDescription("Create a new guardrails group and reload engine"),
		swagger.WithTags("guardrails"),
		swagger.WithRequestModel(guardrailsGroupCreateRequest{}),
		swagger.WithResponseModel(guardrailsGroupUpdateResponse{}),
	)
	rt.V1.POST("/guardrails/reload", h.ReloadGuardrailsConfig,
		swagger.WithDescription("Reload guardrails config from disk"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(guardrailsReloadResponse{}),
	)
	rt.V1.GET("/guardrails/history", h.GetGuardrailsHistory,
		swagger.WithDescription("Get recent guardrails interception history"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(guardrailsHistoryResponse{}),
	)
	rt.V1.DELETE("/guardrails/history", h.ClearGuardrailsHistory,
		swagger.WithDescription("Clear guardrails interception history"),
		swagger.WithTags("guardrails"),
		swagger.WithResponseModel(guardrailsSuccessResponse{}),
	)
}
