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

// WebUI represents the web interface management
type WebUI struct {
	enabled bool
	router  *gin.Engine
	config  *config.AppConfig
	logger  *memory.MemoryLogger
	assets  *EmbeddedAssets
}

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

// NewWebUI creates a new web UI manager
func NewWebUI(enabled bool, appConfig *config.AppConfig, logger *memory.MemoryLogger, router *gin.Engine) *WebUI {
	if !enabled {
		return &WebUI{enabled: false}
	}

	// Initialize embedded assets
	assets, err := NewEmbeddedAssets()
	if err != nil {
		log.Printf("Failed to initialize embedded assets: %v", err)
		// Continue without embedded assets, will fallback to file system
	}

	gin.SetMode(gin.ReleaseMode)
	wui := &WebUI{
		enabled: true,
		config:  appConfig,
		logger:  logger,
		router:  router,
		assets:  assets,
	}

	return wui
}

// useAPIEndpoints configures API routes for web UI
func (wui *WebUI) useAPIEndpoints(engine *gin.Engine) {
	if !wui.enabled {
		return
	}

	api := engine.Group("/api")
	api.Use(wui.authMiddleware()) // Apply authentication to all API routes
	{
		// Providers management
		api.GET("/providers", wui.GetProviders)
		api.GET("/providers/:name", wui.GetProvider)
		api.POST("/providers", wui.AddProvider)
		api.PUT("/providers/:name", wui.UpdateProvider)
		api.POST("/providers/:name/toggle", wui.ToggleProvider)
		api.DELETE("/providers/:name", wui.DeleteProvider)

		// Server management
		api.GET("/status", wui.GetStatus)
		api.POST("/server/start", wui.StartServer)
		api.POST("/server/stop", wui.StopServer)
		api.POST("/server/restart", wui.RestartServer)

		// History
		api.GET("/history", wui.GetHistory)

		// New API endpoints for defaults and provider models
		api.GET("/defaults", wui.GetDefaults)
		api.POST("/defaults", wui.SetDefaults)
		api.GET("/provider-models", wui.GetProviderModels)
		api.POST("/provider-models/:name", wui.FetchProviderModels)
	}
}

// GetRouter returns the gin router (nil if disabled)
func (wui *WebUI) GetRouter() *gin.Engine {
	if !wui.enabled {
		return nil
	}
	return wui.router
}

func useWebUI(engine *Server) {
	ui := NewWebUI(true, engine.config, engine.memoryLogger, engine.router)
	ui.Init()
}

// Init sets up WebUI routes and templates on the main server router
func (wui *WebUI) Init() {
	if !wui.enabled {
		return
	}

	// Middleware
	wui.router.Use(gin.Logger())
	wui.router.Use(gin.Recovery())

	wui.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
	})

	// Dashboard endpoints
	wui.router.GET("/dashboard", wui.Dashboard)

	// UI page routes
	ui := wui.router.Group("/ui")
	{
		ui.GET("/", wui.Dashboard)
		ui.GET("/dashboard", wui.Dashboard)
		ui.GET("/providers", wui.ProvidersPage)
		ui.GET("/system", wui.SystemPage)
		ui.GET("/history", wui.HistoryPage)
	}

	// API routes (for web UI functionality)
	wui.useAPIEndpoints(wui.router)

	// Static files and templates - try embedded assets first, fallback to filesystem
	wui.useStaticEndpoints(wui.router)
}

// IsEnabled returns whether web UI is enabled
func (wui *WebUI) IsEnabled() bool {
	return wui.enabled
}

// Page Handlers (exported for server integration)
func (wui *WebUI) Dashboard(c *gin.Context) {
	// Get user_auth_token from query parameter
	userAuthToken := c.Query("user_auth_token")

	// Prepare template data
	templateData := gin.H{}
	if userAuthToken != "" {
		templateData["user_auth_token"] = userAuthToken
	}

	if wui.assets != nil {
		wui.assets.HTML(c, "index.html", templateData)
	} else {
		panic("No UI resources")
	}
}

func (wui *WebUI) ProvidersPage(c *gin.Context) {
	if wui.assets != nil {
		wui.assets.HTML(c, "index.html", nil)
	} else {
		panic("No UI resources")
	}
}

func (wui *WebUI) SystemPage(c *gin.Context) {
	if wui.assets != nil {
		wui.assets.HTML(c, "index.html", nil)
	} else {
		panic("No UI resources")
	}
}

func (wui *WebUI) HistoryPage(c *gin.Context) {
	if wui.assets != nil {
		wui.assets.HTML(c, "index.html", nil)
	} else {
		panic("No UI resources")
	}
}

// API Handlers (exported for server integration)
func (wui *WebUI) GetProviders(c *gin.Context) {
	providers := wui.config.ListProviders()

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

func (wui *WebUI) GetStatus(c *gin.Context) {
	providers := wui.config.ListProviders()
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
			"port":              wui.config.GetServerPort(),
			"providers_total":   len(providers),
			"providers_enabled": enabledCount,
			"request_count":     0,
		},
	})
}

func (wui *WebUI) GetHistory(c *gin.Context) {
	if wui.logger != nil {
		history := wui.logger.GetHistory(50)
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

func (wui *WebUI) GetDefaults(c *gin.Context) {
	globalConfig := wui.config.GetGlobalConfig()
	if globalConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	requestConfigs := globalConfig.GetRequestConfigs()
	defaultRequestID := globalConfig.GetDefaultRequestID()

	// Convert RequestConfigs to response format
	responseConfigs := make([]map[string]interface{}, len(requestConfigs))
	for i, rc := range requestConfigs {
		responseConfigs[i] = map[string]interface{}{
			"request_model":  rc.RequestModel,
			"response_model": rc.ResponseModel,
			"provider":       rc.Provider,
			"default_model":  rc.DefaultModel,
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

// AddProvider adds a new provider
func (wui *WebUI) AddProvider(c *gin.Context) {
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

	err := wui.config.AddProvider(provider)
	if err != nil {
		if wui.logger != nil {
			wui.logger.LogAction(memory.ActionAddProvider, map[string]interface{}{
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

	if wui.logger != nil {
		wui.logger.LogAction(memory.ActionAddProvider, map[string]interface{}{
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
func (wui *WebUI) DeleteProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	err := wui.config.RemoveProvider(providerName)
	if err != nil {
		if wui.logger != nil {
			wui.logger.LogAction(memory.ActionDeleteProvider, map[string]interface{}{
				"name": providerName,
			}, false, err.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if wui.logger != nil {
		wui.logger.LogAction(memory.ActionDeleteProvider, map[string]interface{}{
			"name": providerName,
		}, true, fmt.Sprintf("Provider %s deleted successfully", providerName))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider deleted successfully",
	})
}

// UpdateProvider updates an existing provider
func (wui *WebUI) UpdateProvider(c *gin.Context) {
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
	provider, err := wui.config.GetProvider(providerName)
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

	err = wui.config.UpdateProvider(providerName, provider)
	if err != nil {
		if wui.logger != nil {
			wui.logger.LogAction(memory.ActionUpdateProvider, map[string]interface{}{
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

	if wui.logger != nil {
		wui.logger.LogAction(memory.ActionUpdateProvider, map[string]interface{}{
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
func (wui *WebUI) GetProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	provider, err := wui.config.GetProvider(providerName)
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
func (wui *WebUI) ToggleProvider(c *gin.Context) {
	providerName := c.Param("name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	provider, err := wui.config.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Provider not found",
		})
		return
	}

	// Toggle enabled status
	provider.Enabled = !provider.Enabled

	err = wui.config.UpdateProvider(providerName, provider)
	if err != nil {
		if wui.logger != nil {
			wui.logger.LogAction(memory.ActionUpdateProvider, map[string]interface{}{
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

	if wui.logger != nil {
		wui.logger.LogAction(memory.ActionUpdateProvider, map[string]interface{}{
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

func (wui *WebUI) StartServer(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Start server via web UI not supported. Please use CLI: tingly start",
	})
}

func (wui *WebUI) StopServer(c *gin.Context) {
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
	if wui.logger != nil {
		wui.logger.LogAction(memory.ActionStopServer, map[string]interface{}{
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

func (wui *WebUI) RestartServer(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Restart server via web UI not supported. Please use CLI: tingly restart",
	})
}

func (wui *WebUI) GenerateToken(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Not implemented",
	})
}

func (wui *WebUI) SetDefaults(c *gin.Context) {
	var req struct {
		RequestConfigs []config.RequestConfig `json:"request_configs"`
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

	globalConfig := wui.config.GetGlobalConfig()
	if globalConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	// Update RequestConfigs if provided
	if req.RequestConfigs != nil {
		if err := globalConfig.SetRequestConfigs(req.RequestConfigs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   fmt.Sprintf("Failed to update request configs: %v", err),
			})
			return
		}
	}

	if wui.logger != nil {
		logData := map[string]interface{}{
			"request_configs_count": 0,
		}

		if req.RequestConfigs != nil {
			logData["request_configs_count"] = len(req.RequestConfigs)
		}

		wui.logger.LogAction(memory.ActionUpdateDefaults, logData, true, "Request configs updated via web interface")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Request configs updated successfully",
	})
}

func (wui *WebUI) FetchProviderModels(c *gin.Context) {
	providerName := c.Param("name")

	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Provider name is required",
		})
		return
	}

	// Fetch and save models
	err := wui.config.FetchAndSaveProviderModels(providerName)
	if err != nil {
		if wui.logger != nil {
			wui.logger.LogAction(memory.ActionFetchModels, map[string]interface{}{
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
	providerModelManager := wui.config.GetProviderModelManager()
	models := providerModelManager.GetModels(providerName)

	if wui.logger != nil {
		wui.logger.LogAction(memory.ActionFetchModels, map[string]interface{}{
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

func (wui *WebUI) GetProviderModels(c *gin.Context) {
	providerModelManager := wui.config.GetProviderModelManager()
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
func (wui *WebUI) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the auth token from global config
		globalConfig := wui.config.GetGlobalConfig()
		if globalConfig == nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Global config not available",
			})
			c.Abort()
			return
		}

		expectedToken := globalConfig.GetUserToken()
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

func (wui *WebUI) useStaticEndpoints(engine *gin.Engine) {
	// Load templates and static files on the main router - try embedded first
	if wui.assets != nil {
		log.Printf("Using embedded assets on main server")
		wui.assets.SetupStaticRoutes(engine)
	} else {
		panic("No UI resources")
	}
}

// GetShutdownChannel returns the shutdown channel for the main process to listen on
func GetShutdownChannel() <-chan struct{} {
	return shutdownChan
}
