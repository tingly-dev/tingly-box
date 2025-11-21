package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type RequestWrapper = openai.ChatCompletionNewParams

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// HealthCheck handles health check requests
func (s *Server) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "tingly-box",
	})
}

// GenerateToken handles token generation requests
func (s *Server) GenerateToken(c *gin.Context) {
	var req struct {
		ClientID string `json:"client_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	token, err := s.jwtManager.GenerateToken(req.ClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to generate token: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"type":  "Bearer",
	})
}

// ChatCompletions handles OpenAI-compatible chat completion requests
func (s *Server) ChatCompletions(c *gin.Context) {
	var req RequestWrapper

	// Parse request body
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate required fields
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "At least one message is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Determine provider and model based on request
	provider, modelDef, err := s.DetermineProviderAndModel(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Update request with actual model name if we have model definition
	if modelDef != nil {
		req.Model = modelDef.Model
	}

	// Forward request to provider
	response, err := s.forwardRequest(provider, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Handle response model modification at JSON level
	globalConfig := s.config.GetGlobalConfig()
	responseModel := ""
	if globalConfig != nil {
		responseModel = globalConfig.GetResponseModel()
	}

	// Convert response to JSON map for modification
	responseJSON, err := json.Marshal(response)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to marshal response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to process response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Update response model if configured and we have model definition
	if responseModel != "" && modelDef != nil {
		responseMap["model"] = modelDef.Name
	}

	// Return modified response
	c.JSON(http.StatusOK, responseMap)
}

// determineProvider selects the appropriate provider based on model or explicit provider name
func (s *Server) determineProvider(model, explicitProvider string) (*config.Provider, error) {
	providers := s.config.ListProviders()

	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}

	// If explicit provider is specified, use it
	if explicitProvider != "" {
		for _, provider := range providers {
			if provider.Name == explicitProvider && provider.Enabled {
				return provider, nil
			}
		}
		return nil, fmt.Errorf("provider '%s' not found or disabled", explicitProvider)
	}

	// Otherwise, try to determine provider based on model name
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}

		// Simple model name matching - can be enhanced
		if strings.Contains(strings.ToLower(provider.APIBase), "openai") &&
			(strings.HasPrefix(strings.ToLower(model), "gpt") || strings.Contains(strings.ToLower(model), "openai")) {
			return provider, nil
		}
		if strings.Contains(strings.ToLower(provider.APIBase), "anthropic") &&
			strings.HasPrefix(strings.ToLower(model), "claude") {
			return provider, nil
		}
	}

	// If no specific match, return first enabled provider
	for _, provider := range providers {
		if provider.Enabled {
			return provider, nil
		}
	}

	return nil, fmt.Errorf("no enabled providers available")
}

// forwardRequest forwards the request to the selected provider using OpenAI library
func (s *Server) forwardRequest(provider *config.Provider, req *RequestWrapper) (*openai.ChatCompletion, error) {
	// Create OpenAI client with provider configuration
	client := openai.NewClient(
		option.WithAPIKey(provider.Token),
		option.WithBaseURL(provider.APIBase),
	)

	// Since RequestWrapper is a type alias to openai.ChatCompletionNewParams,
	// we can directly use it as the request parameters
	chatReq := *req

	// Make the request using OpenAI library
	chatCompletion, err := client.Chat.Completions.New(context.Background(), chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	return chatCompletion, nil
}

// ListModels handles the /v1/models endpoint (OpenAI compatible)
func (s *Server) ListModels(c *gin.Context) {
	if s.modelManager == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model manager not available",
				Type:    "internal_error",
			},
		})
		return
	}

	models := s.modelManager.GetAllModels()

	// Convert to OpenAI-compatible format
	var openaiModels []map[string]interface{}
	for _, model := range models {
		openaiModel := map[string]interface{}{
			"id":       model.Name,
			"object":   "model",
			"created":  time.Now().Unix(), // In a real implementation, use actual creation date
			"owned_by": "tingly-box",
		}

		// Add permission information
		var permissions []string
		if model.Category == "chat" {
			permissions = append(permissions, "chat.completions")
		}

		openaiModel["permission"] = permissions

		// Add aliases as metadata
		if len(model.Aliases) > 0 {
			openaiModel["metadata"] = map[string]interface{}{
				"provider":     model.Provider,
				"api_base":     model.APIBase,
				"actual_model": model.Model,
				"aliases":      model.Aliases,
				"description":  model.Description,
				"category":     model.Category,
			}
		} else {
			openaiModel["metadata"] = map[string]interface{}{
				"provider":     model.Provider,
				"api_base":     model.APIBase,
				"actual_model": model.Model,
				"description":  model.Description,
				"category":     model.Category,
			}
		}

		openaiModels = append(openaiModels, openaiModel)
	}

	response := map[string]interface{}{
		"object": "list",
		"data":   openaiModels,
	}

	c.JSON(http.StatusOK, response)
}

// AuthenticateMiddleware returns the JWT authentication middleware
func (s *Server) AuthenticateMiddleware() gin.HandlerFunc {
	return s.authenticateMiddleware()
}

// authenticateMiddleware provides JWT authentication
func (s *Server) authenticateMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Authorization header required",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>" format
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Invalid authorization header format. Expected: 'Bearer <token>'",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		token := tokenParts[1]

		// Check against global config token first
		globalConfig := s.config.GetGlobalConfig()
		if globalConfig != nil && globalConfig.HasToken() {
			configToken := globalConfig.GetToken()

			// Remove "Bearer " prefix if present in the token
			if strings.HasPrefix(token, "Bearer ") {
				token = token[7:]
			}

			// Direct token comparison
			if token == configToken || strings.TrimPrefix(token, "Bearer ") == configToken {
				// Token matches the one in global config, allow access
				c.Set("client_id", "authenticated")
				c.Next()
				return
			}
		}

		// If not matching global config token, validate as JWT token
		claims, err := s.jwtManager.ValidateAPIKey(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Invalid or expired token",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		// Store client ID in context
		c.Set("client_id", claims.ClientID)
		c.Next()
	}
}

// DetermineProviderAndModel resolves the model name and finds the appropriate provider
func (s *Server) DetermineProviderAndModel(modelName string) (*config.Provider, *config.ModelDefinition, error) {
	// Check if this is the request model name first
	globalConfig := s.config.GetGlobalConfig()
	if globalConfig != nil && globalConfig.IsRequestModel(modelName) {
		// Use default provider and model
		defaultProvider, defaultModel := globalConfig.GetDefaultProvider(), globalConfig.GetDefaultModel()
		if defaultProvider != "" && defaultModel != "" {
			// Find provider configuration
			providers := s.config.ListProviders()
			for _, p := range providers {
				if p.Enabled && p.Name == defaultProvider {
					// Create a mock model definition for the default model
					modelDef := &config.ModelDefinition{
						Name:     defaultModel,
						Provider: defaultProvider,
						Model:    defaultModel,
					}
					return p, modelDef, nil
				}
			}
			return nil, nil, fmt.Errorf("default provider '%s' is not enabled", defaultProvider)
		}
		return nil, nil, fmt.Errorf("default provider or model not configured")
	}

	if s.modelManager == nil {
		// Fallback to old logic if model manager is not available
		provider, err := s.determineProviderFallback(modelName)
		return provider, nil, err
	}

	// Find model definition
	modelDef, err := s.modelManager.FindModel(modelName)
	if err != nil {
		return nil, nil, fmt.Errorf("model not found: %w", err)
	}

	// Find provider configuration
	providers := s.config.ListProviders()
	var provider *config.Provider
	for _, p := range providers {
		if p.Enabled && strings.Contains(p.APIBase, modelDef.APIBase) {
			provider = p
			break
		}
	}

	if provider == nil {
		// Try to match by provider name
		for _, p := range providers {
			if p.Enabled && p.Name == modelDef.Provider {
				provider = p
				break
			}
		}
	}

	if provider == nil {
		return nil, modelDef, fmt.Errorf("no enabled provider found for model '%s' (provider: %s)", modelName, modelDef.Provider)
	}

	return provider, modelDef, nil
}

// determineProviderFallback is the fallback logic for provider determination
func (s *Server) determineProviderFallback(model string) (*config.Provider, error) {
	providers := s.config.ListProviders()

	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}

	// Simple model name matching
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}

		if strings.Contains(strings.ToLower(provider.APIBase), "openai") &&
			(strings.HasPrefix(strings.ToLower(model), "gpt") || strings.Contains(strings.ToLower(model), "openai")) {
			return provider, nil
		}
		if strings.Contains(strings.ToLower(provider.APIBase), "anthropic") &&
			strings.HasPrefix(strings.ToLower(model), "claude") {
			return provider, nil
		}
	}

	// Return first enabled provider
	for _, provider := range providers {
		if provider.Enabled {
			return provider, nil
		}
	}

	return nil, fmt.Errorf("no enabled providers available")
}
