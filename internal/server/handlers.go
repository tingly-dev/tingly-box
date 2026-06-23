package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/routing"
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

// resolveSessionID returns the session identifier for the current request as a string.
// It delegates to routing.ResolveSessionID which checks (in priority order):
// Anthropic metadata.user_id > X-Tingly-Session-ID header > ClientIP fallback.
func resolveSessionID(c *gin.Context, req interface{}) typ.SessionID {
	return routing.ResolveSessionID(c, req)
}

func (s *Server) determineRule(modelName string) (*typ.Rule, error) {
	c := s.config
	if c != nil && c.IsRequestModel(modelName) {

		// Get the Rule for this specific request model using the same method as middleware
		uuid := c.GetUUIDByRequestModel(modelName)
		rules := c.GetRequestConfigs()
		var rule *typ.Rule
		for i := range rules {
			if rules[i].UUID == uuid && rules[i].Active {
				rule = &rules[i] // Get pointer to actual rule in config
				return rule, nil
			}
		}
	}

	return nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
}

func isEnterpriseContextPresent(c *gin.Context) bool {
	return strings.TrimSpace(c.GetString("enterprise_user_id")) != ""
}

// probeSyntheticRuleUUID marks the throwaway rule built for an
// X-Tingly-Probe-Service request — it has no persisted identity.
const probeSyntheticRuleUUID = "probe-synthetic"

func (s *Server) determineRuleWithScenario(ctx *gin.Context, scenario typ.RuleScenario, modelName string) (*typ.Rule, error) {
	// X-Tingly-Probe-Rule: load a specific rule by UUID (for applying its flags
	// while service selection is overridden by X-Tingly-Probe-Service).
	if ruleUUID := ctx.GetHeader("X-Tingly-Probe-Rule"); ruleUUID != "" {
		if rule := s.config.GetRuleByUUID(ruleUUID); rule != nil {
			return rule, nil
		}
		return nil, fmt.Errorf("probe rule not found: %s", ruleUUID)
	}

	// X-Tingly-Probe-Service: no matching rule needed — build a minimal synthetic
	// rule so the handler can proceed with service selection pinned by the header.
	if probeService := ctx.GetHeader("X-Tingly-Probe-Service"); probeService != "" {
		if providerUUID, model, ok := strings.Cut(probeService, ":"); ok {
			svc := &loadbalance.Service{Provider: providerUUID, Model: model, Active: true}
			return &typ.Rule{
				UUID:         probeSyntheticRuleUUID,
				Scenario:     scenario,
				RequestModel: model,
				Services:     []*loadbalance.Service{svc},
				Active:       true,
			}, nil
		}
	}

	cfg := s.config
	if cfg != nil {
		// Use the new MatchRuleByModelAndScenario which supports wildcard matching
		rule := cfg.MatchRuleByModelAndScenario(modelName, scenario)
		if rule != nil && rule.Active {
			return rule, nil
		}
		// Enterprise runtime context is already authorized by TBE.
		// If endpoint scenario has no matching rule, allow lookup by model across scenarios.
		if isEnterpriseContextPresent(ctx) {
			for _, anyRule := range cfg.GetRequestConfigs() {
				if anyRule.Active && anyRule.RequestModel == modelName {
					return &anyRule, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
}
