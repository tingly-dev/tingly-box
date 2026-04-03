package server

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	assets "github.com/tingly-dev/tingly-box/internal"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/configapply"
	"github.com/tingly-dev/tingly-box/internal/server/module/imbot"
	notifymodule "github.com/tingly-dev/tingly-box/internal/server/module/notify"
	oauthmodule "github.com/tingly-dev/tingly-box/internal/server/module/oauth"
	providerQuotaModule "github.com/tingly-dev/tingly-box/internal/server/module/provider_quota"
	"github.com/tingly-dev/tingly-box/internal/server/module/providertemplate"
	rulemodule "github.com/tingly-dev/tingly-box/internal/server/module/rule"
	"github.com/tingly-dev/tingly-box/internal/server/module/scenario"
	"github.com/tingly-dev/tingly-box/internal/server/module/skill"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	usagemodule "github.com/tingly-dev/tingly-box/internal/server/module/usage"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
	"github.com/tingly-dev/tingly-box/pkg/swagger"
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
func (s *Server) UseUIEndpoints(ctx context.Context) {

	// API endpoints are handled separately and won't match this pattern
	// Admin/backend routes that need their own pages:
	// - /provider, /api-keys, /oauth, /routing, /system, /history etc.
	// All serve the same index.html, letting React Router handle the navigation

	// Exclude API routes from SPA catch-all by registering them first
	// The routes registered below (manager APIs, OAuth, usage, etc.) will take precedence

	// Claude Code status line endpoints (no auth required) - register from claudecode module
	// These must be registered before the /tingly/:scenario routes
	var quotaMgr statusline.QuotaManager
	if qm, ok := s.quotaManager.(interface {
		GetQuota(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error)
	}); ok {
		quotaMgr = qm
	}
	statusHandler := statusline.NewHandler(s.config, s.loadBalancer, statusline.NewCache(), quotaMgr)
	statusline.RegisterRoutes(s.engine, statusHandler)

	// Claude Code notification hook endpoint (no auth required)
	notifyHandler := notifymodule.NewHandler()
	notifymodule.RegisterRoutes(s.engine, notifyHandler)

	// Create route manager
	manager := swagger.NewRouteManager(s.engine)

	// API routes (for web UI functionality)
	s.useWebAPIEndpoints(manager)

	// OAuth API routes - register from oauth module
	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.getUserAuthMiddleware())
	oauthmodule.RegisterRoutes(apiV1, s.getUserAuthMiddleware(), s.oauthHandler)
	// Register callback routes (unauthenticated)
	oauthmodule.RegisterCallbackRoutes(manager, s.oauthHandler)

	// Usage API routes - register from usage module
	// Note: apiV1 is already created above with auth middleware
	sm := s.config.StoreManager()
	if sm != nil {
		usageHandler := usagemodule.NewHandler(sm.Usage())
		usagemodule.RegisterRoutes(apiV1, usageHandler)
	}

	// ImBot settings API routes - register from imbotsettings module
	imbotHandler, err := imbot.NewHandler(ctx, s.config)
	if err != nil {
		logrus.WithError(err).Warn("Failed to create imbotsettings handler, imbot settings APIs will not be available")
	} else {
		imbot.RegisterRoutes(apiV1, imbotHandler)
		// Store handler reference for shutdown
		s.imbotSettingsHandler = imbotHandler
	}

	// Config apply API routes
	configapplyHandler := configapply.NewHandler(s.config, s.host)
	configapply.RegisterRoutes(apiV1, configapplyHandler)

	// Provider quota API routes
	if s.quotaManager != nil {
		if qm, ok := s.quotaManager.(providerQuotaModule.Manager); ok {
			quotaHandler := providerQuotaModule.NewHandler(qm, logrus.StandardLogger())
			quotaHandler.RegisterRoutes(apiV1.Router)
			logrus.Info("Provider quota API routes registered")
		}
	}

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

	// Get the appropriate client based on API style
	var prober client.Prober
	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		prober = s.clientPool.GetOpenAIClient(provider, model)
	case protocol.APIStyleAnthropic:
		prober = s.clientPool.GetAnthropicClient(provider, model)
	case protocol.APIStyleGoogle:
		prober = s.clientPool.GetGoogleClient(provider, model)
	default:
		errorMessage := "unknown api style"
		c.JSON(http.StatusNotFound, ProbeResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: fmt.Sprintf("Probe failed: %s", errorMessage),
				Type:    "error",
				Code:    "PROBE_FAILED",
			},
			Data: &ProbeResponseData{
				Request:     mockRequest,
				Response:    ProbeResponseDetail{Content: "", Model: model, Provider: provider.Name, FinishReason: "error", Error: errorMessage},
				Usage:       ProbeUsage{},
				CurlCommand: curlCommand,
			},
		})
		c.Abort()
		return
	}

	// Call the probe method
	var result client.ProbeResult
	if prober != nil {
		result = prober.ProbeChatEndpoint(c.Request.Context(), model)
	}

	endTime := time.Now()

	if !result.Success {
		errorMessage := result.ErrorMessage
		if errorMessage == "" {
			errorMessage = "Probe failed"
		}

		errorResp := ErrorDetail{
			Message: fmt.Sprintf("Probe failed: %s", errorMessage),
			Type:    "error",
			Code:    "PROBE_FAILED",
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
	if result.TotalTokens == 0 {
		finishReason = "unknown"
	}

	usage := ProbeUsage{
		PromptTokens:     result.PromptTokens,
		CompletionTokens: result.CompletionTokens,
		TotalTokens:      result.TotalTokens,
		TimeCost:         int(endTime.Sub(startTime).Milliseconds()),
	}

	c.JSON(http.StatusOK, ProbeResponse{
		Success: true,
		Data: &ProbeResponseData{
			Request:     mockRequest,
			Response:    ProbeResponseDetail{Content: result.Content, FinishReason: finishReason},
			Usage:       usage,
			CurlCommand: curlCommand,
		},
	})
}

func (s *Server) UseIndexHTML(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	f, err := assets.WebDistAssets.Open("web/dist/index.html")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
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

// ValidateAuthToken validates an authentication token without requiring auth
// This is used during login flow to verify a token before establishing session
func (s *Server) ValidateAuthToken(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"valid":   false,
		})
		return
	}

	// Extract token from "Bearer <token>" format
	tokenParts := strings.Split(authHeader, " ")
	if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"valid":   false,
		})
		return
	}

	token := tokenParts[1]

	// Check against global config user token
	cfg := s.config
	if cfg != nil && cfg.HasUserToken() {
		configToken := cfg.GetUserToken()

		// Direct token comparison
		if token == configToken || strings.TrimPrefix(token, "Bearer ") == configToken {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"valid":   true,
			})
			return
		}
	}

	// Token is invalid
	c.JSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"valid":   false,
	})
}

// GetUserToken returns the current user token (masked)
// Requires authentication
func (s *Server) GetUserToken(c *gin.Context) {
	token := s.config.GetUserToken()
	isDefault := token == constant.DefaultUserToken

	// Return full token - frontend will handle masking
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"token":      token,
			"is_default": isDefault,
		},
	})
}

// ResetUserToken generates a new secure random token and updates the configuration
// Requires authentication
func (s *Server) ResetUserToken(c *gin.Context) {
	newToken, err := config.GenerateSecureToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate token",
		})
		return
	}

	if err := s.config.SetUserToken(newToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save token",
		})
		return
	}

	logrus.Info("User token has been reset via web UI")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"token": newToken,
		},
	})
}

// ResetModelToken generates a new secure random model token and updates the configuration
// Requires authentication
func (s *Server) ResetModelToken(c *gin.Context) {
	newToken, err := config.GenerateSecureToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate token",
		})
		return
	}

	if err := s.config.SetModelToken(newToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save token",
		})
		return
	}

	logrus.Info("Model token has been reset via web UI")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"token": newToken,
		},
	})
}

func (s *Server) GetHistory(c *gin.Context) {
	response := HistoryResponse{
		Success: true,
	}

	if s.multiLogger != nil {
		actionLogger := s.multiLogger.WithSource(pkgobs.LogSourceAction)
		history := actionLogger.GetMemoryLatest(50)
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
	if strings.Contains(token, "...") {
		return token
	}

	// For very short tokens, mask all characters
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}

	// For longer tokens, show first 12 and last 4 characters
	// This works for both short and long tokens
	return token[:12] + "..." + token[len(token)-4:]
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
	logrus.WithFields(logrus.Fields{
		"action": obs.ActionStopServer,
		"source": "web_ui",
	}).Info("Server stopped via web interface")

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

	// Auth validation endpoint (no auth required) - for validating tokens before login
	apiAuth := manager.NewGroup("api", "v1", "")
	apiAuth.GET("/auth/validate", s.ValidateAuthToken,
		swagger.WithDescription("Validate authentication token"),
		swagger.WithTags("auth"),
		swagger.WithResponseModel(gin.H{}),
	)

	// Create authenticated API group
	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.getUserAuthMiddleware())
	apiV1.GET("/auth/token", s.GetUserToken,
		swagger.WithDescription("Get current user token (masked)"),
		swagger.WithTags("auth"),
		swagger.WithResponseModel(gin.H{}),
	)
	apiV1.POST("/auth/token/reset", s.ResetUserToken,
		swagger.WithDescription("Reset user token to a new secure random value"),
		swagger.WithTags("auth"),
		swagger.WithResponseModel(gin.H{}),
	)
	// Model token management endpoints (authenticated)
	apiV1.POST("/auth/model-token/reset", s.ResetModelToken,
		swagger.WithDescription("Reset model token to a new secure random value"),
		swagger.WithTags("auth"),
		swagger.WithResponseModel(gin.H{}),
	)

	apiV2 := manager.NewGroup("api", "v2", "")
	apiV2.Router.Use(s.getUserAuthMiddleware())

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

	apiV1.GET("/info/version/check", s.GetLatestVersion,
		swagger.WithTags("info"),
		swagger.WithDescription("Check if a newer version is available on GitHub"),
		swagger.WithResponseModel(LatestVersionResponse{}),
	)

	// Log API routes (HTTP request logs from memory)
	apiV1.GET("/log", s.GetLogs,
		swagger.WithDescription("Get HTTP request logs with optional filtering"),
		swagger.WithTags("logs"),
		swagger.WithResponseModel(LogsResponse{}),
	)
	apiV1.GET("/log/stats", s.GetLogStats,
		swagger.WithDescription("Get HTTP request log statistics"),
		swagger.WithTags("logs"),
	)
	apiV1.DELETE("/log", s.ClearLogs,
		swagger.WithDescription("Clear all HTTP request logs"),
		swagger.WithTags("logs"),
	)

	// System Log API routes (application logs from JSON file)
	apiV1.GET("/system/logs", s.GetSystemLogs,
		swagger.WithDescription("Get recent system logs with optional filtering (from JSON log file). Use 'limit' parameter to control how many recent entries to return."),
		swagger.WithTags("system-logs"),
		swagger.WithResponseModel(SystemLogsResponse{}),
	)
	apiV1.GET("/system/logs/stats", s.GetSystemLogStats,
		swagger.WithDescription("Get system log statistics"),
		swagger.WithTags("system-logs"),
	)
	apiV1.GET("/system/logs/level", s.GetSystemLogLevel,
		swagger.WithDescription("Get the current system log level"),
		swagger.WithTags("system-logs"),
	)
	apiV1.POST("/system/logs/level", s.SetSystemLogLevel,
		swagger.WithDescription("Set the minimum log level for system logs"),
		swagger.WithTags("system-logs"),
	)

	// Action History API routes (user operations/audit log)
	apiV1.GET("/actions/history", s.GetActionHistory,
		swagger.WithDescription("Get user action history from memory (recent operations)"),
		swagger.WithTags("actions"),
		swagger.WithResponseModel(ActionHistoryResponse{}),
	)
	apiV1.GET("/actions/stats", s.GetActionStats,
		swagger.WithDescription("Get statistics about user actions"),
		swagger.WithTags("actions"),
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

	// Create skill handler with skill manager
	// Initialize skill manager for skill locations
	skillManager, err := skill.NewSkillManager(s.config.ConfigDir)
	if err != nil {
		log.Printf("Failed to add skill api: %v", err)
		// Continue without skill manager - skill features will be disabled
	} else {
		handler := skill.NewHandler(skillManager)
		// Register routes from skill module
		skill.RegisterRoutes(apiV2, handler)
		log.Printf("Skill api initialized")
	}

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

	// Rule Management - register from rule module
	ruleHandler := rulemodule.NewHandler(s.config)
	rulemodule.RegisterRoutes(apiV1, ruleHandler)

	// Scenario Management - register from scenario module
	scenarioHandler := scenario.NewHandler(s.config, s)
	scenario.RegisterRoutes(apiV1, scenarioHandler)

	// Guardrails Management
	apiV1.GET("/guardrails/config", s.GetGuardrailsConfig,
		swagger.WithDescription("Get guardrails config content and parsed config"),
		swagger.WithTags("guardrails"),
	)
	apiV1.GET("/guardrails/builtins", s.GetGuardrailsBuiltins,
		swagger.WithDescription("Get curated builtin guardrails policy templates"),
		swagger.WithTags("guardrails"),
	)
	apiV1.GET("/guardrails/credentials", s.GetGuardrailsCredentials,
		swagger.WithDescription("List protected credentials used by guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
	)
	apiV1.GET("/guardrails/credential/:id", s.GetGuardrailsCredential,
		swagger.WithDescription("Get a protected credential for the local editor dialog"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/credential", s.CreateGuardrailsCredential,
		swagger.WithDescription("Create a protected credential for guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
	)
	apiV1.PUT("/guardrails/credential/:id", s.UpdateGuardrailsCredential,
		swagger.WithDescription("Update a protected credential for guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
	)
	apiV1.DELETE("/guardrails/credential/:id", s.DeleteGuardrailsCredential,
		swagger.WithDescription("Delete a protected credential for guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
	)
	apiV1.PUT("/guardrails/config", s.UpdateGuardrailsConfig,
		swagger.WithDescription("Update guardrails config and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.PUT("/guardrails/policy/:id", s.UpdateGuardrailsPolicy,
		swagger.WithDescription("Update a guardrails policy and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.DELETE("/guardrails/policy/:id", s.DeleteGuardrailsPolicy,
		swagger.WithDescription("Delete a guardrails policy and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/policy", s.CreateGuardrailsPolicy,
		swagger.WithDescription("Create a new guardrails policy and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.PUT("/guardrails/group/:id", s.UpdateGuardrailsGroup,
		swagger.WithDescription("Update a guardrails group and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.DELETE("/guardrails/group/:id", s.DeleteGuardrailsGroup,
		swagger.WithDescription("Delete a guardrails group and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/group", s.CreateGuardrailsGroup,
		swagger.WithDescription("Create a new guardrails group and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/reload", s.ReloadGuardrailsConfig,
		swagger.WithDescription("Reload guardrails config from disk"),
		swagger.WithTags("guardrails"),
	)
	apiV1.GET("/guardrails/history", s.GetGuardrailsHistory,
		swagger.WithDescription("Get recent guardrails interception history"),
		swagger.WithTags("guardrails"),
	)
	apiV1.DELETE("/guardrails/history", s.ClearGuardrailsHistory,
		swagger.WithDescription("Clear guardrails interception history"),
		swagger.WithTags("guardrails"),
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

	apiV1.POST("/probe/model/capability", s.HandleProbeModelEndpoints,
		swagger.WithDescription("Probe model endpoints (chat and responses) concurrently"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(ModelProbeRequest{}),
		swagger.WithResponseModel(ModelProbeResponse{}),
	)

	// Probe V2 endpoints (new unified probe API)
	apiV2.POST("/probe", s.HandleProbeV2,
		swagger.WithDescription("Probe V2 - Unified probe endpoint for testing rules and providers"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(ProbeV2Request{}),
		swagger.WithResponseModel(ProbeV2Response{}),
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
		swagger.WithQuery("force", "bool", "Force to add without checking"),
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

	// Provider template endpoints - register from providertemplate module
	providerTemplateHandler := providertemplate.NewHandler(s.templateManager)
	providertemplate.RegisterRoutes(api, providerTemplateHandler)
}

func (s *Server) useWebStaticEndpoints(engine *gin.Engine) {
	// Load templates and static files on the main engine - try embedded first
	log.Printf("Using embedded assets on main server")

	// Serve static assets from embedded filesystem
	st, _ := fs.Sub(assets.WebDistAssets, "web/dist/assets")
	engine.StaticFS("/assets", http.FS(st))

	// SPA catch-all - must be registered LAST
	// Serves index.html for all non-API frontend routes, letting React Router handle navigation
	// NoRoute handles unmatched paths including nested routes like /provider/settings/detail/123
	engine.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes - let them return 404s
		path := c.Request.URL.Path
		// Check if this looks like an API route
		if path == "" || strings.HasPrefix(path, "/api/v") || strings.HasPrefix(path, "/v") || strings.HasPrefix(path, "/openai") || strings.HasPrefix(path, "/anthropic") || strings.HasPrefix(path, "/tingly") {
			// This looks like an API route, return 404
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "API endpoint not found",
					"type":    "invalid_request_error",
					"code":    "not_found",
				},
			})
			c.Abort()
			return
		}

		s.UseIndexHTML(c)
	})
}

// GetShutdownChannel returns the shutdown channel for the main process to listen on
func GetShutdownChannel() <-chan struct{} {
	return shutdownChan
}

func init() {
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".png", "image/png")
}
