package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/info"
	"github.com/tingly-dev/tingly-box/internal/server/module/onboarding"
	probemodule "github.com/tingly-dev/tingly-box/internal/server/module/probe"
	providermodule "github.com/tingly-dev/tingly-box/internal/server/module/provider"
	"github.com/tingly-dev/tingly-box/internal/server/module/providertemplate"
	rulemodule "github.com/tingly-dev/tingly-box/internal/server/module/rule"
	"github.com/tingly-dev/tingly-box/internal/server/module/scenario"
	"github.com/tingly-dev/tingly-box/internal/server/module/skill"
	"github.com/tingly-dev/tingly-box/swagger"
)

// UseWebAPIEndpoints configures API routes for web UI using swagger manager
func (s *Server) UseWebAPIEndpoints(manager *swagger.RouteManager) {
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

	// Info endpoints: health (unauthenticated) + config/version (authenticated)
	infoHandler := info.NewHandler(s.version, s.config.ConfigFile, s.config.ConfigDir)
	info.RegisterRoutes(apiAuth, apiV1, infoHandler)

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

	// Log API routes (HTTP request logs from memory)
	apiV1.GET("/log", s.controlHandler.GetLogs,
		swagger.WithDescription("Get HTTP request logs with optional filtering"),
		swagger.WithTags("logs"),
		swagger.WithResponseModel(LogsResponse{}),
	)
	apiV1.GET("/log/stats", s.controlHandler.GetLogStats,
		swagger.WithDescription("Get HTTP request log statistics"),
		swagger.WithTags("logs"),
	)
	apiV1.DELETE("/log", s.controlHandler.ClearLogs,
		swagger.WithDescription("Clear all HTTP request logs"),
		swagger.WithTags("logs"),
	)

	// System Log API routes (application logs from JSON file)
	apiV1.GET("/system/logs", s.controlHandler.GetSystemLogs,
		swagger.WithDescription("Get recent system logs with optional filtering (from JSON log file). Use 'limit' parameter to control how many recent entries to return."),
		swagger.WithTags("system-logs"),
		swagger.WithResponseModel(SystemLogsResponse{}),
	)
	apiV1.GET("/system/logs/stats", s.controlHandler.GetSystemLogStats,
		swagger.WithDescription("Get system log statistics"),
		swagger.WithTags("system-logs"),
	)
	apiV1.GET("/system/logs/level", s.controlHandler.GetSystemLogLevel,
		swagger.WithDescription("Get the current system log level"),
		swagger.WithTags("system-logs"),
	)
	apiV1.POST("/system/logs/level", s.controlHandler.SetSystemLogLevel,
		swagger.WithDescription("Set the minimum log level for system logs"),
		swagger.WithTags("system-logs"),
	)

	// Model Request routes (correlated per-request traces across pipeline stages)
	apiV1.GET("/requests", s.controlHandler.GetModelRequests,
		swagger.WithDescription("List recent model requests, one row per correlation id, joining the HTTP access log, model-request stage logs and smart-routing traces. Supports 'limit', 'scenario', 'provider' and 'status' filters."),
		swagger.WithTags("requests"),
		swagger.WithResponseModel(ModelRequestsResponse{}),
	)
	apiV1.GET("/requests/:id", s.controlHandler.GetModelRequestDetail,
		swagger.WithDescription("Get the full, time-ordered event timeline for a single model request by correlation id."),
		swagger.WithTags("requests"),
		swagger.WithResponseModel(ModelRequestDetail{}),
	)

	// Action History API routes (user operations/audit log)
	apiV1.GET("/actions/history", s.controlHandler.GetActionHistory,
		swagger.WithDescription("Get user action history from memory (recent operations)"),
		swagger.WithTags("actions"),
		swagger.WithResponseModel(ActionHistoryResponse{}),
	)
	apiV1.GET("/actions/stats", s.controlHandler.GetActionStats,
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
	apiV1.GET("/status", s.controlHandler.GetStatus,
		swagger.WithDescription("Get server status and statistics"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(StatusResponse{}),
	)

	apiV1.POST("/server/start", s.controlHandler.StartServer,
		swagger.WithDescription("Start the server"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	apiV1.POST("/server/stop", s.StopServer,
		swagger.WithDescription("Stop the server gracefully"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	apiV1.POST("/server/restart", s.controlHandler.RestartServer,
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
	apiV1.GET("/guardrails/config", s.guardrailsHandler.GetGuardrailsConfig,
		swagger.WithDescription("Get guardrails config content and parsed config"),
		swagger.WithTags("guardrails"),
	)
	apiV1.GET("/guardrails/builtins", s.guardrailsHandler.GetGuardrailsBuiltins,
		swagger.WithDescription("Get curated builtin guardrails policies"),
		swagger.WithTags("guardrails"),
	)
	apiV1.GET("/guardrails/registry", s.guardrailsHandler.GetGuardrailsRegistry,
		swagger.WithDescription("List downloadable guardrails policies from a remote registry"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/registry/install", s.guardrailsHandler.InstallGuardrailsRegistryPolicy,
		swagger.WithDescription("Download a guardrails policy from a remote registry into the local guardrails directory"),
		swagger.WithTags("guardrails"),
	)
	apiV1.GET("/guardrails/credentials", s.guardrailsHandler.GetGuardrailsCredentials,
		swagger.WithDescription("List protected credentials used by guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
	)
	apiV1.GET("/guardrails/credential/:id", s.guardrailsHandler.GetGuardrailsCredential,
		swagger.WithDescription("Get a protected credential for the local editor dialog"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/credential", s.guardrailsHandler.CreateGuardrailsCredential,
		swagger.WithDescription("Create a protected credential for guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
	)
	apiV1.PUT("/guardrails/credential/:id", s.guardrailsHandler.UpdateGuardrailsCredential,
		swagger.WithDescription("Update a protected credential for guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
	)
	apiV1.DELETE("/guardrails/credential/:id", s.guardrailsHandler.DeleteGuardrailsCredential,
		swagger.WithDescription("Delete a protected credential for guardrails pseudonymization"),
		swagger.WithTags("guardrails"),
	)
	apiV1.PUT("/guardrails/config", s.guardrailsHandler.UpdateGuardrailsConfig,
		swagger.WithDescription("Update guardrails config and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/fragment/import", s.guardrailsHandler.ImportGuardrailsFragment,
		swagger.WithDescription("Import one or more guardrails policies into the shared custom fragment"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/fragment/export", s.guardrailsHandler.ExportGuardrailsFragments,
		swagger.WithDescription("Export one or more imported guardrails policy fragments"),
		swagger.WithTags("guardrails"),
	)
	apiV1.PUT("/guardrails/policy/:id", s.guardrailsHandler.UpdateGuardrailsPolicy,
		swagger.WithDescription("Update a guardrails policy and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.DELETE("/guardrails/policy/:id", s.guardrailsHandler.DeleteGuardrailsPolicy,
		swagger.WithDescription("Delete a guardrails policy and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/policy", s.guardrailsHandler.CreateGuardrailsPolicy,
		swagger.WithDescription("Create a new guardrails policy and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.PUT("/guardrails/group/:id", s.guardrailsHandler.UpdateGuardrailsGroup,
		swagger.WithDescription("Update a guardrails group and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.DELETE("/guardrails/group/:id", s.guardrailsHandler.DeleteGuardrailsGroup,
		swagger.WithDescription("Delete a guardrails group and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/group", s.guardrailsHandler.CreateGuardrailsGroup,
		swagger.WithDescription("Create a new guardrails group and reload engine"),
		swagger.WithTags("guardrails"),
	)
	apiV1.POST("/guardrails/reload", s.guardrailsHandler.ReloadGuardrailsConfig,
		swagger.WithDescription("Reload guardrails config from disk"),
		swagger.WithTags("guardrails"),
	)
	apiV1.GET("/guardrails/history", s.guardrailsHandler.GetGuardrailsHistory,
		swagger.WithDescription("Get recent guardrails interception history"),
		swagger.WithTags("guardrails"),
	)
	apiV1.DELETE("/guardrails/history", s.guardrailsHandler.ClearGuardrailsHistory,
		swagger.WithDescription("Clear guardrails interception history"),
		swagger.WithTags("guardrails"),
	)

	// History
	apiV1.GET("/history", s.controlHandler.GetHistory,
		swagger.WithDescription("Get request history"),
		swagger.WithTags("history"),
		swagger.WithResponseModel(HistoryResponse{}),
	)

	// Onboarding: extract URLs and possible API tokens from arbitrary pasted
	// text. Vendor-agnostic — the user picks which URL/token to use.
	onboardingHandler := onboarding.NewHandler(onboarding.NewRuleExtractor())
	onboarding.RegisterRoutes(apiV1, onboardingHandler)

	// E2E + lightweight probe endpoints
	probemodule.RegisterRoutes(apiV2, probemodule.NewHandler(s.probeE2EService, s.probeLightweight))

	// Token Management
	apiV1.POST("/token", s.controlHandler.GenerateToken,
		swagger.WithDescription("Generate a new API token"),
		swagger.WithTags("token"),
		swagger.WithRequestModel(GenerateTokenRequest{}),
		swagger.WithResponseModel(TokenResponse{}),
	)

	apiV1.GET("/token", s.controlHandler.GetToken,
		swagger.WithDescription("Get existing API token or generate new one"),
		swagger.WithTags("token"),
		swagger.WithResponseModel(TokenResponse{}),
	)

	// Setup Swagger and OpenAPI documentation endpoints
	// - /swagger.json (Swagger 2.0)
	// - /openapi.json (OpenAPI 3.0)
	manager.SetupOpenAPIEndpoints()

	// Provider CRUD + model management + provider export / import
	providerHandler := providermodule.NewHandler(s.config, s.quotaManager)
	providermodule.RegisterRoutes(apiV2, providerHandler)

	// Provider template endpoints
	providerTemplateHandler := providertemplate.NewHandler(s.templateManager)
	providertemplate.RegisterRoutes(apiV2, providerTemplateHandler)
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
	newToken, err := config.GenerateUserToken()
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
	newToken, err := config.GenerateModelToken()
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
