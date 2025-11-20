package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"tingly-box/internal/config"
	"tingly-box/internal/memory"

	"github.com/gin-gonic/gin"
)

// WebUI represents the web interface management
type WebUI struct {
	enabled bool
	router  *gin.Engine
	config  *config.AppConfig
	logger  *memory.MemoryLogger
}

// NewWebUI creates a new web UI manager
func NewWebUI(enabled bool, appConfig *config.AppConfig, logger *memory.MemoryLogger) *WebUI {
	if !enabled {
		return &WebUI{enabled: false}
	}

	gin.SetMode(gin.ReleaseMode)
	wui := &WebUI{
		enabled: true,
		config:  appConfig,
		logger:  logger,
		router:  gin.New(),
	}

	wui.setupRoutes()
	return wui
}

// setupRoutes configures web UI routes
func (wui *WebUI) setupRoutes() {
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

	// Static files and templates - use absolute paths
	templatePath := getTemplatePath()
	staticPath := getStaticPath()

	// Log the paths for debugging
	log.Printf("Loading templates from: %s", templatePath)
	log.Printf("Loading static files from: %s", staticPath)

	// Load templates
	wui.router.LoadHTMLGlob(templatePath)
	log.Printf("Templates loaded successfully")
	wui.router.Static("/static", staticPath)

	// Dashboard endpoints
	wui.router.GET("/", wui.Dashboard)
	wui.router.GET("/dashboard", wui.Dashboard)

	// UI page routes
	ui := wui.router.Group("/ui")
	{
		ui.GET("/", wui.Dashboard)
		ui.GET("/dashboard", wui.Dashboard)
		ui.GET("/providers", wui.ProvidersPage)
		ui.GET("/server", wui.ServerPage)
		ui.GET("/history", wui.HistoryPage)
	}

	// API routes (for web UI functionality)
	wui.setupAPIRoutes()
}

// setupAPIRoutes configures API routes for web UI
func (wui *WebUI) setupAPIRoutes() {
	if !wui.enabled {
		return
	}

	api := wui.router.Group("/api")
	{
		// Providers management
		api.GET("/providers", wui.GetProviders)
		api.POST("/providers", wui.AddProvider)
		api.DELETE("/providers/:name", wui.DeleteProvider)

		// Server management
		api.GET("/status", wui.GetStatus)
		api.POST("/server/start", wui.StartServer)
		api.POST("/server/stop", wui.StopServer)
		api.POST("/server/restart", wui.RestartServer)

		// Token generation
		api.GET("/token", wui.GenerateToken)

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

// SetupRoutesOnServer sets up WebUI routes and templates on the main server router
func (wui *WebUI) SetupRoutesOnServer(mainRouter *gin.Engine) {
	if !wui.enabled {
		return
	}

	// Load templates on the main router
	templatePath := getTemplatePath()
	staticPath := getStaticPath()

	log.Printf("Loading templates on main server from: %s", templatePath)
	log.Printf("Loading static files on main server from: %s", staticPath)

	// Load templates on main router
	mainRouter.LoadHTMLGlob(templatePath)
	log.Printf("Templates loaded successfully on main server")

	// Load static files on main router
	mainRouter.Static("/static", staticPath)

	// Add dashboard routes to main router
	mainRouter.GET("/", wui.Dashboard)
	mainRouter.GET("/dashboard", wui.Dashboard)

	// UI page routes on main router
	ui := mainRouter.Group("/ui")
	{
		ui.GET("/", wui.Dashboard)
		ui.GET("/dashboard", wui.Dashboard)
		ui.GET("/providers", wui.ProvidersPage)
		ui.GET("/server", wui.ServerPage)
		ui.GET("/history", wui.HistoryPage)
	}

	// Add API routes for web UI functionality on main router
	api := mainRouter.Group("/api")
	{
		// Providers management
		api.GET("/providers", wui.GetProviders)
		api.POST("/providers", wui.AddProvider)
		api.DELETE("/providers/:name", wui.DeleteProvider)

		// Server management
		api.GET("/status", wui.GetStatus)
		api.POST("/server/start", wui.StartServer)
		api.POST("/server/stop", wui.StopServer)
		api.POST("/server/restart", wui.RestartServer)

		// Token generation
		api.GET("/token", wui.GenerateToken)

		// History
		api.GET("/history", wui.GetHistory)

		// Defaults and provider models
		api.GET("/defaults", wui.GetDefaults)
		api.POST("/defaults", wui.SetDefaults)
		api.GET("/provider-models", wui.GetProviderModels)
		api.POST("/provider-models/:name", wui.FetchProviderModels)
	}
}

// IsEnabled returns whether web UI is enabled
func (wui *WebUI) IsEnabled() bool {
	return wui.enabled
}

// Page Handlers (exported for server integration)
func (wui *WebUI) Dashboard(c *gin.Context) {
	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"title": "Tingly Box Dashboard",
	})
}

func (wui *WebUI) ProvidersPage(c *gin.Context) {
	c.HTML(http.StatusOK, "providers.html", gin.H{
		"title": "Providers - Tingly Box",
	})
}

func (wui *WebUI) ServerPage(c *gin.Context) {
	c.HTML(http.StatusOK, "server.html", gin.H{
		"title": "Server - Tingly Box",
	})
}

func (wui *WebUI) HistoryPage(c *gin.Context) {
	c.HTML(http.StatusOK, "history.html", gin.H{
		"title": "History - Tingly Box",
	})
}

// API Handlers (exported for server integration)
func (wui *WebUI) GetProviders(c *gin.Context) {
	providers := wui.config.ListProviders()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    providers,
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

	defaultProvider, defaultModel, requestModel, responseModel := globalConfig.GetDefaults()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"defaultProvider": defaultProvider,
			"defaultModel":    defaultModel,
			"requestModel":    requestModel,
			"responseModel":   responseModel,
		},
	})
}

// Placeholder implementations for complex handlers
func (wui *WebUI) AddProvider(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Not implemented",
	})
}

func (wui *WebUI) DeleteProvider(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Not implemented",
	})
}

func (wui *WebUI) StartServer(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Not implemented",
	})
}

func (wui *WebUI) StopServer(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Not implemented",
	})
}

func (wui *WebUI) RestartServer(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"error":   "Not implemented",
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
		DefaultProvider string `json:"defaultProvider"`
		DefaultModel    string `json:"defaultModel"`
		RequestModel    string `json:"requestModel"`
		ResponseModel   string `json:"responseModel"`
	}

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

	// Update defaults
	if req.DefaultProvider != "" {
		if err := globalConfig.SetDefaultProvider(req.DefaultProvider); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	}

	if req.DefaultModel != "" {
		if err := globalConfig.SetDefaultModel(req.DefaultModel); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	}

	if req.RequestModel != "" {
		if err := globalConfig.SetRequestModel(req.RequestModel); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	}

	if req.ResponseModel != "" {
		if err := globalConfig.SetResponseModel(req.ResponseModel); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	}

	if wui.logger != nil {
		wui.logger.LogAction(memory.ActionUpdateDefaults, map[string]interface{}{
			"default_provider": req.DefaultProvider,
			"default_model":    req.DefaultModel,
			"request_model":    req.RequestModel,
			"response_model":   req.ResponseModel,
		}, true, "Global defaults updated via web interface")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Defaults updated successfully",
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

// Helper functions to get template and static file paths
func getTemplatePath() string {
	if wd, err := os.Getwd(); err == nil {
		templatePath := filepath.Join(wd, "web", "templates", "*")
		if _, err := os.Stat(templatePath); err == nil {
			return templatePath
		}
	}
	// Fallback to relative path
	return "web/templates/*"
}

func getStaticPath() string {
	if wd, err := os.Getwd(); err == nil {
		staticPath := filepath.Join(wd, "web", "static")
		if _, err := os.Stat(staticPath); err == nil {
			return staticPath
		}
	}
	// Fallback to relative path
	return "./web/static"
}
