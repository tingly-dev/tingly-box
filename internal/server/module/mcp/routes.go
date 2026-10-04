package mcp

import (
	"github.com/tingly-dev/tingly-box/internal/mcp/local"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers all MCP configuration routes with swagger documentation
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	localHandler, transportHandler := h.GetLocalHandler(), h.GetTransportHandler()
	router.POST("/mcp/sources", h.CreateSource, swagger.WithTags("mcp"), swagger.WithRequestModel(typ.MCPSourceConfig{}), swagger.WithResponseModel(MCPRuntimeConfigResponse{}))
	router.PATCH("/mcp/sources/:source_id", h.PatchSource, swagger.WithTags("mcp"), swagger.WithRequestModel(typ.MCPSourceConfig{}), swagger.WithResponseModel(MCPRuntimeConfigResponse{}))
	router.DELETE("/mcp/sources/:source_id", h.DeleteSource, swagger.WithTags("mcp"), swagger.WithResponseModel(MCPRuntimeConfigResponse{}))
	router.POST("/mcp/sources/:source_id/check", h.CheckSource, swagger.WithTags("mcp"), swagger.WithResponseModel(MCPSourceStatusResponse{}))
	router.POST("/mcp/sources/:source_id/reconnect", h.ReconnectSource, swagger.WithTags("mcp"), swagger.WithResponseModel(MCPSourceStatusResponse{}))
	router.GET("/mcp/catalog", h.GetCatalog, swagger.WithTags("mcp"), swagger.WithResponseModel(MCPCatalogResponse{}))
	router.POST("/mcp/tools/call", h.CallTool, swagger.WithTags("mcp"), swagger.WithRequestModel(MCPToolCallRequest{}), swagger.WithResponseModel(MCPToolCallResponse{}))
	router.GET("/mcp/client-profiles", h.ListClientProfiles, swagger.WithTags("mcp"), swagger.WithResponseModel(MCPClientProfilesResponse{}))
	router.PUT("/mcp/client-profiles/:profile_id", h.SaveClientProfile, swagger.WithTags("mcp"), swagger.WithRequestModel(typ.MCPClientProfile{}), swagger.WithResponseModel(MCPRuntimeConfigResponse{}))
	router.DELETE("/mcp/client-profiles/:profile_id", h.DeleteClientProfile, swagger.WithTags("mcp"), swagger.WithResponseModel(MCPRuntimeConfigResponse{}))

	router.GET("/mcp/config", h.GetMCPRuntimeConfig,
		swagger.WithDescription("Get global MCP runtime configuration"),
		swagger.WithTags("mcp"),
		swagger.WithResponseModel(MCPRuntimeConfigResponse{}),
	)

	router.PUT("/mcp/config", h.SetMCPRuntimeConfig,
		swagger.WithDescription("Set global MCP runtime configuration"),
		swagger.WithTags("mcp"),
		swagger.WithRequestModel(MCPRuntimeConfigRequest{}),
		swagger.WithResponseModel(MCPRuntimeConfigResponse{}),
	)

	router.GET("/mcp/clients", localHandler.ListClients,
		swagger.WithDescription("List all registered MCP clients"),
		swagger.WithTags("mcp"),
		swagger.WithResponseModel(local.ClientListResponse{}),
	)

	router.POST("/mcp/client", localHandler.CreateClient,
		swagger.WithDescription("Register a new MCP client"),
		swagger.WithTags("mcp"),
		swagger.WithRequestModel(local.CreateClientRequest{}),
		swagger.WithResponseModel(local.ClientResponse{}),
	)

	router.GET("/mcp/client/:id", localHandler.GetClient,
		swagger.WithDescription("Get a specific MCP client by ID"),
		swagger.WithTags("mcp"),
		swagger.WithResponseModel(local.ClientResponse{}),
	)

	router.PUT("/mcp/client/:id", localHandler.UpdateClient,
		swagger.WithDescription("Update an MCP client configuration"),
		swagger.WithTags("mcp"),
		swagger.WithRequestModel(local.UpdateClientRequest{}),
		swagger.WithResponseModel(local.ClientResponse{}),
	)

	router.DELETE("/mcp/client/:id", localHandler.DeleteClient,
		swagger.WithDescription("Remove an MCP client"),
		swagger.WithTags("mcp"),
		swagger.WithResponseModel(local.ClientResponse{}),
	)

	router.POST("/mcp/client/:id/reconnect", localHandler.ReconnectClient,
		swagger.WithDescription("Reconnect an MCP client"),
		swagger.WithTags("mcp"),
		swagger.WithResponseModel(local.ClientResponse{}),
	)

	router.GET("/mcp/install/:name", localHandler.GetInstallCommand,
		swagger.WithDescription("Get MCP install command for a client"),
		swagger.WithTags("mcp"),
		swagger.WithResponseModel(local.InstallCommandResponse{}),
	)

	// Tool execution endpoint for testing
	router.POST("/mcp/execute", localHandler.ExecuteTool,
		swagger.WithDescription("Execute an MCP tool for testing"),
		swagger.WithTags("mcp"),
		swagger.WithRequestModel(local.ExecuteToolRequest{}),
		swagger.WithResponseModel(local.ExecuteToolResponse{}),
	)

	// MCP Transport endpoints for local mode
	// These handle HTTP/SSE connections from external MCP clients
	if transportHandler != nil {
		router.POST("/mcp/:client_name", transportHandler.HandleMCP,
			swagger.WithDescription("MCP HTTP transport endpoint"),
			swagger.WithTags("mcp-transport"),
		)
		// Some MCP clients perform GET-based health checks or negotiate streamable HTTP on the base endpoint.
		// Keep GET on the same path to maximize compatibility.
		router.GET("/mcp/:client_name", transportHandler.HandleMCP,
			swagger.WithDescription("MCP HTTP transport endpoint (GET compatibility)"),
			swagger.WithTags("mcp-transport"),
		)

		router.DELETE("/mcp/:client_name", transportHandler.HandleMCP, swagger.WithTags("mcp-transport"))

		router.GET("/mcp/:client_name/stream", transportHandler.HandleMCPStream,
			swagger.WithDescription("MCP SSE transport endpoint"),
			swagger.WithTags("mcp-transport"),
		)
	}
}
