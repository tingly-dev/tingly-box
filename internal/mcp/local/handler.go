package local

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler handles MCP Local mode HTTP requests
type Handler struct {
	cfg      *config.Config
	registry *Registry
	baseURL  string
	runtime  *runtime.Runtime
}

// NewHandler creates a new Local mode handler
func NewHandler(cfg *config.Config, registry *Registry, baseURL string) *Handler {
	return &Handler{
		cfg:      cfg,
		registry: registry,
		baseURL:  baseURL,
	}
}

// isMCPEnabled checks if MCP feature is enabled via scenario flag
func (h *Handler) isMCPEnabled() bool {
	if h.cfg == nil {
		return false
	}
	return h.cfg.GetScenarioFlag(typ.ScenarioGlobal, config.ExtensionMCP) ||
		h.cfg.GetScenarioFlag(typ.ScenarioClaudeCode, config.ExtensionMCP)
}

// MCPModeResponse is the API response for MCP mode
type MCPModeResponse struct {
	Success bool        `json:"success"`
	Mode    typ.MCPMode `json:"mode,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// GetMCPMode returns the current MCP runtime mode
func (h *Handler) GetMCPMode(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusForbidden, MCPModeResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	if h.cfg == nil {
		c.JSON(http.StatusInternalServerError, MCPModeResponse{
			Success: false,
			Error:   "Config not available",
		})
		return
	}

	var mcpCfg typ.MCPRuntimeConfig
	h.cfg.GetToolConfig("mcp_runtime", &mcpCfg)

	mode := mcpCfg.Mode
	if mode == "" {
		mode = typ.MCPModeClienttool // default mode
	}

	c.JSON(http.StatusOK, MCPModeResponse{
		Success: true,
		Mode:    mode,
	})
}

// SetMCPModeRequest is the API request for setting MCP mode
type SetMCPModeRequest struct {
	Mode typ.MCPMode `json:"mode"`
}

// SetMCPMode sets the MCP runtime mode
func (h *Handler) SetMCPMode(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusForbidden, MCPModeResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	if h.cfg == nil {
		c.JSON(http.StatusInternalServerError, MCPModeResponse{
			Success: false,
			Error:   "Config not available",
		})
		return
	}

	var req SetMCPModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MCPModeResponse{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate mode
	if req.Mode != typ.MCPModeServertool && req.Mode != typ.MCPModeClienttool {
		c.JSON(http.StatusBadRequest, MCPModeResponse{
			Success: false,
			Error:   "Invalid mode. Must be 'servertool' or 'clienttool'",
		})
		return
	}

	var mcpCfg typ.MCPRuntimeConfig
	h.cfg.GetToolConfig("mcp_runtime", &mcpCfg)
	mcpCfg.Mode = req.Mode

	if err := h.cfg.SetToolConfig("mcp_runtime", &mcpCfg); err != nil {
		c.JSON(http.StatusInternalServerError, MCPModeResponse{
			Success: false,
			Error:   "Failed to save config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, MCPModeResponse{
		Success: true,
		Mode:    req.Mode,
	})
}

// ClientListResponse is the API response for listing clients
type ClientListResponse struct {
	Success bool             `json:"success"`
	Clients []*typ.MCPClient `json:"clients,omitempty"`
	Error   string           `json:"error,omitempty"`
}

// ListClients returns all registered MCP clients
func (h *Handler) ListClients(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusForbidden, ClientListResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	clients := h.registry.List()
	c.JSON(http.StatusOK, ClientListResponse{
		Success: true,
		Clients: clients,
	})
}

// ClientResponse is the API response for a single client
type ClientResponse struct {
	Success bool           `json:"success"`
	Client  *typ.MCPClient `json:"client,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// GetClient returns a specific client by ID
func (h *Handler) GetClient(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusForbidden, ClientResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	id := c.Param("id")

	client, err := h.registry.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ClientResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ClientResponse{
		Success: true,
		Client:  client,
	})
}

// CreateClientRequest is the API request for creating a client
type CreateClientRequest struct {
	Name                string              `json:"name" binding:"required"`
	ConnectionType      string              `json:"connection_type" binding:"required"`
	Enabled             *bool               `json:"enabled,omitempty"`
	StdioConfig         *typ.MCPStdioConfig `json:"stdio_config,omitempty"`
	ConnectionString    string              `json:"connection_string,omitempty"`
	AuthType            string              `json:"auth_type,omitempty"`
	Headers             map[string]string   `json:"headers,omitempty"`
	AllowedExtraHeaders []string            `json:"allowed_extra_headers,omitempty"`
	OAuthConfig         *typ.MCPOAuthConfig `json:"oauth_config,omitempty"`
	ToolsToExecute      []string            `json:"tools_to_execute,omitempty"`
	ToolsAutoExec       []string            `json:"tools_to_auto_execute,omitempty"`
	IsPingAvailable     *bool               `json:"is_ping_available,omitempty"`
	ProxyURL            string              `json:"proxy_url,omitempty"`
	Env                 map[string]string   `json:"env,omitempty"`
	Args                []string            `json:"args,omitempty"`
	Cwd                 string              `json:"cwd,omitempty"`
}

// CreateClient registers a new MCP client
func (h *Handler) CreateClient(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusForbidden, ClientResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	var req CreateClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ClientResponse{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate name (no spaces, hyphens, can't start with number)
	if !isValidClientName(req.Name) {
		c.JSON(http.StatusBadRequest, ClientResponse{
			Success: false,
			Error:   "Invalid name. Must contain only ASCII characters, no hyphens or spaces, and cannot start with a number",
		})
		return
	}

	// Build config
	config := typ.MCPSourceConfig{
		Name:                req.Name,
		Enabled:             req.Enabled,
		AuthType:            typ.MCPAuthType(req.AuthType),
		AllowedExtraHeaders: req.AllowedExtraHeaders,
		StdioConfig:         req.StdioConfig,
		OAuthConfig:         req.OAuthConfig,
		ToolsToExecute:      req.ToolsToExecute,
		ToolsAutoExec:       req.ToolsAutoExec,
		IsPingAvailable:     req.IsPingAvailable,
		ProxyURL:            req.ProxyURL,
	}

	// Set connection type
	switch req.ConnectionType {
	case "stdio":
		config.ConnectionType = typ.MCPConnectionTypeSTDIO
		if req.StdioConfig != nil {
			config.Command = req.StdioConfig.Command
			config.Args = req.StdioConfig.Args
			config.Cwd = req.StdioConfig.Cwd
			config.Env = req.Env
		}
	case "sse":
		config.ConnectionType = typ.MCPConnectionTypeSSE
		config.Endpoint = req.ConnectionString
		config.Headers = req.Headers
	default:
		config.ConnectionType = typ.MCPConnectionTypeHTTP
		config.Endpoint = req.ConnectionString
		config.Headers = req.Headers
	}

	// Default auth type
	if config.AuthType == "" {
		if config.ConnectionType == typ.MCPConnectionTypeSTDIO {
			config.AuthType = typ.MCPAuthTypeNone
		} else {
			config.AuthType = typ.MCPAuthTypeHeader
		}
	}

	client, err := h.registry.Register(config)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClientResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, ClientResponse{
		Success: true,
		Client:  client,
	})
}

// UpdateClientRequest is the API request for updating a client
type UpdateClientRequest struct {
	Name                string              `json:"name,omitempty"`
	Enabled             *bool               `json:"enabled,omitempty"`
	ConnectionType      string              `json:"connection_type,omitempty"`
	StdioConfig         *typ.MCPStdioConfig `json:"stdio_config,omitempty"`
	ConnectionString    string              `json:"connection_string,omitempty"`
	AuthType            string              `json:"auth_type,omitempty"`
	Headers             map[string]string   `json:"headers,omitempty"`
	AllowedExtraHeaders []string            `json:"allowed_extra_headers,omitempty"`
	OAuthConfig         *typ.MCPOAuthConfig `json:"oauth_config,omitempty"`
	ToolsToExecute      []string            `json:"tools_to_execute,omitempty"`
	ToolsAutoExec       []string            `json:"tools_to_auto_execute,omitempty"`
	IsPingAvailable     *bool               `json:"is_ping_available,omitempty"`
	ProxyURL            string              `json:"proxy_url,omitempty"`
	Env                 map[string]string   `json:"env,omitempty"`
}

// UpdateClient updates an existing MCP client
func (h *Handler) UpdateClient(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusForbidden, ClientResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	id := c.Param("id")

	var req UpdateClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ClientResponse{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate name if provided
	if req.Name != "" && !isValidClientName(req.Name) {
		c.JSON(http.StatusBadRequest, ClientResponse{
			Success: false,
			Error:   "Invalid name. Must contain only ASCII characters, no hyphens or spaces, and cannot start with a number",
		})
		return
	}

	// Get existing client
	existing, err := h.registry.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ClientResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Build updated config
	config := existing.Config
	if req.Name != "" {
		config.Name = req.Name
	}
	if req.Enabled != nil {
		config.Enabled = req.Enabled
	}
	if req.ConnectionType != "" {
		switch req.ConnectionType {
		case "stdio":
			config.ConnectionType = typ.MCPConnectionTypeSTDIO
		case "sse":
			config.ConnectionType = typ.MCPConnectionTypeSSE
		default:
			config.ConnectionType = typ.MCPConnectionTypeHTTP
		}
	}
	if req.StdioConfig != nil {
		config.StdioConfig = req.StdioConfig
		config.Command = req.StdioConfig.Command
		config.Args = req.StdioConfig.Args
		config.Cwd = req.StdioConfig.Cwd
	}
	if req.ConnectionString != "" {
		config.Endpoint = req.ConnectionString
	}
	if req.AuthType != "" {
		config.AuthType = typ.MCPAuthType(req.AuthType)
	}
	if req.Headers != nil {
		config.Headers = req.Headers
	}
	if req.AllowedExtraHeaders != nil {
		config.AllowedExtraHeaders = req.AllowedExtraHeaders
	}
	if req.OAuthConfig != nil {
		config.OAuthConfig = req.OAuthConfig
	}
	if req.ToolsToExecute != nil {
		config.ToolsToExecute = req.ToolsToExecute
	}
	if req.ToolsAutoExec != nil {
		config.ToolsAutoExec = req.ToolsAutoExec
	}
	if req.IsPingAvailable != nil {
		config.IsPingAvailable = req.IsPingAvailable
	}
	if req.ProxyURL != "" {
		config.ProxyURL = req.ProxyURL
	}
	if req.Env != nil {
		config.Env = req.Env
	}

	client, err := h.registry.Update(id, config)
	if err != nil {
		c.JSON(http.StatusBadRequest, ClientResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ClientResponse{
		Success: true,
		Client:  client,
	})
}

// DeleteClient removes an MCP client
func (h *Handler) DeleteClient(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusForbidden, ClientResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	id := c.Param("id")

	if err := h.registry.Unregister(id); err != nil {
		c.JSON(http.StatusNotFound, ClientResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ClientResponse{
		Success: true,
	})
}

// ReconnectClient triggers a reconnection for a client
func (h *Handler) ReconnectClient(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusForbidden, ClientResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	id := c.Param("id")

	client, err := h.registry.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ClientResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Update state to connecting
	h.registry.UpdateState(id, typ.MCPClientStateConnecting)

	// TODO: Implement actual reconnection logic with transport

	c.JSON(http.StatusOK, ClientResponse{
		Success: true,
		Client:  client,
	})
}

// InstallCommandResponse is the API response for install command
type InstallCommandResponse struct {
	Success        bool   `json:"success"`
	InstallCommand string `json:"install_command,omitempty"`
	Error          string `json:"error,omitempty"`
}

// GetInstallCommand returns the MCP install command for a client
func (h *Handler) GetInstallCommand(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusNotFound, InstallCommandResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	name := c.Param("name")

	client, err := h.registry.GetByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, InstallCommandResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Generate install command based on transport type
	var installCmd string
	baseURL := h.baseURL
	if baseURL == "" {
		// Get actual server port from config
		port := 12580 // default fallback
		if h.cfg != nil {
			port = h.cfg.GetServerPort()
		}
		baseURL = fmt.Sprintf("http://localhost:%d", port)
	}

	switch client.Config.ConnectionType {
	case typ.MCPConnectionTypeSSE:
		installCmd = fmt.Sprintf(`mcp install %s --url "%s/mcp/%s/stream"`, name, baseURL, name)
	case typ.MCPConnectionTypeHTTP, typ.MCPConnectionTypeSTDIO:
		fallthrough
	default:
		installCmd = fmt.Sprintf(`mcp install %s --url "%s/mcp/%s"`, name, baseURL, name)
	}

	c.JSON(http.StatusOK, InstallCommandResponse{
		Success:        true,
		InstallCommand: installCmd,
	})
}

// ExecuteToolRequest is the API request for executing a tool
type ExecuteToolRequest struct {
	ClientID  string                 `json:"client_id" binding:"required"`
	ToolName  string                 `json:"tool_name" binding:"required"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ExecuteToolResponse is the API response for tool execution
type ExecuteToolResponse struct {
	Success       bool   `json:"success"`
	Result        string `json:"result,omitempty"`
	Error         string `json:"error,omitempty"`
	ExecutionTime int64  `json:"execution_time,omitempty"` // milliseconds
}

// ExecuteTool executes an MCP tool for testing purposes
func (h *Handler) ExecuteTool(c *gin.Context) {
	// Check if MCP is enabled
	if !h.isMCPEnabled() {
		c.JSON(http.StatusForbidden, ExecuteToolResponse{
			Success: false,
			Error:   "MCP feature is disabled",
		})
		return
	}

	if h.runtime == nil {
		c.JSON(http.StatusServiceUnavailable, ExecuteToolResponse{
			Success: false,
			Error:   "MCP runtime not available",
		})
		return
	}

	var req ExecuteToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ExecuteToolResponse{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Get client to verify it exists and find the source
	client, err := h.registry.Get(req.ClientID)
	if err != nil {
		c.JSON(http.StatusNotFound, ExecuteToolResponse{
			Success: false,
			Error:   "Client not found: " + req.ClientID,
		})
		return
	}

	// Check if client is enabled
	if client.Config.Enabled != nil && !*client.Config.Enabled {
		c.JSON(http.StatusBadRequest, ExecuteToolResponse{
			Success: false,
			Error:   "Client is disabled",
		})
		return
	}

	// Check if tool is allowed for this client
	toolAllowed := false
	allowedTools := client.Config.ToolsToExecute
	if len(allowedTools) == 0 {
		// If no specific tools configured, allow all
		toolAllowed = true
	} else {
		for _, tool := range allowedTools {
			if tool == "*" || tool == req.ToolName {
				toolAllowed = true
				break
			}
		}
	}

	if !toolAllowed {
		c.JSON(http.StatusForbidden, ExecuteToolResponse{
			Success: false,
			Error:   "Tool not allowed for this client",
		})
		return
	}

	// Execute the tool using the runtime
	startTime := time.Now()

	// Build the normalized tool name: tingly_box_mcp__<source_id>__<tool_name>
	normalizedToolName := runtime.NormalizeToolName(client.Config.ID, req.ToolName)

	// Serialize arguments
	argsJSON, err := json.Marshal(req.Arguments)
	if err != nil {
		c.JSON(http.StatusBadRequest, ExecuteToolResponse{
			Success: false,
			Error:   "Invalid arguments: " + err.Error(),
		})
		return
	}

	// Call the tool via runtime
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	result, err := h.runtime.CallTool(ctx, normalizedToolName, string(argsJSON))
	executionTime := time.Since(startTime).Milliseconds()

	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"client_id": client.Config.ID,
			"tool_name": req.ToolName,
		}).Warn("Tool execution failed")

		c.JSON(http.StatusOK, ExecuteToolResponse{
			Success:       false,
			Error:         err.Error(),
			ExecutionTime: executionTime,
		})
		return
	}

	c.JSON(http.StatusOK, ExecuteToolResponse{
		Success:       true,
		Result:        result.FirstText(),
		ExecutionTime: executionTime,
	})
}

// SetRuntime sets the MCP runtime for tool execution
func (h *Handler) SetRuntime(runtime *runtime.Runtime) {
	h.runtime = runtime
}

// isValidClientName checks if a client name is valid
func isValidClientName(name string) bool {
	if len(name) == 0 {
		return false
	}

	// Check for invalid characters
	for _, c := range name {
		if c == '-' || c == ' ' || c == '\t' {
			return false
		}
	}

	// Can't start with number
	if name[0] >= '0' && name[0] <= '9' {
		return false
	}

	return true
}
