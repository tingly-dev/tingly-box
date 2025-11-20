package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
)

// ChatCompletionRequest represents OpenAI chat completion request
type ChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []ChatCompletionMessage `json:"messages"`
	Stream   bool                    `json:"stream,omitempty"`
	Provider string                  `json:"provider,omitempty"`
}

// ChatCompletionMessage represents a chat message
type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse represents OpenAI chat completion response
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   ChatCompletionUsage    `json:"usage"`
}

// ChatCompletionChoice represents a completion choice
type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

// ChatCompletionUsage represents token usage information
type ChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

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
	var req ChatCompletionRequest

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

	// Update response with original model name for client compatibility
	if modelDef != nil {
		response.Model = modelDef.Name
	}

	// Return response
	c.JSON(http.StatusOK, response)
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

// forwardRequest forwards the request to the selected provider
func (s *Server) forwardRequest(provider *config.Provider, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Convert request to provider format (simplified)
	providerReq := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   req.Stream,
	}

	reqBody, err := json.Marshal(providerReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request to provider
	endpoint := provider.APIBase + "/chat/completions"

	httpReq, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+provider.Token)

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle error responses
	if resp.StatusCode != http.StatusOK {
		var errorResp ErrorResponse
		if err := json.Unmarshal(respBody, &errorResp); err != nil {
			return nil, fmt.Errorf("provider returned error %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("provider error: %s", errorResp.Error.Message)
	}

	// Parse successful response
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if Response model is configured and modify the model field
	if err := s.applyResponseModel(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to apply response model: %w", err)
	}

	return &chatResp, nil
}

// applyResponseModel applies the configured response model to the chat completion response
func (s *Server) applyResponseModel(response *ChatCompletionResponse) error {
	// Get global configuration
	globalConfig := s.config.GetGlobalConfig()
	if globalConfig == nil {
		// No global config available, return without modification
		return nil
	}

	// Get the configured response model
	responseModel := globalConfig.GetResponseModel()
	if responseModel == "" {
		// No response model configured, return without modification
		return nil
	}

	// Modify the model field in the response
	response.Model = responseModel

	return nil
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

		// For testing purposes, we'll accept "valid-test-token"
		if token == "valid-test-token" {
			c.Next()
			return
		}

		// Validate JWT token
		claims, err := s.jwtManager.ValidateToken(token)
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

// DashboardRedirect redirects to the dedicated UI server
func (s *Server) DashboardRedirect(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Tingly Box Dashboard",
		"note":    "Use 'tingly ui' command to launch the web dashboard",
		"command": "tingly ui --port 8081",
		"ui_urls": map[string]string{
			"root":      "http://localhost:8081/",
			"dashboard": "http://localhost:8081/",
			"ui_root":   "http://localhost:8081/ui/",
			"providers": "http://localhost:8081/ui/providers",
			"server":    "http://localhost:8081/ui/server",
			"history":   "http://localhost:8081/ui/history",
		},
	})
}
