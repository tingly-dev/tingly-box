package server

import (
	"fmt"
	"net/http"
	"strings"

	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
)

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

	err = s.config.SetModelToken(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to save token: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	response := struct {
		Success bool          `json:"success"`
		Data    TokenResponse `json:"data"`
	}{
		Success: true,
		Data:    TokenResponse{Token: token, Type: "Bearer"},
	}

	c.JSON(http.StatusOK, response)
}

// GetToken handles token retrieval requests - generates a token if it doesn't exist
func (s *Server) GetToken(c *gin.Context) {
	globalConfig := s.config

	// Check if token already exists
	if globalConfig != nil && globalConfig.HasModelToken() {
		token := globalConfig.GetModelToken()
		c.JSON(http.StatusOK, gin.H{
			"token": token,
			"type":  "Bearer",
		})
		return
	}

	// Generate a new token if it doesn't exist
	// Use a default client ID for automatic token generation
	clientID := "auto-generated"
	token, err := s.jwtManager.GenerateToken(clientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to generate token: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Save the token to config
	err = globalConfig.SetModelToken(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to save token: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	response := struct {
		Success bool          `json:"success"`
		Data    TokenResponse `json:"data"`
	}{
		Success: true,
		Data:    TokenResponse{Token: token, Type: "Bearer"},
	}

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

// UserAuth middleware for UI and control API authentication
func (s *Server) UserAuth() gin.HandlerFunc {
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

		// Check against global config user token first
		globalConfig := s.config
		if globalConfig != nil && globalConfig.HasUserToken() {
			configToken := globalConfig.GetUserToken()

			// Remove "Bearer " prefix if present in the token
			if strings.HasPrefix(token, "Bearer ") {
				token = token[7:]
			}

			// Direct token comparison
			if token == configToken || strings.TrimPrefix(token, "Bearer ") == configToken {
				// Token matches the one in global config, allow access
				c.Set("client_id", "user_authenticated")
				c.Next()
				return
			}
		}

		// If not matching global config user token, validate as JWT token
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

// ModelAuth middleware for OpenAI and Anthropic API authentication
// The auth will support both `Authorization` and `X-Api-Key`
func (s *Server) ModelAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		xApiKey := c.GetHeader("X-Api-Key")
		if authHeader == "" && xApiKey == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Authorization header required",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		token := authHeader
		// Remove "Bearer " prefix if present in the token
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		}

		// Check against global config model token first
		globalConfig := s.config
		if globalConfig != nil && globalConfig.HasModelToken() {
			configToken := globalConfig.GetModelToken()

			// Direct token comparison
			if token == configToken || xApiKey == configToken {
				// Token matches the one in global config, allow access
				c.Set("client_id", "model_authenticated")
				c.Next()
				return
			}
		}

		// If not matching global config model token, validate as JWT token
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

// AuthenticateMiddleware returns the JWT authentication middleware (for backward compatibility)
func (s *Server) AuthenticateMiddleware() gin.HandlerFunc {
	// For backward compatibility, use UserAuth
	return s.UserAuth()
}

// DetermineProviderAndModel resolves the model name and finds the appropriate provider using load balancing
func (s *Server) DetermineProviderAndModel(modelName string) (*config.Provider, *config.Service, *config.Rule, error) {
	// Check if this is the request model name first
	c := s.config
	if c != nil && c.IsRequestModel(modelName) {
		// Get the Rule for this specific request model
		uuid := c.GetUUIDByRequestModel(modelName)
		rule := c.GetRequestConfigByRequestModel(uuid)
		if rule != nil && rule.Active {
			// Use the load balancer to select service
			selectedService, err := s.loadBalancer.SelectService(rule)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to select service: %w", err)
			}

			if selectedService == nil {
				return nil, nil, nil, fmt.Errorf("no available service for request model '%s'", modelName)
			}

			// Verify the provider exists and is enabled
			provider, err := c.GetProvider(selectedService.Provider)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("provider '%s' not found: %w", selectedService.Provider, err)
			}

			if !provider.Enabled {
				return nil, nil, nil, fmt.Errorf("provider '%s' is not enabled", selectedService.Provider)
			}

			// Update the current service index for the rule
			s.loadBalancer.UpdateServiceIndex(rule, selectedService)

			// Return provider, selected service, and rule
			return provider, selectedService, rule, nil
		}
		return nil, nil, nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
	}

	return nil, nil, nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
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
