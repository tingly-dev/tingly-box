package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type (
	// AnthropicModel maps to Anthropic's native /v1/models response format.
	AnthropicModel struct {
		ID          string `json:"id"`
		CreatedAt   string `json:"created_at"`
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
		// Anthropic native fields.
		MaxInputTokens int `json:"max_input_tokens,omitempty"`
		MaxTokens      int `json:"max_tokens,omitempty"`
		// Detail contains tingly-box extensions shared with other model list formats.
		Detail *ModelDetail `json:"detail,omitempty"`
	}
	AnthropicModelsResponse struct {
		Data    []AnthropicModel `json:"data"`
		FirstID string           `json:"first_id"`
		HasMore bool             `json:"has_more"`
		LastID  string           `json:"last_id"`
	}
)

// HandleAnthropicMessages handles Anthropic v1 messages API requests
// This is the entry point that delegates to the appropriate implementation (v1 or beta)
func (s *Server) HandleAnthropicMessages(c *gin.Context) {
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

	//if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportAnthropic) {
	//	c.JSON(http.StatusBadRequest, ErrorResponse{
	//		Error: ErrorDetail{
	//			Message: fmt.Sprintf("scenario %s does not support Anthropic messages", scenario),
	//			Type:    "invalid_request_error",
	//		},
	//	})
	//	return
	//}

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
			},
		})
		return
	}

	// Determine provider & requestModel
	var (
		provider        *typ.Provider
		selectedService *loadbalance.Service
		rule            *typ.Rule
	)
	var requestModel string
	var reqParams interface{} // For smart routing context extraction

	var betaMessages = &protocol.AnthropicBetaMessagesRequest{}
	var messages = &protocol.AnthropicMessagesRequest{}
	if beta {
		if err := json.Unmarshal(bodyBytes, betaMessages); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Message decode error: %s", err.Error()),
					Type:    "invalid_request_error",
				},
			})
			logrus.WithError(err).Errorf("Anthropic beta decode error")
			c.Abort()
			return
		}
		requestModel = string(betaMessages.Model)
		reqParams = betaMessages.BetaMessageNewParams

	} else {
		if err := json.Unmarshal(bodyBytes, messages); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Message decode error: %s", err.Error()),
					Type:    "invalid_request_error",
				},
			})
			logrus.WithError(err).Errorf("Anthropic decode error")
			c.Abort()
			return
		}

		requestModel = string(messages.Model)
		reqParams = messages.MessageNewParams
	}

	// Check if this is the request requestModel name first
	rule, err = s.determineRuleWithScenario(c, scenarioType, requestModel)
	if rule == nil || err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	s.applyVisionProxy(c, scenarioType, rule, reqParams)

	// Select service using routing pipeline
	provider, selectedService, err = s.routingSelector.SelectService(c, scenarioType, rule, reqParams)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		logrus.WithError(err).Errorf("Select service error")
		return
	}

	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	actualModel := selectedService.Model

	// Delegate to the appropriate implementation based on beta parameter
	if beta {
		s.AnthropicMessagesV1Beta(c, betaMessages, actualModel, requestModel, rule, provider)

	} else {
		s.AnthropicMessagesV1(c, messages, actualModel, requestModel, rule, provider)
	}
}

// HandleAnthropicListModels handles Anthropic v1 models endpoint
func (s *Server) HandleAnthropicListModels(c *gin.Context) {
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
	templateManager := cfg.GetTemplateManager()

	var models []AnthropicModel
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		if scenario != nil && !shouldIncludeRuleInModelList(*scenario, rule.GetScenario()) {
			continue
		}

		// Build display name with provider info
		displayName := rule.RequestModel
		services := rule.GetServices()

		// Track provider for template lookup
		var primaryProvider *typ.Provider

		if len(services) > 0 {
			providerNames := make([]string, 0, len(services))
			for i := range services {
				svc := services[i]
				if svc.Active {
					provider, err := cfg.GetProviderByUUID(svc.Provider)
					if err == nil {
						if primaryProvider == nil {
							primaryProvider = provider
						}
						providerNames = append(providerNames, provider.Name)
					}
				}
			}
			if len(providerNames) > 0 {
				displayName += fmt.Sprintf(" (via %v)", providerNames)
			}
		}

		// Get model description from template if available
		var description string
		var maxInputTokens int
		var maxTokens int
		if templateManager != nil && primaryProvider != nil {
			if tmpl, err := templateManager.GetTemplate(primaryProvider.Name); err == nil && tmpl != nil {
				for _, modelInfo := range tmpl.Models {
					if modelInfo.ID == rule.RequestModel {
						description = modelInfo.Description
						maxInputTokens = modelInfo.Context
						maxTokens = modelInfo.MaxTokens
						break
					}
				}
			}
		}

		models = append(models, AnthropicModel{
			ID:             rule.RequestModel,
			CreatedAt:      "2024-01-01T00:00:00Z",
			DisplayName:    displayName,
			Type:           "model",
			MaxInputTokens: maxInputTokens,
			MaxTokens:      maxTokens,
			Detail: &ModelDetail{
				Description:         description,
				Context:             maxInputTokens,
				MaxTokens:           maxTokens,
				MaxCompletionTokens: maxTokens,
				InputModalities:     []string{"text"},
				OutputModalities:    []string{"text"},
				AuthType:            string(primaryAuthTypeForRule(cfg, rule)),
			},
		})
	}

	sort.SliceStable(models, func(i, j int) bool {
		return authTypeSortWeight(modelDetailAuthType(models[i].Detail)) <
			authTypeSortWeight(modelDetailAuthType(models[j].Detail))
	})

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
