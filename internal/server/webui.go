package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"tingly-box/internal/config"
	"tingly-box/internal/memory"

	"github.com/gin-gonic/gin"
)

// GlobalServerManager manages the global server instance for web UI control
var (
	globalServer     *Server
	globalServerLock sync.RWMutex
	shutdownChan     = make(chan struct{}, 1)
)

// SetGlobalServer sets the global server instance for web UI control
func SetGlobalServer(server *Server) {
	globalServerLock.Lock()
	defer globalServerLock.Unlock()
	globalServer = server
}

// GetGlobalServer gets the global server instance
func GetGlobalServer() *Server {
	globalServerLock.RLock()
	defer globalServerLock.RUnlock()
	return globalServer
}

// Init sets up Server routes and templates on the main server router
func (s *Server) UseUIEndpoints() {

	// Middleware
	s.router.Use(gin.Logger())
	s.router.Use(gin.Recovery())

	s.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
	})

	// Dashboard endpoints

	// UI page routes
	s.router.GET("/dashboard", s.UseIndex)
	s.router.GET("/providers", s.UseIndex)
	s.router.GET("/system", s.UseIndex)
	s.router.GET("/history", s.UseIndex)

	// API routes (for web UI functionality)
	s.useWebAPIEndpoints(s.router)

	// Static files and templates - try embedded assets first, fallback to filesystem
	s.useWebStaticEndpoints(s.router)
}

// ProbeRule tests a rule configuration by sending a sample request to the configured provider
func (s *Server) ProbeRule(c *gin.Context) {

	var rule config.Rule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Get the first rule or create a default one for testing
	globalConfig := s.config
	if globalConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "CONFIG_UNAVAILABLE",
				"message": "Global config not available",
			},
		})
		return
	}

	// Find the provider for this rule
	providers := s.config.ListProviders()
	var testProvider *config.Provider

	for _, provider := range providers {
		if provider.Enabled && provider.Name == rule.GetDefaultProvider() {
			testProvider = provider
			break
		}
	}

	probe(c, rule, testProvider)
}

func (s *Server) UseIndex(c *gin.Context) {
	if s.assets != nil {
		s.assets.HTML(c, "index.html", nil)
	} else {
		panic("No UI resources")
	}
}

// API Handlers (exported for server integration)
func (s *Server) GetProviders(c *gin.Context) {
	providers := s.config.ListProviders()

	// Mask tokens for security
	maskedProviders := make([]struct {
		Name     string `json:"name"`
		APIBase  string `json:"api_base"`
		APIStyle string `json:"api_style"`
		Token    string `json:"token"`
		Enabled  bool   `json:"enabled"`
	}, len(providers))

	for i, provider := range providers {
		maskedProviders[i] = struct {
			Name     string `json:"name"`
			APIBase  string `json:"api_base"`
			APIStyle string `json:"api_style"`
			Token    string `json:"token"`
			Enabled  bool   `json:"enabled"`
		}{
			Name:     provider.Name,
			APIBase:  provider.APIBase,
			APIStyle: string(provider.APIStyle),
			Token:    maskToken(provider.Token),
			Enabled:  provider.Enabled,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    maskedProviders,
	})
}

func (s *Server) GetStatus(c *gin.Context) {
	providers := s.config.ListProviders()
	enabledCount := 0
	for _, p := range providers {
		if p.Enabled {
			enabledCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"server_running":    true,
			"port":              s.config.GetServerPort(),
			"providers_total":   len(providers),
			"providers_enabled": enabledCount,
			"request_count":     0,
		},
	})
}

func (s *Server) GetHistory(c *gin.Context) {
	if s.logger != nil {
		history := s.logger.GetHistory(50)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    history,
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    []interface{}{},
		})
	}
}

func (s *Server) GetDefaults(c *gin.Context) {
	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	requestConfigs := cfg.GetRequestConfigs()
	defaultRequestID := cfg.GetDefaultRequestID()

	// Convert Rules to response format
	responseConfigs := make([]map[string]interface{}, len(requestConfigs))
	for i, rc := range requestConfigs {
		responseConfigs[i] = map[string]interface{}{
			"request_model":  rc.RequestModel,
			"response_model": rc.ResponseModel,
			"provider":       rc.GetDefaultProvider(),
			"default_model":  rc.GetDefaultModel(),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"request_configs":    responseConfigs,
			"default_request_id": defaultRequestID,
		},
	})
}

// GetRules returns all rules
func (s *Server) GetRules(c *gin.Context) {
	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	rules := cfg.GetRequestConfigs()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rules,
	})
}

// GetRule returns a specific rule by name
func (s *Server) GetRule(c *gin.Context) {
	ruleName := c.Param("name")
	if ruleName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Rule name is required",
		})
		return
	}

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	rule := cfg.GetRequestConfigByRequestModel(ruleName)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Rule not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rule,
	})
}

// SetRule creates or updates a rule
func (s *Server) SetRule(c *gin.Context) {
	ruleName := c.Param("name")
	if ruleName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Rule name is required",
		})
		return
	}

	var rule config.Rule

	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	if err := cfg.SetDefaultRequestConfig(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save rule: " + err.Error(),
		})
		return
	}

	// Log the action
	if s.logger != nil {
		s.logger.LogAction(memory.ActionUpdateProvider, map[string]interface{}{
			"name": ruleName,
		}, true, fmt.Sprintf("Rule %s updated successfully", ruleName))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Rule saved successfully",
		"data": map[string]interface{}{
			"request_model":  rule.RequestModel,
			"response_model": rule.ResponseModel,
			"provider":       rule.GetDefaultProvider(),
			"default_model":  rule.GetDefaultModel(),
			"active":         rule.Active,
		},
	})
}

// AddProvider adds a new provider
func (s *Server) AddProvider(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		APIBase  string `json:"api_base" binding:"required"`
		APIStyle string `json:"api_style"`
		Token    string `json:"token" binding:"required"`
		Enabled  bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Set default enabled status if not provided
	if !req.Enabled {
		req.Enabled = true
	}

	// Set default API style if not provided
	if req.APIStyle == "" {
		req.APIStyle = "openai"
	}

	provider := &config.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: config.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  req.Enabled,
	}

	err := s.config.AddProvider(provider)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(memory.ActionAddProvider, map[string]interface{}{
				"name":     req.Name,
				"api_base": req.APIBase,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if s.logger != nil {
		s.logger.LogAction(memory.ActionAddProvider, map[string]interface{}{
			"name":     req.Name,
			"api_base": req.APIBase,
		}, true, fmt.Sprintf("Provider %s added successfully", req.Name))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider added successfully",
		"data":    provider,
	})
}

// DeleteProvider removes a provider
func (s *Server) DeleteProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	err := s.config.DeleteProvider(providerName)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(memory.ActionDeleteProvider, map[string]interface{}{
				"name": providerName,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if s.logger != nil {
		s.logger.LogAction(memory.ActionDeleteProvider, map[string]interface{}{
			"name": providerName,
		}, true, fmt.Sprintf("Provider %s deleted successfully", providerName))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider deleted successfully",
	})
}

// UpdateProvider updates an existing provider
func (s *Server) UpdateProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	var req struct {
		NewName  *string `json:"name,omitempty"`
		APIBase  *string `json:"api_base,omitempty"`
		APIStyle *string `json:"api_style,omitempty"`
		Token    *string `json:"token,omitempty"`
		Enabled  *bool   `json:"enabled,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Get existing provider
	provider, err := s.config.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Provider not found",
		})
		return
	}

	// Update fields if provided
	if req.NewName != nil {
		provider.Name = *req.NewName
	}
	if req.APIBase != nil {
		provider.APIBase = *req.APIBase
	}
	if req.APIStyle != nil {
		provider.APIStyle = config.APIStyle(*req.APIStyle)
	}
	// Only update token if it's provided and not empty
	if req.Token != nil && *req.Token != "" {
		provider.Token = *req.Token
	}
	if req.Enabled != nil {
		provider.Enabled = *req.Enabled
	}

	err = s.config.UpdateProvider(providerName, provider)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(memory.ActionUpdateProvider, map[string]interface{}{
				"name":    providerName,
				"updates": req,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if s.logger != nil {
		s.logger.LogAction(memory.ActionUpdateProvider, map[string]interface{}{
			"name": providerName,
		}, true, fmt.Sprintf("Provider %s updated successfully", providerName))
	}

	// Return masked provider data
	responseProvider := struct {
		Name     string `json:"name"`
		APIBase  string `json:"api_base"`
		APIStyle string `json:"api_style"`
		Token    string `json:"token"`
		Enabled  bool   `json:"enabled"`
	}{
		Name:     provider.Name,
		APIBase:  provider.APIBase,
		APIStyle: string(provider.APIStyle),
		Token:    maskToken(provider.Token),
		Enabled:  provider.Enabled,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider updated successfully",
		"data":    responseProvider,
	})
}

// GetProvider returns details for a specific provider (with masked token)
func (s *Server) GetProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	provider, err := s.config.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Provider not found",
		})
		return
	}

	// Mask the token for security
	maskedToken := maskToken(provider.Token)

	responseProvider := struct {
		Name     string `json:"name"`
		APIBase  string `json:"api_base"`
		APIStyle string `json:"api_style"`
		Token    string `json:"token"`
		Enabled  bool   `json:"enabled"`
	}{
		Name:     provider.Name,
		APIBase:  provider.APIBase,
		APIStyle: string(provider.APIStyle),
		Token:    maskedToken,
		Enabled:  provider.Enabled,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    responseProvider,
	})
}

// ToggleProvider enables/disables a provider
func (s *Server) ToggleProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	provider, err := s.config.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Provider not found",
		})
		return
	}

	// Toggle enabled status
	provider.Enabled = !provider.Enabled

	err = s.config.UpdateProvider(providerName, provider)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(memory.ActionUpdateProvider, map[string]interface{}{
				"name":    providerName,
				"enabled": provider.Enabled,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	action := "disabled"
	if provider.Enabled {
		action = "enabled"
	}

	if s.logger != nil {
		s.logger.LogAction(memory.ActionUpdateProvider, map[string]interface{}{
			"name":    providerName,
			"enabled": provider.Enabled,
		}, true, fmt.Sprintf("Provider %s %s successfully", providerName, action))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Provider %s %s successfully", providerName, action),
		"data": gin.H{
			"enabled": provider.Enabled,
		},
	})
}

// Helper function to mask tokens for display
func maskToken(token string) string {
	if token == "" {
		return ""
	}

	// If already masked, return as is
	if strings.Contains(token, "*") {
		return token
	}

	// For very short tokens, mask all characters
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}

	// For longer tokens, show first 4 and last 4 characters
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

func (s *Server) StartServer(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Start server via web UI not supported. Please use CLI: tingly start",
	})
}

func (s *Server) StopServer(c *gin.Context) {
	// Get the global server instance
	server := GetGlobalServer()
	if server == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "No server instance available to stop",
		})
		return
	}

	// Stop the server gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to stop server: %v", err),
		})
		return
	}

	// Log the action
	if s.logger != nil {
		s.logger.LogAction(memory.ActionStopServer, map[string]interface{}{
			"source": "web_ui",
		}, true, "Server stopped via web interface")
	}

	// Send shutdown signal to main process
	select {
	case shutdownChan <- struct{}{}:
	default:
		// Channel already has a signal
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Server stopped successfully. The application will now exit.",
	})
}

func (s *Server) RestartServer(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Restart server via web UI not supported. Please use CLI: tingly restart",
	})
}

func (s *Server) SetDefaults(c *gin.Context) {
	var req struct {
		RequestConfigs []config.Rule `json:"request_configs"`
	}

	// Body
	//bodyBytes, _ := c.GetRawData()
	//println(string(bodyBytes))

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	// Update Rules if provided
	if req.RequestConfigs != nil {
		if err := cfg.SetRequestConfigs(req.RequestConfigs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   fmt.Sprintf("Failed to update request configs: %v", err),
			})
			return
		}
	}

	if s.logger != nil {
		logData := map[string]interface{}{
			"request_configs_count": 0,
		}

		if req.RequestConfigs != nil {
			logData["request_configs_count"] = len(req.RequestConfigs)
		}

		s.logger.LogAction(memory.ActionUpdateDefaults, logData, true, "Request configs updated via web interface")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Request configs updated successfully",
	})
}

func (s *Server) FetchProviderModels(c *gin.Context) {
	providerName := c.Param("name")

	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	// Fetch and save models
	err := s.config.FetchAndSaveProviderModels(providerName)
	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(memory.ActionFetchModels, map[string]interface{}{
				"provider": providerName,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Get the updated models
	modelManager := s.config.GetModelManager()
	models := modelManager.GetModels(providerName)

	if s.logger != nil {
		s.logger.LogAction(memory.ActionFetchModels, map[string]interface{}{
			"provider":     providerName,
			"models_count": len(models),
		}, true, fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), providerName))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), providerName),
		"data":    models,
	})
}

func (s *Server) GetProviderModels(c *gin.Context) {
	providerModelManager := s.config.GetModelManager()
	if providerModelManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Provider model manager not available",
		})
		return
	}

	providers := providerModelManager.GetAllProviders()
	providerModels := make(map[string]interface{})

	for _, providerName := range providers {
		models := providerModelManager.GetModels(providerName)
		apiBase, lastUpdated, _ := providerModelManager.GetProviderInfo(providerName)

		providerModels[providerName] = map[string]interface{}{
			"models":       models,
			"api_base":     apiBase,
			"last_updated": lastUpdated,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    providerModels,
	})
}

// authMiddleware validates the authentication token
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the auth token from global config
		cfg := s.config
		if cfg == nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Global config not available",
			})
			c.Abort()
			return
		}

		expectedToken := cfg.GetUserToken()
		if expectedToken == "" {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "User auth token not configured",
			})
			c.Abort()
			return
		}

		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization header required",
			})
			c.Abort()
			return
		}

		// Support both "Bearer token" and just "token" formats
		token := strings.TrimPrefix(authHeader, "Bearer ")
		token = strings.TrimSpace(token)

		if token != expectedToken {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid authentication token",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// useWebAPIEndpoints configures API routes for web UI
func (s *Server) useWebAPIEndpoints(engine *gin.Engine) {
	api := engine.Group("/api")
	api.Use(s.authMiddleware()) // Apply authentication to all API routes
	{
		// Providers management
		api.GET("/providers", s.GetProviders)
		api.GET("/providers/:name", s.GetProvider)
		api.POST("/providers", s.AddProvider)
		api.PUT("/providers/:name", s.UpdateProvider)
		api.POST("/providers/:name/toggle", s.ToggleProvider)
		api.DELETE("/providers/:name", s.DeleteProvider)

		// Server management
		api.GET("/status", s.GetStatus)
		api.POST("/server/start", s.StartServer)
		api.POST("/server/stop", s.StopServer)
		api.POST("/server/restart", s.RestartServer)

		// Rule management
		api.GET("/rules", s.GetRules)
		api.GET("/rule/:name", s.GetRule)
		api.POST("/rule/:name", s.SetRule)

		// History
		api.GET("/history", s.GetHistory)

		// New API endpoints for defaults and provider models
		api.GET("/defaults", s.GetDefaults)
		api.POST("/defaults", s.SetDefaults)
		api.GET("/provider-models", s.GetProviderModels)
		api.POST("/provider-models/:name", s.FetchProviderModels)

		// Probe endpoint for testing rule configurations
		api.POST("/probe", s.ProbeRule)
	}
}

func (s *Server) useWebStaticEndpoints(engine *gin.Engine) {
	// Load templates and static files on the main router - try embedded first
	if s.assets != nil {
		log.Printf("Using embedded assets on main server")
		s.assets.SetupStaticRoutes(engine)
	} else {
		panic("No UI resources")
	}
}

// GetShutdownChannel returns the shutdown channel for the main process to listen on
func GetShutdownChannel() <-chan struct{} {
	return shutdownChan
}
