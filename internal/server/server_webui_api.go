package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/imagegen"
	"github.com/tingly-dev/tingly-box/internal/server/module/info"
	"github.com/tingly-dev/tingly-box/internal/server/module/onboarding"
	probemodule "github.com/tingly-dev/tingly-box/internal/server/module/probe"
	providermodule "github.com/tingly-dev/tingly-box/internal/server/module/provider"
	"github.com/tingly-dev/tingly-box/internal/server/module/providercatalog"
	rulemodule "github.com/tingly-dev/tingly-box/internal/server/module/rule"
	"github.com/tingly-dev/tingly-box/internal/server/module/scenario"
	shortcutmodule "github.com/tingly-dev/tingly-box/internal/server/module/shortcut"
	"github.com/tingly-dev/tingly-box/internal/server/module/skill"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	"github.com/tingly-dev/tingly-box/swagger"
)

// UseWebAPIEndpoints configures the core API routes for the web UI using the
// swagger manager and returns the shared route groups so the caller can mount
// further modules on them (see apiModules).
func (s *Server) UseWebAPIEndpoints(manager *swagger.RouteManager) *module.Routes {
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

	// CORS (including OPTIONS preflight) is applied once, engine-wide, by
	// setupMiddleware; routes registered here inherit it.

	// Auth validation endpoint (no auth required) - for validating tokens before login
	apiAuth := manager.NewGroup("api", "v1", "")
	apiAuth.GET("/auth/validate", s.ValidateAuthToken,
		swagger.WithDescription("Validate authentication token"),
		swagger.WithTags("auth"),
		swagger.WithResponseModel(gin.H{}),
	)

	// Create authenticated API groups
	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.getUserAuthMiddleware())
	apiV2 := manager.NewGroup("api", "v2", "")
	apiV2.Router.Use(s.getUserAuthMiddleware())

	rt := &module.Routes{Public: apiAuth, V1: apiV1, V2: apiV2, Engine: s.engine, Manager: manager, UserAuth: s.getUserAuthMiddleware()}

	// Info endpoints: health (unauthenticated) + config/version (authenticated)
	infoHandler := info.NewHandler(s.version, s.config.ConfigFile, s.config.ConfigDir, s.launchSource)
	module.Mount(rt, infoHandler)

	// Desktop / start-menu shortcut creation (authenticated) — see
	// .design/shortcut.md §6. Not shown to Wails GUI users on the frontend
	// side; the endpoint itself is harmless to call from any runtime mode.
	shortcutHandler := shortcutmodule.NewHandler(s.launchSource, s.version)
	module.Mount(rt, shortcutHandler)

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

	// Log API routes (HTTP request logs from memory)
	apiV1.GET("/log", s.webHandler.GetLogs,
		swagger.WithDescription("Get HTTP request logs with optional filtering"),
		swagger.WithTags("logs"),
		swagger.WithResponseModel(LogsResponse{}),
	)
	apiV1.GET("/log/stats", s.webHandler.GetLogStats,
		swagger.WithDescription("Get HTTP request log statistics"),
		swagger.WithTags("logs"),
	)
	apiV1.DELETE("/log", s.webHandler.ClearLogs,
		swagger.WithDescription("Clear all HTTP request logs"),
		swagger.WithTags("logs"),
	)

	// Trace API routes (in-memory span store — default trace egress, see
	// .design/otel.md §7.4; log entries link here via their trace_id field)
	apiV1.GET("/traces/:trace_id", s.GetTrace,
		swagger.WithDescription("Get the spans of one trace by trace id. 404 means the trace was never sampled or has been evicted from the in-memory buffer."),
		swagger.WithTags("traces"),
		swagger.WithResponseModel(TraceDetailResponse{}),
	)

	// System Log API routes (application logs from JSON file)
	apiV1.GET("/system/logs", s.webHandler.GetSystemLogs,
		swagger.WithDescription("Get recent system logs with optional filtering (from JSON log file). Use 'limit' parameter to control how many recent entries to return."),
		swagger.WithTags("system-logs"),
		swagger.WithQuery("limit", "integer", "Maximum number of recent entries (default 100, max 1000)"),
		swagger.WithResponseModel(SystemLogsResponse{}),
	)
	apiV1.GET("/system/logs/stats", s.webHandler.GetSystemLogStats,
		swagger.WithDescription("Get system log statistics"),
		swagger.WithTags("system-logs"),
	)
	apiV1.GET("/system/logs/level", s.webHandler.GetSystemLogLevel,
		swagger.WithDescription("Get the current system log level"),
		swagger.WithTags("system-logs"),
		swagger.WithResponseModel(SystemLogLevelResponse{}),
	)
	apiV1.POST("/system/logs/level", s.webHandler.SetSystemLogLevel,
		swagger.WithDescription("Set the minimum log level for system logs"),
		swagger.WithTags("system-logs"),
		swagger.WithRequestModel(SystemLogLevelRequest{}),
		swagger.WithResponseModel(SystemLogLevelResponse{}),
	)

	// Model Request routes (correlated per-request traces across pipeline stages)
	apiV1.GET("/requests", s.webHandler.GetModelRequests,
		swagger.WithDescription("List recent model requests, one row per correlation id, joining the HTTP access log, model-request stage logs and smart-routing traces. Supports 'limit', 'scenario', 'provider' and 'status' filters."),
		swagger.WithTags("requests"),
		swagger.WithQuery("limit", "integer", "Maximum number of requests (default 100, max 1000)"),
		swagger.WithQuery("scenario", "string", "Exact scenario filter"),
		swagger.WithQuery("provider", "string", "Exact provider filter"),
		swagger.WithQuery("status", "string", "Exact HTTP status filter"),
		swagger.WithResponseModel(ModelRequestsResponse{}),
	)
	apiV1.GET("/requests/:id", s.webHandler.GetModelRequestDetail,
		swagger.WithDescription("Get the full, time-ordered event timeline for a single model request by correlation id."),
		swagger.WithTags("requests"),
		swagger.WithResponseModel(ModelRequestDetail{}),
	)

	// Action History API routes (user operations/audit log)
	apiV1.GET("/actions/history", s.webHandler.GetActionHistory,
		swagger.WithDescription("Get user action history from memory (recent operations)"),
		swagger.WithTags("actions"),
		swagger.WithResponseModel(ActionHistoryResponse{}),
	)
	apiV1.GET("/actions/stats", s.webHandler.GetActionStats,
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
		module.Mount(rt, handler)
		log.Printf("Skill api initialized")
	}

	// Server Management
	apiV1.GET("/status", s.webHandler.GetStatus,
		swagger.WithDescription("Get server status and statistics"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(StatusResponse{}),
	)

	apiV1.POST("/server/start", s.webHandler.StartServer,
		swagger.WithDescription("Start the server"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	apiV1.POST("/server/stop", s.StopServer,
		swagger.WithDescription("Stop the server gracefully"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	apiV1.POST("/server/restart", s.webHandler.RestartServer,
		swagger.WithDescription("Restart the server"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(ServerActionResponse{}),
	)

	// Rule Management - register from rule module
	ruleHandler := rulemodule.NewHandler(s.config)
	module.Mount(rt, ruleHandler)

	// Scenario Management - register from scenario module
	// Route previews for the model-tier listing; PreviewRoute needs only the
	// config and the load balancer, never the status line's cache or quota.
	scenarioHandler := scenario.NewHandler(s.config, s).
		WithRoutePreview(statusline.NewHandler(s.config, s.loadBalancer, statusline.NewCache(), nil))
	module.Mount(rt, scenarioHandler)

	// Image generation (authenticated): the output directory, the image archive
	// under it, and the focus workbenches built on the archive.
	imagegenHandler := imagegen.NewHandler(s.config.ConfigDir)
	module.Mount(rt, imagegenHandler)

	// Guardrails admin API
	module.Mount(rt, s.guardrailsHandler)

	// History
	apiV1.GET("/history", s.webHandler.GetHistory,
		swagger.WithDescription("Get request history"),
		swagger.WithTags("history"),
		swagger.WithResponseModel(HistoryResponse{}),
	)

	// Onboarding: extract URLs and possible API tokens from arbitrary pasted
	// text. Vendor-agnostic — the user picks which URL/token to use.
	onboardingHandler := onboarding.NewHandler(onboarding.NewRuleExtractor())
	module.Mount(rt, onboardingHandler)

	// E2E + lightweight probe endpoints
	module.Mount(rt, probemodule.NewHandler(s.probeE2e, s.probeLight))

	// Token Management
	apiV1.POST("/token", s.webHandler.GenerateToken,
		swagger.WithDescription("Generate a new API token"),
		swagger.WithTags("token"),
		swagger.WithRequestModel(GenerateTokenRequest{}),
		swagger.WithResponseModel(TokenResponse{}),
	)

	apiV1.GET("/token", s.webHandler.GetToken,
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
	module.Mount(rt, providerHandler)

	// Provider catalog endpoints
	providerCatalogHandler := providercatalog.NewHandler(s.templateManager)
	module.Mount(rt, providerCatalogHandler)

	return rt
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
		apierr.Failure(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	if err := s.config.SetUserToken(newToken); err != nil {
		apierr.Failure(c, http.StatusInternalServerError, "Failed to save token")
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
		apierr.Failure(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	if err := s.config.SetModelToken(newToken); err != nil {
		apierr.Failure(c, http.StatusInternalServerError, "Failed to save token")
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
