package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	assets "tingly-box/internal"
	"tingly-box/internal/obs"
	"tingly-box/internal/typ"
	"tingly-box/pkg/swagger"
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

// Init sets up Server routes and templates on the main server engine
func (s *Server) UseUIEndpoints() {
	// UI page routes
	s.engine.GET("/home", s.UseIndexHTML)
	s.engine.GET("/provider", s.UseIndexHTML)
	s.engine.GET("/api-keys", s.UseIndexHTML)
	s.engine.GET("/oauth", s.UseIndexHTML)
	s.engine.GET("/routing", s.UseIndexHTML)
	s.engine.GET("/system", s.UseIndexHTML)
	s.engine.GET("/history", s.UseIndexHTML)

	// Create route manager
	manager := swagger.NewRouteManager(s.engine)

	// API routes (for web UI functionality)
	s.useWebAPIEndpoints(manager)

	s.useOAuthEndpoints(manager)

	// Usage API routes
	s.RegisterUsageRoutes(manager)

	// Static files and templates - try embedded assets first, fallback to filesystem
	s.useWebStaticEndpoints(s.engine)
}

// HandleProbeModel tests a rule configuration by sending a sample request to the configured provider
func (s *Server) HandleProbeModel(c *gin.Context) {

	var req ProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if req.Provider == "" || req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{})
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
	var provider *typ.Provider
	var model = req.Model

	for _, p := range providers {
		if p.Enabled && p.UUID == req.Provider {
			provider = p
			break
		}
	}

	if provider == nil {
		errorResp := ErrorDetail{
			Code:    "PROVIDER_NOT_FOUND",
			Message: fmt.Sprintf("Provider '%s' not found or disabled", req.Provider),
		}

		c.JSON(http.StatusBadRequest, ProbeResponse{
			Success: false,
			Error:   &errorResp,
			Data:    &ProbeResponseData{},
		})
		return
	}

	startTime := time.Now()

	// Generate curl command for this provider/model
	curlCommand := GenerateCurlCommand(
		provider.APIBase,
		string(provider.APIStyle),
		provider.Token,
		model,
	)

	// Create the mock request data that would be sent to the API
	mockRequest := NewMockRequest(provider.Name, req.Model)

	// Call the appropriate probe function based on provider API style
	var responseContent string
	var usage ProbeUsage
	var err error

	switch provider.APIStyle {
	case typ.APIStyleAnthropic:
		responseContent, usage, err = s.probeWithAnthropic(c, provider, model)
	case typ.APIStyleOpenAI:
		fallthrough
	default:
		responseContent, usage, err = s.probeWithOpenAI(c, provider, model)
	}

	endTime := time.Now()

	if err != nil {
		// Extract error code from the formatted error message
		errorMessage := err.Error()
		errorCode := "PROBE_FAILED"

		errorResp := ErrorDetail{
			Message: fmt.Sprintf("Probe failed: %s", errorMessage),
			Type:    "error",
			Code:    errorCode,
		}

		c.JSON(http.StatusNotFound, ProbeResponse{
			Success: false,
			Error:   &errorResp,
			Data: &ProbeResponseData{
				Request:     mockRequest,
				Response:    ProbeResponseDetail{Content: "", Model: model, Provider: provider.Name, FinishReason: "error", Error: errorMessage},
				Usage:       ProbeUsage{},
				CurlCommand: curlCommand,
			},
		})
		return
	}

	finishReason := "stop"
	if usage.TotalTokens == 0 {
		finishReason = "unknown"
	}

	usage.TimeCost = int(endTime.Sub(startTime).Milliseconds())
	c.JSON(http.StatusOK, ProbeResponse{
		Success: true,
		Data: &ProbeResponseData{
			Request:     mockRequest,
			Response:    ProbeResponseDetail{Content: responseContent, FinishReason: finishReason},
			Usage:       usage,
			CurlCommand: curlCommand,
		},
	})
}

func (s *Server) UseIndexHTML(c *gin.Context) {
	c.FileFromFS("web/dist/index.html", http.FS(assets.WebDistAssets))
}

func (s *Server) GetStatus(c *gin.Context) {
	providers := s.config.ListProviders()
	enabledCount := 0
	for _, p := range providers {
		if p.Enabled {
			enabledCount++
		}
	}

	response := StatusResponse{
		Success: true,
	}
	response.Data.ServerRunning = true
	response.Data.Port = s.config.GetServerPort()
	response.Data.ProvidersTotal = len(providers)
	response.Data.ProvidersEnabled = enabledCount
	response.Data.RequestCount = 0

	c.JSON(http.StatusOK, response)
}

func (s *Server) GetHistory(c *gin.Context) {
	response := HistoryResponse{
		Success: true,
	}

	if s.logger != nil {
		history := s.logger.GetHistory(50)
		response.Data = history
	} else {
		response.Data = []interface{}{}
	}

	c.JSON(http.StatusOK, response)
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
	response := ServerActionResponse{
		Success: false,
		Message: "Start server via web UI not supported. Please use CLI: tingly start",
	}
	c.JSON(http.StatusNotImplemented, response)
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
		s.logger.LogAction(obs.ActionStopServer, map[string]interface{}{
			"source": "web_ui",
		}, true, "Server stopped via web interface")
	}

	// Send shutdown signal to main process
	select {
	case shutdownChan <- struct{}{}:
	default:
		// Channel already has a signal
	}

	response := ServerActionResponse{
		Success: true,
		Message: "Server stopped successfully. The application will now exit.",
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) RestartServer(c *gin.Context) {
	response := ServerActionResponse{
		Success: false,
		Message: "Restart server via web UI not supported. Please use CLI: tingly restart",
	}
	c.JSON(http.StatusNotImplemented, response)
}

// NewGinHandlerWrapper converts gin.HandlerFunc to swagger.Handler
func NewGinHandlerWrapper(h gin.HandlerFunc) swagger.Handler {
	return swagger.Handler(h)
}

// useWebAPIEndpoints configures API routes for web UI using swagger manager
func (s *Server) useWebAPIEndpoints(manager *swagger.RouteManager) {
	// Set Swagger information
	manager.SetSwaggerInfo(swagger.SwaggerInfo{
		Title:       "Tingly Box API",
		Description: "A Restful API for tingly-box with automatic Swagger documentation generation.",
		Version:     "1.0.0",
		Host:        fmt.Sprintf("localhost:%d", s.config.ServerPort),
		BasePath:    "/",
		Contact: swagger.SwaggerContact{
			Name:  "API Support",
			Email: "ops@tingly.dev",
		},
		License: swagger.SwaggerLicense{
			Name: "Mozilla Public License\nVersion 2.0",
			URL:  "https://www.mozilla.org/en-US/MPL/2.0/",
		},
	})

	// Add global middleware
	manager.AddGlobalMiddleware(
		func(c *gin.Context) {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
				return
			}
			c.Next()
		},
	)

	// Create authenticated API group
	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.authMW.UserAuthMiddleware())

	apiV2 := manager.NewGroup("api", "v2", "")
	apiV2.Router.Use(s.authMW.UserAuthMiddleware())

	// Health check endpoint
	apiV1.GET("/info/health", s.GetHealthInfo,
		swagger.WithTags("info"),
		swagger.WithResponseModel(HealthInfoResponse{}),
	)

	apiV1.GET("/info/config", s.GetInfoConfig,
		swagger.WithTags("info"),
		swagger.WithDescription("Get config info about this application"),
		swagger.WithResponseModel(ConfigInfoResponse{}),
	)

	apiV1.GET("/info/version", s.GetInfoVersion,
		swagger.WithTags("info"),
		swagger.WithDescription("Get version info about this application"),
		swagger.WithResponseModel(VersionInfoResponse{}),
	)

	// Log API routes
	apiV1.GET("/log", s.GetLogs,
		swagger.WithDescription("Get logs with optional filtering"),
		swagger.WithTags("logs"),
		swagger.WithResponseModel(LogsResponse{}),
	)
	apiV1.GET("/log/stats", s.GetLogStats,
		swagger.WithDescription("Get log statistics"),
		swagger.WithTags("logs"),
	)
	apiV1.DELETE("/log", s.ClearLogs,
		swagger.WithDescription("Clear all logs"),
		swagger.WithTags("logs"),
	)

	// Provider Management
	//apiV1.GET("/providers", (s.GetProviders),
	//	swagger.WithDescription("Get all configured providers with masked tokens"),
	//	swagger.WithTags("providers"),
	//	swagger.WithResponseModel(ProvidersResponse{}),
	//)
	//
	//apiV1.GET("/providers/:name", s.GetProviderByName,
	//	swagger.WithDescription("Get specific provider details with masked token"),
	//	swagger.WithTags("providers"),
	//	swagger.WithResponseModel(ProviderResponse{}),
	//)
	//
	//apiV1.POST("/providers", s.CreateProvider,
	//	swagger.WithDescription("Add a new provider configuration"),
	//	swagger.WithTags("providers"),
	//	swagger.WithRequestModel(CreateProviderRequest{}),
	//	swagger.WithResponseModel(CreateProviderResponse{}),
	//)
	//
	//apiV1.PUT("/providers/:name", s.UpdateProvider,
	//	swagger.WithDescription("Update existing provider configuration"),
	//	swagger.WithTags("providers"),
	//	swagger.WithRequestModel(UpdateProviderRequest{}),
	//	swagger.WithResponseModel(UpdateProviderResponse{}),
	//)
	//
	//apiV1.POST("/providers/:name/toggle", s.ToggleProvider,
	//	swagger.WithDescription("Toggle provider enabled/disabled status"),
	//	swagger.WithTags("providers"),
	//	swagger.WithResponseModel(ToggleProviderResponse{}),
	//)

	useV2Provider(s, apiV2)

	// Server Management
	apiV1.GET("/status", s.GetStatus,
		swagger.WithDescription("Get server status and statistics"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(StatusResponse{}),
	)

	apiV1.POST("/server/start", s.StartServer,
		swagger.WithDescription("Start the server"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	apiV1.POST("/server/stop", s.StopServer,
		swagger.WithDescription("Stop the server gracefully"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	apiV1.POST("/server/restart", s.RestartServer,
		swagger.WithDescription("Restart the server"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	// Rule Management
	apiV1.GET("/rules", s.GetRules,
		swagger.WithDescription("Get all configured rules"),
		swagger.WithTags("rules"),
		swagger.WithQueryRequired("scenario", "string", "Filter by scenario"),
		swagger.WithResponseModel(RulesResponse{}),
	)

	apiV1.GET("/rule/:uuid", s.GetRule,
		swagger.WithDescription("Get specific rule by UUID"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(RuleResponse{}),
	)

	apiV1.POST("/rule/:uuid", s.UpdateRule,
		swagger.WithDescription("Create or update a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithRequestModel(UpdateRuleRequest{}),
		swagger.WithResponseModel(UpdateRuleResponse{}),
	)

	apiV1.POST("/rule", s.CreateRule,
		swagger.WithDescription("Create or update a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithRequestModel(CreateRuleRequest{}),
		swagger.WithResponseModel(UpdateRuleResponse{}),
	)

	apiV1.DELETE("/rule/:uuid", s.DeleteRule,
		swagger.WithDescription("Delete a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(DeleteRuleResponse{}),
	)

	// Scenario Management
	apiV1.GET("/scenarios", s.GetScenarios,
		swagger.WithDescription("Get all scenario configurations"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenariosResponse{}),
	)

	apiV1.GET("/scenario/:scenario", s.GetScenarioConfig,
		swagger.WithDescription("Get configuration for a specific scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioResponse{}),
	)

	apiV1.POST("/scenario/:scenario", s.SetScenarioConfig,
		swagger.WithDescription("Create or update scenario configuration"),
		swagger.WithTags("scenarios"),
		swagger.WithRequestModel(ScenarioUpdateRequest{}),
		swagger.WithResponseModel(ScenarioUpdateResponse{}),
	)

	apiV1.GET("/scenario/:scenario/flag/:flag", s.GetScenarioFlag,
		swagger.WithDescription("Get a specific flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	apiV1.PUT("/scenario/:scenario/flag/:flag", s.SetScenarioFlag,
		swagger.WithDescription("Set a specific flag value for a scenario"),
		swagger.WithTags("scenarios"),
		swagger.WithResponseModel(ScenarioFlagResponse{}),
	)

	// History
	apiV1.GET("/history", s.GetHistory,
		swagger.WithDescription("Get request history"),
		swagger.WithTags("history"),
		swagger.WithResponseModel(HistoryResponse{}),
	)

	// Provider Models Management
	apiV1.GET("/provider-models/:uuid", s.GetProviderModelsByUUID,
		swagger.WithDescription("Get all provider models"),
		swagger.WithTags("models"),
		swagger.WithResponseModel(ProviderModelsResponse{}),
	)

	apiV1.POST("/provider-models/:uuid", s.UpdateProviderModelsByUUID,
		swagger.WithDescription("Fetch models for a specific provider"),
		swagger.WithTags("models"),
		swagger.WithResponseModel(ProviderModelsResponse{}),
	)

	// Probe endpoint
	apiV1.POST("/probe", s.HandleProbeModel,
		swagger.WithDescription("Test a rule configuration by sending a sample request"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(ProbeRequest{}),
		swagger.WithResponseModel(ProbeResponse{}),
	)

	apiV1.POST("/probe/model", s.HandleProbeModel,
		swagger.WithDescription("Test a model forwarding by sending a sample request"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(ProbeRequest{}),
		swagger.WithResponseModel(ProbeResponse{}),
	)

	apiV1.POST("/probe/provider", s.HandleProbeProvider,
		swagger.WithDescription("Test api key for the provider"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(ProbeProviderRequest{}),
		swagger.WithResponseModel(ProbeProviderResponse{}),
	)

	// Token Management
	apiV1.POST("/token", s.GenerateToken,
		swagger.WithDescription("Generate a new API token"),
		swagger.WithTags("token"),
		swagger.WithRequestModel(GenerateTokenRequest{}),
		swagger.WithResponseModel(TokenResponse{}),
	)

	apiV1.GET("/token", s.GetToken,
		swagger.WithDescription("Get existing API token or generate new one"),
		swagger.WithTags("token"),
		swagger.WithResponseModel(TokenResponse{}),
	)

	// Setup Swagger documentation endpoint
	manager.SetupSwaggerEndpoints()
}

func useV2Provider(s *Server, api *swagger.RouteGroup) {

	api.GET("/providers", s.GetProviders,
		swagger.WithDescription("Get all configured providers with masked tokens"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProvidersResponse{}),
	)

	api.GET("/providers/:uuid", s.GetProvider,
		swagger.WithDescription("Get specific provider details with masked token"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProviderResponse{}),
	)

	api.POST("/providers", s.CreateProvider,
		swagger.WithDescription("Create a new provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithRequestModel(CreateProviderRequest{}),
		swagger.WithResponseModel(CreateProviderResponse{}),
	)

	api.PUT("/providers/:uuid", s.UpdateProvider,
		swagger.WithDescription("Update existing provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithRequestModel(UpdateProviderRequest{}),
		swagger.WithResponseModel(UpdateProviderResponse{}),
	)

	api.POST("/providers/:uuid/toggle", s.ToggleProvider,
		swagger.WithDescription("Toggle provider enabled/disabled status"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ToggleProviderResponse{}),
	)

	api.DELETE("/providers/:uuid", s.DeleteProvider,
		swagger.WithDescription("Delete a provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(DeleteProviderResponse{}),
	)

	// Provider template endpoints
	api.GET("/provider-templates", s.GetProviderTemplates,
		swagger.WithDescription("Get all provider templates"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(TemplateResponse{}),
	)

	api.GET("/provider-templates/:id", s.GetProviderTemplate,
		swagger.WithDescription("Get a specific provider template by ID"),
		swagger.WithTags("providers"),
	)

	api.POST("/provider-templates/refresh", s.RefreshProviderTemplates,
		swagger.WithDescription("Refresh provider templates from GitHub"),
		swagger.WithTags("providers"),
	)

	api.GET("/provider-templates/version", s.GetProviderTemplateVersion,
		swagger.WithDescription("Get current provider template registry version"),
		swagger.WithTags("providers"),
	)
}

func (s *Server) useWebStaticEndpoints(engine *gin.Engine) {
	// Load templates and static files on the main engine - try embedded first
	log.Printf("Using embedded assets on main server")

	// Serve static assets from embedded filesystem
	st, _ := fs.Sub(assets.WebDistAssets, "web/dist/assets")
	engine.StaticFS("/assets", http.FS(st))

	engine.StaticFile("/icon.svg", "web/dist/icon.svg")

	engine.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes - let them return 404s
		path := c.Request.URL.Path
		// Check if this looks like an API route
		if path == "" || strings.HasPrefix(path, "/api/v") || strings.HasPrefix(path, "/v") || strings.HasPrefix(path, "/openai") || strings.HasPrefix(path, "/anthropic") {
			// This looks like an API route, return 404
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "API endpoint not found",
					"type":    "invalid_request_error",
					"code":    "not_found",
				},
			})
			return
		}

		// For all other routes, serve the SPA index.html
		data, err := assets.WebDistAssets.ReadFile("web/dist/index.html")
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}

// GetShutdownChannel returns the shutdown channel for the main process to listen on
func GetShutdownChannel() <-chan struct{} {
	return shutdownChan
}
