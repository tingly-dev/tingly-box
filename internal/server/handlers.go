package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

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

	token = "tingly-box-" + token
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
	token = "tingly-box-" + token
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
func (s *Server) determineProvider(model, explicitProvider string) (*typ.Provider, error) {
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

// DetermineProviderAndModelWithScenario
func (s *Server) DetermineProviderAndModelWithScenario(scenario typ.RuleScenario, modelName string, req interface{}) (*typ.Provider, *loadbalance.Service, *typ.Rule, error) {
	// Check if this is the request model name first
	c := s.config
	if c != nil && c.IsRequestModelInScenario(modelName, scenario) {
		// Get the Rule for this specific request model using the same method as middleware
		uuid := c.GetUUIDByRequestModelAndScenario(modelName, scenario)
		rules := c.GetRequestConfigs()
		var rule *typ.Rule
		for i := range rules {
			if rules[i].UUID == uuid && rules[i].Active {
				rule = &rules[i] // Get pointer to actual rule in config
				break
			}
		}

		if rule != nil && rule.Active {
			var selectedService *loadbalance.Service
			var err error

			// Smart routing: check if enabled and try to match rules
			if rule.SmartEnabled && len(rule.SmartRouting) > 0 && req != nil {
				logrus.Debugf("[smart_routing] smart routing enabled for model %s", modelName)

				// Extract context from request (type switch handles different request types)
				ctx, err := s.ExtractRequestContext(req)
				if err == nil && ctx != nil {
					// Create router and evaluate
					router, err := smartrouting.NewRouter(rule.SmartRouting)
					if err == nil {
						if matchedServices, matched := router.EvaluateRequest(ctx); matched && len(matchedServices) > 0 {
							logrus.Debugf("[smart_routing] rule matched for model %s, selecting from %d services", modelName, len(matchedServices))
							// Select service from matched services using load balancing
							selectedService, err = s.SelectServiceFromSmartRouting(matchedServices, rule)
							if err == nil && selectedService != nil {
								// Verify the provider exists and is enabled
								provider, err := c.GetProviderByUUID(selectedService.Provider)
								if err == nil && provider.Enabled {
									logrus.Infof("[smart_routing] using smart routed service: %s -> %s", provider.Name, selectedService.Model)
									return provider, selectedService, rule, nil
								}
							}
						} else {
							logrus.Debugf("[smart_routing] no rule matched, falling through to load balancer")
						}
					} else {
						logrus.Debugf("[smart_routing] failed to create router: %v", err)
					}
				} else {
					logrus.Debugf("[smart_routing] failed to extract context: %v", err)
				}
				// Fall through to normal load balancer on any error
			}

			// Normal load balancing path
			selectedService, err = s.loadBalancer.SelectService(rule)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to select service: %w", err)
			}

			if selectedService == nil {
				return nil, nil, nil, fmt.Errorf("no available service for request model '%s'", modelName)
			}

			// Verify the provider exists and is enabled
			provider, err := c.GetProviderByUUID(selectedService.Provider)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("provider '%s' not found: %w", selectedService.Provider, err)
			}

			if !provider.Enabled {
				return nil, nil, nil, fmt.Errorf("provider '%s' is not enabled", selectedService.Provider)
			}

			// Update the current service index for the rule
			s.loadBalancer.UpdateServiceIndex(rule, selectedService)

			// Persist the updated CurrentServiceID to SQLite (not config.json)
			// This is critical for round-robin to work correctly across requests
			if err := c.SaveCurrentServiceID(rule.UUID, rule.CurrentServiceID); err != nil {
				// Log error but don't fail the request
				fmt.Printf("Warning: failed to persist CurrentServiceID: %v\n", err)
			}

			// Return provider, selected service, and rule
			return provider, selectedService, rule, nil
		}
		return nil, nil, nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
	}

	return nil, nil, nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
}

// DetermineProviderAndModel resolves the model name and finds the appropriate provider using load balancing
func (s *Server) DetermineProviderAndModel(modelName string) (*typ.Provider, *loadbalance.Service, *typ.Rule, error) {
	// Check if this is the request model name first
	c := s.config
	if c != nil && c.IsRequestModel(modelName) {
		// Get the Rule for this specific request model using the same method as middleware
		uuid := c.GetUUIDByRequestModel(modelName)
		rules := c.GetRequestConfigs()
		var rule *typ.Rule
		for i := range rules {
			if rules[i].UUID == uuid && rules[i].Active {
				rule = &rules[i] // Get pointer to actual rule in config
				break
			}
		}

		if rule != nil && rule.Active {
			// Set the rule in the context so middleware can use the same rule
			// We need to pass this context to the actual HTTP handler, but this function
			// doesn't have access to the Gin context. For now, we'll use a different approach.

			// Use the load balancer to select service
			selectedService, err := s.loadBalancer.SelectService(rule)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to select service: %w", err)
			}

			if selectedService == nil {
				return nil, nil, nil, fmt.Errorf("no available service for request model '%s'", modelName)
			}

			// Verify the provider exists and is enabled
			provider, err := c.GetProviderByUUID(selectedService.Provider)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("provider '%s' not found: %w", selectedService.Provider, err)
			}

			if !provider.Enabled {
				return nil, nil, nil, fmt.Errorf("provider '%s' is not enabled", selectedService.Provider)
			}

			// Update the current service index for the rule
			s.loadBalancer.UpdateServiceIndex(rule, selectedService)

			// Persist the updated CurrentServiceID to SQLite (not config.json)
			// This is critical for round-robin to work correctly across requests
			if err := c.SaveCurrentServiceID(rule.UUID, rule.CurrentServiceID); err != nil {
				// Log error but don't fail the request
				fmt.Printf("Warning: failed to persist CurrentServiceID: %v\n", err)
			}

			// Return provider, selected service, and rule
			return provider, selectedService, rule, nil
		}
		return nil, nil, nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
	}

	return nil, nil, nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
}

// determineProviderFallback is the fallback logic for provider determination
func (s *Server) determineProviderFallback(model string) (*typ.Provider, error) {
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
