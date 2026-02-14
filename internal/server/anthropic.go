package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type (
	// AnthropicModel Model types - based on Anthropic's official models API format
	AnthropicModel struct {
		ID          string `json:"id"`
		CreatedAt   string `json:"created_at"`
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
	}
	AnthropicModelsResponse struct {
		Data    []AnthropicModel `json:"data"`
		FirstID string           `json:"first_id"`
		HasMore bool             `json:"has_more"`
		LastID  string           `json:"last_id"`
	}
)

// AnthropicMessages handles Anthropic v1 messages API requests
// This is the entry point that delegates to the appropriate implementation (v1 or beta)
func (s *Server) AnthropicMessages(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	// Check if beta parameter is set to true
	beta := c.Query("beta") == "true"
	logrus.Debugf("scenario: %s beta: %v", scenario, beta)

	// Validate scenario
	if !isValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Start scenario-level recording (client -> tingly-box traffic) only if enabled
	var recorder *ScenarioRecorder
	if s.ApplyRecording(scenarioType) {
		recorder = s.RecordScenarioRequest(c, scenario)
		if recorder != nil {
			// Store recorder in context for use in handlers
			c.Set("scenario_recorder", recorder)
			// Note: RecordResponse will be called by handler after stream completes
		}
	}

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		// Record error if recording is enabled
		if recorder != nil {
			recorder.RecordError(err)
		}
	} else {
		// Store the body back for parsing
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	// Determine provider & model
	var (
		provider        *typ.Provider
		selectedService *loadbalance.Service
		rule            *typ.Rule
	)
	var model string
	var reqParams interface{} // For smart routing context extraction

	var betaMessages protocol.AnthropicBetaMessagesRequest
	var messages protocol.AnthropicMessagesRequest
	if beta {
		if err := json.Unmarshal(bodyBytes, &betaMessages); err != nil {
			// Record error if recording is enabled
			if recorder != nil {
				recorder.RecordError(err)
			}
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: "Message error",
					Type:    "invalid_request_error",
				},
			})
			return
		}
		model = string(betaMessages.Model)
		reqParams = &betaMessages.BetaMessageNewParams

	} else {
		if err := json.Unmarshal(bodyBytes, &messages); err != nil {
			// Record error if recording is enabled
			if recorder != nil {
				recorder.RecordError(err)
			}
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: "Message error",
					Type:    "invalid_request_error",
				},
			})
			return
		}

		model = string(messages.Model)
		reqParams = &messages.MessageNewParams
	}

	// Check if this is the request model name first
	rule, err = s.determineRuleWithScenario(scenarioType, model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}
	provider, selectedService, err = s.DetermineProviderAndModelWithScenario(scenarioType, rule, reqParams)
	if err != nil {
		// Record error if recording is enabled
		if recorder != nil {
			recorder.RecordError(nil)
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Set the rule and provider in context
	if rule != nil {
		c.Set("rule", rule)
	}

	// Delegate to the appropriate implementation based on beta parameter
	if beta {

		// Apply compact transformation only if the compact feature is enabled for this scenario
		if s.ApplySmartCompact(scenarioType) {
			tf := smart_compact.NewCompactTransformer(2)
			tf.HandleV1Beta(&betaMessages.BetaMessageNewParams)
			logrus.Infoln("smart compact triggered")
		}
		s.anthropicMessagesV1Beta(c, betaMessages, model, provider, selectedService.Model, rule)

	} else {

		// Apply compact transformation only if the compact feature is enabled for this scenario
		if s.ApplySmartCompact(scenarioType) {
			tf := smart_compact.NewCompactTransformer(2)
			tf.HandleV1(&messages.MessageNewParams)
			logrus.Infoln("smart compact triggered")
		}
		s.anthropicMessagesV1(c, messages, model, provider, selectedService.Model, rule)
	}
}

// AnthropicListModels handles Anthropic v1 models endpoint
func (s *Server) AnthropicListModels(c *gin.Context) {
	s.anthropicListModelsWithScenario(c, nil)
}

// AnthropicListModelsForScenario handles scenario-scoped model listing for Anthropic format
func (s *Server) AnthropicListModelsForScenario(c *gin.Context, scenario typ.RuleScenario) {
	s.anthropicListModelsWithScenario(c, &scenario)
}

func (s *Server) anthropicListModelsWithScenario(c *gin.Context, scenario *typ.RuleScenario) {
	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Config not available",
				Type:    "internal_error",
			},
		})
		return
	}

	rules := cfg.GetRequestConfigs()

	var models []AnthropicModel
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		if scenario != nil && rule.GetScenario() != *scenario {
			continue
		}

		// Build display name with provider info
		displayName := rule.RequestModel
		services := rule.GetServices()
		if len(services) > 0 {
			providerNames := make([]string, 0, len(services))
			for i := range services {
				svc := services[i]
				if svc.Active {
					provider, err := cfg.GetProviderByUUID(svc.Provider)
					if err == nil {
						providerNames = append(providerNames, provider.Name)
					}
				}
			}
			if len(providerNames) > 0 {
				displayName += fmt.Sprintf(" (via %v)", providerNames)
			}
		}

		models = append(models, AnthropicModel{
			ID:          rule.RequestModel,
			CreatedAt:   "2024-01-01T00:00:00Z",
			DisplayName: displayName,
			Type:        "model",
		})
	}

	firstID := ""
	lastID := ""
	if len(models) > 0 {
		firstID = models[0].ID
		lastID = models[len(models)-1].ID
	}

	c.JSON(http.StatusOK, AnthropicModelsResponse{
		Data:    models,
		FirstID: firstID,
		HasMore: false,
		LastID:  lastID,
	})
}
