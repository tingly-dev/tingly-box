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

	// Middleware
	s.engine.Use(gin.Logger())
	s.engine.Use(gin.Recovery())

	s.engine.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
	})

	// Dashboard endpoints

	// UI page routes
	s.engine.GET("/home", s.UseIndex)
	s.engine.GET("/credential", s.UseIndex)
	s.engine.GET("/rule", s.UseIndex)
	s.engine.GET("/system", s.UseIndex)
	s.engine.GET("/history", s.UseIndex)

	// API routes (for web UI functionality)
	s.useWebAPIEndpoints(s.engine)

	// Static files and templates - try embedded assets first, fallback to filesystem
	s.useWebStaticEndpoints(s.engine)
}

// HandleProbe tests a rule configuration by sending a sample request to the configured provider
func (s *Server) HandleProbe(c *gin.Context) {

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

func (s *Server) UseIndex(c *gin.Context) {
	c.FileFromFS("web/dist/index.html", http.FS(assets.WebDistAssets))
}

// API Handlers (exported for server integration)
func (s *Server) GetProviders(c *gin.Context) {
	providers := s.config.ListProviders()

	// Mask tokens for security
	maskedProviders := make([]ProviderResponse, len(providers))

	for i, provider := range providers {
		maskedProviders[i] = ProviderResponse{
			Name:     provider.Name,
			APIBase:  provider.APIBase,
			APIStyle: string(provider.APIStyle),
			Token:    maskToken(provider.Token),
			Enabled:  provider.Enabled,
		}
	}

	response := ProvidersResponse{
		Success: true,
		Data:    maskedProviders,
	}

	c.JSON(http.StatusOK, response)
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

	response := RulesResponse{
		Success: true,
		Data:    rules,
	}

	c.JSON(http.StatusOK, response)
}

// GetRule returns a specific rule by name
func (s *Server) GetRule(c *gin.Context) {
	ruleUUID := c.Param("uuid")
	if ruleUUID == "" {
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

	rule := cfg.GetRequestConfigByRequestModel(ruleUUID)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Rule not found",
		})
		return
	}

	response := RuleResponse{
		Success: true,
		Data:    rule,
	}

	c.JSON(http.StatusOK, response)
}

// SetRule creates or updates a rule
func (s *Server) SetRule(c *gin.Context) {
	ruleUUID := c.Param("uuid")
	if ruleUUID == "" {
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
		s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
			"name": ruleUUID,
		}, true, fmt.Sprintf("Rule %s updated successfully", ruleUUID))
	}

	response := SetRuleResponse{
		Success: true,
		Message: "Rule saved successfully",
	}
	response.Data.RequestModel = rule.RequestModel
	response.Data.ResponseModel = rule.ResponseModel
	response.Data.Provider = rule.GetDefaultProvider()
	response.Data.DefaultModel = rule.GetDefaultModel()
	response.Data.Active = rule.Active

	c.JSON(http.StatusOK, response)
}

func (s *Server) DeleteRule(c *gin.Context) {
	ruleUUID := c.Param("uuid")
	if ruleUUID == "" {
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

	err := cfg.DeleteRule(ruleUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete rule: " + err.Error(),
		})
		return
	}

	response := DeleteRuleResponse{
		Success: true,
		Message: "Rule deleted successfully",
	}

	c.JSON(http.StatusOK, response)
}

// AddProvider adds a new provider
func (s *Server) AddProvider(c *gin.Context) {
	var req AddProviderRequest

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
			s.logger.LogAction(obs.ActionAddProvider, map[string]interface{}{
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
		s.logger.LogAction(obs.ActionAddProvider, map[string]interface{}{
			"name":     req.Name,
			"api_base": req.APIBase,
		}, true, fmt.Sprintf("Provider %s added successfully", req.Name))
	}

	response := AddProviderResponse{
		Success: true,
		Message: "Provider added successfully",
		Data:    provider,
	}

	c.JSON(http.StatusOK, response)
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
			s.logger.LogAction(obs.ActionDeleteProvider, map[string]interface{}{
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
		s.logger.LogAction(obs.ActionDeleteProvider, map[string]interface{}{
			"name": providerName,
		}, true, fmt.Sprintf("Provider %s deleted successfully", providerName))
	}

	response := DeleteProviderResponse{
		Success: true,
		Message: "Provider deleted successfully",
	}

	c.JSON(http.StatusOK, response)
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

	var req UpdateProviderRequest

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
	if req.Name != nil {
		provider.Name = *req.Name
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
			s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
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
		s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
			"name": providerName,
		}, true, fmt.Sprintf("Provider %s updated successfully", providerName))
	}

	// Return masked provider data
	responseProvider := ProviderResponse{
		Name:     provider.Name,
		APIBase:  provider.APIBase,
		APIStyle: string(provider.APIStyle),
		Token:    maskToken(provider.Token),
		Enabled:  provider.Enabled,
	}

	response := UpdateProviderResponse{
		Success: true,
		Message: "Provider updated successfully",
		Data:    responseProvider,
	}

	c.JSON(http.StatusOK, response)
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
	responseProvider := ProviderResponse{
		Name:     provider.Name,
		APIBase:  provider.APIBase,
		APIStyle: string(provider.APIStyle),
		Token:    maskToken(provider.Token),
		Enabled:  provider.Enabled,
	}

	response := struct {
		Success bool             `json:"success"`
		Data    ProviderResponse `json:"data"`
	}{
		Success: true,
		Data:    responseProvider,
	}

	c.JSON(http.StatusOK, response)
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
			s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
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
		s.logger.LogAction(obs.ActionUpdateProvider, map[string]interface{}{
			"name":    providerName,
			"enabled": provider.Enabled,
		}, true, fmt.Sprintf("Provider %s %s successfully", providerName, action))
	}

	response := ToggleProviderResponse{
		Success: true,
		Message: fmt.Sprintf("Provider %s %s successfully", providerName, action),
	}
	response.Data.Enabled = provider.Enabled

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
			s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
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
		s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
			"provider":     providerName,
			"models_count": len(models),
		}, true, fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), providerName))
	}

	response := FetchProviderModelsResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully fetched %d models for provider %s", len(models), providerName),
		Data:    models,
	}

	c.JSON(http.StatusOK, response)
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
	providerModels := make(map[string]*ProviderModelInfo)

	for _, providerName := range providers {
		models := providerModelManager.GetModels(providerName)
		apiBase, lastUpdated, _ := providerModelManager.GetProviderInfo(providerName)

		providerModels[providerName] = &ProviderModelInfo{
			Models:      models,
			APIBase:     apiBase,
			LastUpdated: lastUpdated,
		}
	}

	response := ProviderModelsResponse{
		Success: true,
		Data:    providerModels,
	}

	c.JSON(http.StatusOK, response)
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

	// Provider Management
	authAPI.GET("/providers", (s.GetProviders),
		swagger.WithDescription("Get all configured providers with masked tokens"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProvidersResponse{}),
	)

	authAPI.GET("/providers/:name", (s.GetProvider),
		swagger.WithDescription("Get specific provider details with masked token"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ProviderResponse{}),
	)

	authAPI.POST("/providers", (s.AddProvider),
		swagger.WithDescription("Add a new provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithRequestModel(AddProviderRequest{}),
		swagger.WithResponseModel(AddProviderResponse{}),
	)

	authAPI.PUT("/providers/:name", (s.UpdateProvider),
		swagger.WithDescription("Update existing provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithRequestModel(UpdateProviderRequest{}),
		swagger.WithResponseModel(UpdateProviderResponse{}),
	)

	authAPI.POST("/providers/:name/toggle", (s.ToggleProvider),
		swagger.WithDescription("Toggle provider enabled/disabled status"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(ToggleProviderResponse{}),
	)

	authAPI.DELETE("/providers/:name", (s.DeleteProvider),
		swagger.WithDescription("Delete a provider configuration"),
		swagger.WithTags("providers"),
		swagger.WithResponseModel(DeleteProviderResponse{}),
	)

	// Server Management
	authAPI.GET("/status", (s.GetStatus),
		swagger.WithDescription("Get server status and statistics"),
		swagger.WithTags("server"),
		swagger.WithResponseModel(StatusResponse{}),
	)

	authAPI.POST("/server/start", (s.StartServer),
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
	authAPI.POST("/probe", (s.HandleProbe),
		swagger.WithDescription("Test a rule configuration by sending a sample request"),
		swagger.WithTags("testing"),
		swagger.WithRequestModel(ProbeRequest{}),
		swagger.WithResponseModel(ProbeResponse{}),
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
