package mcp

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/mcp/local"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler handles MCP configuration HTTP requests
type Handler struct {
	cfg              *config.Config
	localHandler     *local.Handler
	transportHandler *local.TransportHandler
}

// NewHandler creates a new MCP handler.
// rt is an optional shared Runtime; when provided (e.g. from the main server) it is
// reused so the transport handler sees already-connected sources and registered builtins.
// When nil (e.g. in openapi/CLI mode) a fresh runtime is created from config.
func NewHandler(cfg *config.Config, rt ...*mcpruntime.Runtime) *Handler {
	h := &Handler{cfg: cfg}

	// Create registry for local mode clients
	registry := local.NewRegistry()
	h.localHandler = local.NewHandler(cfg, registry, "")

	// Use provided runtime or create a standalone one (no active source connections).
	var sharedRuntime *mcpruntime.Runtime
	if len(rt) > 0 && rt[0] != nil {
		sharedRuntime = rt[0]
	} else {
		sharedRuntime = mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig {
			var mcpCfg typ.MCPRuntimeConfig
			if cfg != nil {
				cfg.GetToolConfig(db.ToolTypeMCPRuntime, &mcpCfg)
			}
			return &mcpCfg
		})
	}

	h.localHandler.SetRuntime(sharedRuntime)

	// Get base URL from config (use localhost as fallback for auto-registration)
	baseURL := "http://localhost"
	if cfg != nil {
		baseURL = fmt.Sprintf("http://localhost:%d", cfg.GetServerPort())
	}

	h.transportHandler = local.NewTransportHandler(sharedRuntime, registry, baseURL, cfg)

	return h
}

// GetLocalHandler returns the local mode handler
func (h *Handler) GetLocalHandler() *local.Handler {
	return h.localHandler
}

// GetTransportHandler returns the transport handler for local mode
func (h *Handler) GetTransportHandler() *local.TransportHandler {
	return h.transportHandler
}

// IsMCPEnabled checks if MCP feature is enabled via scenario flag
func (h *Handler) IsMCPEnabled() bool {
	if h.cfg == nil {
		return false
	}
	return h.cfg.GetScenarioFlag(typ.ScenarioGlobal, config.ExtensionMCP) ||
		h.cfg.GetScenarioFlag(typ.ScenarioClaudeCode, config.ExtensionMCP)
}

// MCPRuntimeConfigResponse is the API response for MCP runtime config
type MCPRuntimeConfigResponse struct {
	Success bool                  `json:"success"`
	Config  *typ.MCPRuntimeConfig `json:"config,omitempty"`
	Error   string                `json:"error,omitempty"`
}

// MCPRuntimeConfigRequest is the API request for setting MCP runtime config
type MCPRuntimeConfigRequest struct {
	Sources               []typ.MCPSourceConfig `json:"sources,omitempty"`
	RequestTimeout        int                   `json:"request_timeout,omitempty"`          // seconds, default: 30
	StripDisabledMCPTools bool                  `json:"strip_disabled_mcp_tools,omitempty"` // dangerous: strip disabled MCP declarations/tool_calls
}

// GetMCPRuntimeConfig returns the global MCP runtime configuration
func (h *Handler) GetMCPRuntimeConfig(c *gin.Context) {
	// Check if MCP is enabled
	if !h.IsMCPEnabled() {
		c.JSON(http.StatusForbidden, MCPRuntimeConfigResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	if h.cfg == nil {
		c.JSON(http.StatusInternalServerError, MCPRuntimeConfigResponse{
			Success: false,
			Error:   "Global config not available",
		})
		return
	}

	var cfg typ.MCPRuntimeConfig
	found := h.cfg.GetToolConfig(db.ToolTypeMCPRuntime, &cfg)
	if !found {
		// Return empty config (not configured yet)
		c.JSON(http.StatusOK, MCPRuntimeConfigResponse{
			Success: true,
			Config:  &typ.MCPRuntimeConfig{},
		})
		return
	}

	typ.ApplyMCPRuntimeDefaults(&cfg)

	c.JSON(http.StatusOK, MCPRuntimeConfigResponse{
		Success: true,
		Config:  &cfg,
	})
}

// SetMCPRuntimeConfig sets the global MCP runtime configuration
func (h *Handler) SetMCPRuntimeConfig(c *gin.Context) {
	// Check if MCP is enabled
	if !h.IsMCPEnabled() {
		c.JSON(http.StatusForbidden, MCPRuntimeConfigResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	if h.cfg == nil {
		c.JSON(http.StatusInternalServerError, MCPRuntimeConfigResponse{
			Success: false,
			Error:   "Global config not available",
		})
		return
	}

	var req MCPRuntimeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MCPRuntimeConfigResponse{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate sources and apply defaults
	for i, source := range req.Sources {
		if source.ID == "" {
			c.JSON(http.StatusBadRequest, MCPRuntimeConfigResponse{
				Success: false,
				Error:   "MCP source ID cannot be empty",
			})
			return
		}
		if source.Transport != "" && source.Transport != "http" && source.Transport != "stdio" && source.Transport != "sse" && source.Transport != "advisor" {
			c.JSON(http.StatusBadRequest, MCPRuntimeConfigResponse{
				Success: false,
				Error:   "Invalid transport type: " + source.Transport + ". Must be one of 'http', 'stdio', 'sse'",
			})
			return
		}

		isBuiltin := source.ID == mcptools.BuiltinAdvisorSourceID || source.ID == mcptools.BuiltinWebtoolsSourceID
		if source.Visibility == "" {
			req.Sources[i].Visibility = typ.ToolVisibilityClient
			if isBuiltin && source.ID == mcptools.BuiltinAdvisorSourceID {
				req.Sources[i].Visibility = typ.ToolVisibilityServer
			}
		}
		if req.Sources[i].Visibility != typ.ToolVisibilityClient && req.Sources[i].Visibility != typ.ToolVisibilityServer {
			c.JSON(http.StatusBadRequest, MCPRuntimeConfigResponse{
				Success: false,
				Error:   "Invalid visibility: " + string(req.Sources[i].Visibility) + ". Must be one of 'client', 'server'",
			})
			return
		}
	}

	issues := mcpruntime.ValidateEnabledMCPSourceEnvRefs(req.Sources)
	if len(issues) > 0 {
		parts := make([]string, 0, len(issues))
		for _, issue := range issues {
			parts = append(parts, fmt.Sprintf("source=%s field=%s missing=${%s}", issue.SourceID, issue.FieldPath, issue.VarName))
		}
		c.JSON(http.StatusBadRequest, MCPRuntimeConfigResponse{
			Success: false,
			Error:   "missing environment variables for enabled MCP source(s): " + strings.Join(parts, "; "),
		})
		return
	}

	mcpCfg := &typ.MCPRuntimeConfig{
		Sources:               req.Sources,
		RequestTimeout:        req.RequestTimeout,
		StripDisabledMCPTools: req.StripDisabledMCPTools,
	}
	typ.ApplyMCPRuntimeDefaults(mcpCfg)

	if err := h.cfg.SetToolConfig(db.ToolTypeMCPRuntime, mcpCfg); err != nil {
		c.JSON(http.StatusInternalServerError, MCPRuntimeConfigResponse{
			Success: false,
			Error:   "Failed to save MCP config: " + err.Error(),
		})
		return
	}

	// Tool sources changed — reset all transport servers so the next request
	// rebuilds each with a fresh tool list.
	if h.transportHandler != nil {
		h.transportHandler.ResetAll()
	}

	c.JSON(http.StatusOK, MCPRuntimeConfigResponse{
		Success: true,
		Config:  mcpCfg,
	})
}
