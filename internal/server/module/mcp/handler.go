package mcp

import (
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

	// Create local mode handler
	localHandler := local.NewHandler(cfg, local.NewRegistry(), "")
	h.localHandler = localHandler

	// Create mcpruntime adapter for local mode
	// We need to get the config provider from the runtime
	runtime := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig {
		var mcpCfg typ.MCPRuntimeConfig
		if cfg != nil {
			cfg.GetToolConfig(db.ToolTypeMCPRuntime, &mcpCfg)
		}
		return &mcpCfg
	})
	adapter := local.NewMCPRuntimeAdapter(runtime)

	// Create transport handler for local mode
	transportHandler := local.NewTransportHandler(adapter)
	h.transportHandler = transportHandler

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
