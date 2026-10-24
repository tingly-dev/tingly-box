package mcp

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/mcp/local"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler handles MCP configuration HTTP requests
type Handler struct {
	cfg              *config.Config
	runtime          *mcpruntime.Runtime
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
				cfg.GetToolConfig(config.ToolTypeMCPRuntime, &mcpCfg)
			}
			return &mcpCfg
		})
	}

	h.runtime = sharedRuntime
	h.localHandler.SetRuntime(sharedRuntime)

	// Get base URL from config (use localhost as fallback for auto-registration)
	baseURL := "http://localhost"
	if cfg != nil {
		baseURL = fmt.Sprintf("http://localhost:%d", cfg.GetServerPort())
	}

	h.transportHandler = local.NewTransportHandler(sharedRuntime, registry, baseURL, cfg)
	h.localHandler.SetReconnect(h.transportHandler.ReconnectClient)

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
	return h.cfg.GetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionMCP) ||
		h.cfg.GetScenarioFlag(typ.ScenarioClaudeCode, constant.ExtensionMCP)
}

// MCPRuntimeConfigResponse is the API response for MCP runtime config
type MCPRuntimeConfigResponse struct {
	Success bool                  `json:"success"`
	Enabled bool                  `json:"enabled"`
	Config  *typ.MCPRuntimeConfig `json:"config,omitempty"`
	Error   string                `json:"error,omitempty"`
}

// MCPRuntimeConfigRequest is the API request for setting MCP runtime config
type MCPRuntimeConfigRequest struct {
	Sources               []typ.MCPSourceConfig `json:"sources,omitempty"`
	RequestTimeout        *int                  `json:"request_timeout,omitempty"`          // seconds, default: 30
	StripDisabledMCPTools *bool                 `json:"strip_disabled_mcp_tools,omitempty"` // dangerous: strip disabled MCP declarations/tool_calls
}

// GetMCPRuntimeConfig returns the global MCP runtime configuration
func (h *Handler) GetMCPRuntimeConfig(c *gin.Context) {
	if h.cfg == nil {
		c.JSON(http.StatusInternalServerError, MCPRuntimeConfigResponse{
			Success: false,
			Error:   "Global config not available",
		})
		return
	}

	var cfg typ.MCPRuntimeConfig
	found := h.cfg.GetToolConfig(config.ToolTypeMCPRuntime, &cfg)
	if !found {
		// Return empty config (not configured yet)
		c.JSON(http.StatusOK, MCPRuntimeConfigResponse{
			Success: true,
			Config:  &typ.MCPRuntimeConfig{RequestTimeout: 30},
			Enabled: h.IsMCPEnabled(),
		})
		return
	}

	typ.ApplyMCPRuntimeDefaults(&cfg)

	c.JSON(http.StatusOK, MCPRuntimeConfigResponse{
		Success: true,
		Config:  &cfg,
		Enabled: h.IsMCPEnabled(),
	})
}

// SetMCPRuntimeConfig sets the global MCP runtime configuration
func (h *Handler) SetMCPRuntimeConfig(c *gin.Context) {
	if h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, MCPRuntimeConfigResponse{Error: "Global config not available"})
		return
	}
	var req MCPRuntimeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MCPRuntimeConfigResponse{Error: err.Error()})
		return
	}
	next, err := h.cfg.UpdateMCPRuntimeConfig(func(cfg *typ.MCPRuntimeConfig) error {
		if req.Sources != nil {
			cfg.Sources = req.Sources
		}
		if req.RequestTimeout != nil {
			cfg.RequestTimeout = *req.RequestTimeout
		}
		if req.StripDisabledMCPTools != nil {
			cfg.StripDisabledMCPTools = *req.StripDisabledMCPTools
		}
		return validateConfig(cfg)
	})
	if err != nil {
		status := http.StatusInternalServerError
		var invalid *validationError
		if errors.As(err, &invalid) {
			status = http.StatusBadRequest
		}
		c.JSON(status, MCPRuntimeConfigResponse{Error: err.Error()})
		return
	}
	h.configChanged(c)
	c.JSON(http.StatusOK, MCPRuntimeConfigResponse{Success: true, Config: next, Enabled: h.IsMCPEnabled()})
}
