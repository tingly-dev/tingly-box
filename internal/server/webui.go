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

	assets "tingly-box/internal"
	"tingly-box/internal/config"
	"tingly-box/internal/obs"
	"tingly-box/pkg/swagger"

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

// Init sets up Server routes and templates on the main server engine
func (s *Server) UseUIEndpoints() {
	// UI page routes
	s.engine.GET("/home", s.UseIndexHTML)
	s.engine.GET("/credential", s.UseIndexHTML)
	s.engine.GET("/rule", s.UseIndexHTML)
	s.engine.GET("/system", s.UseIndexHTML)
	s.engine.GET("/history", s.UseIndexHTML)

	// API routes (for web UI functionality)
	s.useWebAPIEndpoints(s.engine)

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
	var provider *config.Provider
	var model = req.Model

	for _, p := range providers {
		if p.Enabled && p.Name == req.Provider {
			provider = p
			break
		}
	}

	startTime := time.Now()

	// Create the mock request data that would be sent to the API
	mockRequest := NewMockRequest(req.Provider, req.Model)

	if provider == nil {
		errorResp := ErrorDetail{
			Code:    "PROVIDER_NOT_FOUND",
			Message: fmt.Sprintf("Provider '%s' not found or disabled", req.Provider),
		}

		c.JSON(http.StatusBadRequest, ProbeResponse{
			Success: false,
			Error:   &errorResp,
			Data: &ProbeResponseData{
				Request: mockRequest,
			},
		})
		return
	}

	// Call the appropriate probe function based on provider API style
	var responseContent string
	var usage ProbeUsage
	var err error

	switch provider.APIStyle {
	case config.APIStyleAnthropic:
		responseContent, usage, err = probeWithAnthropic(c, provider, model)
	case config.APIStyleOpenAI:
		fallthrough
	default:
		responseContent, usage, err = probeWithOpenAI(c, provider, model)
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

		c.JSON(http.StatusOK, ProbeResponse{
			Success: false,
			Error:   &errorResp,
			Data: &ProbeResponseData{
				Request: mockRequest,
				Response: ProbeResponseDetail{
					Content:      "",
					Model:        model,
					Provider:     provider.Name,
					FinishReason: "error",
					Error:        errorMessage,
				},
				Usage: ProbeUsage{},
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
			Request: mockRequest,
			Response: ProbeResponseDetail{
				Content:      responseContent,
				FinishReason: finishReason,
			},
			Usage: usage,
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
func (s *Server) useWebAPIEndpoints(engine *gin.Engine) {
	// Create route manager
	manager := swagger.NewRouteManager(engine)

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
			Name: "Apache License\nVersion 2.0",
			URL:  "https://www.apache.org/licenses/LICENSE-2.0",
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
	authAPI := manager.NewGroup("api", "v1", "")
	authAPI.Router.Use(s.UserAuthMiddleware())

	// Health check endpoint
	authAPI.GET("/info/health", s.GetHealthInfo,
		swagger.WithResponseModel(HealthInfoResponse{}),
	)

	authAPI.GET("/info/config", s.GetInfoConfig,
		swagger.WithDescription("Get config info about this application"),
		swagger.WithResponseModel(ConfigInfoResponse{}),
	)

	// Provider Management
	authAPI.GET("/providers", (s.GetProviders),
		swagger.WithDescription("Get all configured providers with masked tokens"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProvidersResponse{}),
	)

	authAPI.GET("/providers/:name", s.GetProvider,
		swagger.WithDescription("Get specific provider details with masked token"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProviderResponse{}),
	)

	authAPI.POST("/providers", s.AddProvider,
		swagger.WithDescription("Add a new provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithRequestModel(AddProviderRequest{}),
		swagger.WithResponseModel(AddProviderResponse{}),
	)

	authAPI.PUT("/providers/:name", s.UpdateProvider,
		swagger.WithDescription("Update existing provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithRequestModel(UpdateProviderRequest{}),
		swagger.WithResponseModel(UpdateProviderResponse{}),
	)

	authAPI.POST("/providers/:name/toggle", s.ToggleProvider,
		swagger.WithDescription("Toggle provider enabled/disabled status"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ToggleProviderResponse{}),
	)

	authAPI.DELETE("/providers/:name", s.DeleteProvider,
		swagger.WithDescription("Delete a provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(DeleteProviderResponse{}),
	)

	// Server Management
	authAPI.GET("/status", s.GetStatus,
		swagger.WithDescription("Get server status and statistics"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(StatusResponse{}),
	)

	authAPI.POST("/server/start", s.StartServer,
		swagger.WithDescription("Start the server"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	authAPI.POST("/server/stop", (s.StopServer),
		swagger.WithDescription("Stop the server gracefully"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	authAPI.POST("/server/restart", (s.RestartServer),
		swagger.WithDescription("Restart the server"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	// Rule Management
	authAPI.GET("/rules", (s.GetRules),
		swagger.WithDescription("Get all configured rules"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(RulesResponse{}),
	)

	authAPI.GET("/rule/:uuid", (s.GetRule),
		swagger.WithDescription("Get specific rule by UUID"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(RuleResponse{}),
	)

	authAPI.POST("/rule/:uuid", (s.SetRule),
		swagger.WithDescription("Create or update a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithRequestModel(SetRuleRequest{}),
		swagger.WithResponseModel(SetRuleResponse{}),
	)

	authAPI.DELETE("/rule/:uuid", (s.DeleteRule),
		swagger.WithDescription("Delete a rule configuration"),
		swagger.WithTags("rules"),
		swagger.WithResponseModel(DeleteRuleResponse{}),
	)

	// History
	authAPI.GET("/history", (s.GetHistory),
		swagger.WithDescription("Get request history"),
		swagger.WithTags("history"),
		swagger.WithResponseModel(HistoryResponse{}),
	)

	// Provider Models Management
	authAPI.GET("/provider-models", (s.GetProviderModels),
		swagger.WithDescription("Get all provider models"),
		swagger.WithTags("models"),
		swagger.WithResponseModel(ProviderModelsResponse{}),
	)

	authAPI.POST("/provider-models/:name", (s.FetchProviderModels),
		swagger.WithDescription("Fetch models for a specific provider"),
		swagger.WithTags("models"),
		swagger.WithResponseModel(FetchProviderModelsResponse{}),
	)

	// Probe endpoint
	authAPI.POST("/probe", s.HandleProbeModel,
		swagger.WithDescription("Test a rule configuration by sending a sample request"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(ProbeRequest{}),
		swagger.WithResponseModel(ProbeResponse{}),
	)

	authAPI.POST("/probe/model", s.HandleProbeModel,
		swagger.WithDescription("Test a model forwarding by sending a sample request"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(ProbeRequest{}),
		swagger.WithResponseModel(ProbeResponse{}),
	)

	authAPI.POST("/probe/provider", s.HandleProbeProvider,
		swagger.WithDescription("Test api key for the provider"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(ProbeProviderRequest{}),
		swagger.WithResponseModel(ProbeProviderResponse{}),
	)

	// Token Management
	authAPI.POST("/token", (s.GenerateToken),
		swagger.WithDescription("Generate a new API token"),
		swagger.WithTags("token"),
		swagger.WithRequestModel(GenerateTokenRequest{}),
		swagger.WithResponseModel(TokenResponse{}),
	)

	authAPI.GET("/token", (s.GetToken),
		swagger.WithDescription("Get existing API token or generate new one"),
		swagger.WithTags("token"),
		swagger.WithResponseModel(TokenResponse{}),
	)

	// Setup Swagger documentation endpoint
	manager.SetupSwaggerEndpoints()
}

func (s *Server) useWebStaticEndpoints(engine *gin.Engine) {
	// Load templates and static files on the main engine - try embedded first
	log.Printf("Using embedded assets on main server")

	// Serve static assets from embedded filesystem
	st, _ := fs.Sub(assets.WebDistAssets, "web/dist/assets")
	engine.StaticFS("/assets", http.FS(st))

	engine.StaticFile("/vite.svg", "web/dist/vite.svg")

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
