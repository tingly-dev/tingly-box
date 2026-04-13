package mcp

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/mcp/local"
	"github.com/tingly-dev/tingly-box/internal/mcpruntime"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler handles MCP configuration HTTP requests
type Handler struct {
	cfg             *config.Config
	localHandler    *local.Handler
	transportHandler *local.TransportHandler
}

// NewHandler creates a new MCP handler
func NewHandler(cfg *config.Config) *Handler {
	h := &Handler{cfg: cfg}

	// Create registry for local mode clients
	registry := local.NewRegistry()
	h.localHandler = local.NewHandler(cfg, registry, "")

	// Create mcpruntime for local mode
	runtime := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig {
		var mcpCfg typ.MCPRuntimeConfig
		if cfg != nil {
			cfg.GetToolConfig(db.ToolTypeMCPRuntime, &mcpCfg)
		}
		return &mcpCfg
	})

	// Set runtime on local handler for tool execution
	h.localHandler.SetRuntime(runtime)

	// Get base URL from config (use localhost as fallback for auto-registration)
	baseURL := "http://localhost"
	if cfg != nil {
		baseURL = fmt.Sprintf("http://localhost:%d", cfg.GetServerPort())
	}

	// Create transport handler for local mode with registry for auto-registration
	h.transportHandler = local.NewTransportHandler(runtime, registry, baseURL, cfg)

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
	return h.cfg.GetScenarioFlag(typ.ScenarioGlobal, "mcp") ||
		h.cfg.GetScenarioFlag(typ.ScenarioClaudeCode, "mcp")
}

// MCPRuntimeConfigResponse is the API response for MCP runtime config
type MCPRuntimeConfigResponse struct {
	Success bool                       `json:"success"`
	Config  *typ.MCPRuntimeConfig      `json:"config,omitempty"`
	Error   string                     `json:"error,omitempty"`
}

// MCPRuntimeConfigRequest is the API request for setting MCP runtime config
type MCPRuntimeConfigRequest struct {
	Sources        []typ.MCPSourceConfig `json:"sources,omitempty"`
	RequestTimeout int                   `json:"request_timeout,omitempty"` // seconds, default: 30
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

	// Validate sources
	for _, source := range req.Sources {
		if source.ID == "" {
			c.JSON(http.StatusBadRequest, MCPRuntimeConfigResponse{
				Success: false,
				Error:   "MCP source ID cannot be empty",
			})
			return
		}
		if source.Transport != "" && source.Transport != "http" && source.Transport != "stdio" {
			c.JSON(http.StatusBadRequest, MCPRuntimeConfigResponse{
				Success: false,
				Error:   "Invalid transport type: " + source.Transport + ". Must be 'http' or 'stdio'",
			})
			return
		}
	}

	mcpCfg := &typ.MCPRuntimeConfig{
		Sources:        req.Sources,
		RequestTimeout: req.RequestTimeout,
	}
	typ.ApplyMCPRuntimeDefaults(mcpCfg)

	if err := h.cfg.SetToolConfig(db.ToolTypeMCPRuntime, mcpCfg); err != nil {
		c.JSON(http.StatusInternalServerError, MCPRuntimeConfigResponse{
			Success: false,
			Error:   "Failed to save MCP config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, MCPRuntimeConfigResponse{
		Success: true,
		Config:  mcpCfg,
	})
}
