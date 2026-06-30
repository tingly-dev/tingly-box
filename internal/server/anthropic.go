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
	// CapabilitySupport indicates whether a capability is supported.
	CapabilitySupport struct {
		Supported bool `json:"supported"`
	}

	// ContextManagementCapability describes context management support.
	ContextManagementCapability struct {
		Supported             bool              `json:"supported"`
		ClearThinking20251015 CapabilitySupport `json:"clear_thinking_20251015,omitempty"`
		ClearToolUses20250919 CapabilitySupport `json:"clear_tool_uses_20250919,omitempty"`
		Compact20260112       CapabilitySupport `json:"compact_20260112,omitempty"`
	}

	// EffortCapability describes reasoning_effort support and levels.
	EffortCapability struct {
		Supported bool              `json:"supported"`
		Low       CapabilitySupport `json:"low,omitempty"`
		Medium    CapabilitySupport `json:"medium,omitempty"`
		High      CapabilitySupport `json:"high,omitempty"`
		XHigh     CapabilitySupport `json:"xhigh,omitempty"`
		Max       CapabilitySupport `json:"max,omitempty"`
	}

	// ThinkingTypes describes supported thinking type configurations.
	ThinkingTypes struct {
		Adaptive CapabilitySupport `json:"adaptive,omitempty"`
		Enabled  CapabilitySupport `json:"enabled,omitempty"`
	}

	// ThinkingCapability describes thinking support.
	ThinkingCapability struct {
		Supported bool           `json:"supported"`
		Types     *ThinkingTypes `json:"types,omitempty"`
	}

	// ModelCapabilities maps to Anthropic's ModelCapabilities in /v1/models.
	ModelCapabilities struct {
		Batch             CapabilitySupport            `json:"batch"`
		Citations         CapabilitySupport            `json:"citations"`
		CodeExecution     CapabilitySupport            `json:"code_execution"`
		ContextManagement *ContextManagementCapability `json:"context_management,omitempty"`
		Effort            *EffortCapability            `json:"effort,omitempty"`
		ImageInput        CapabilitySupport            `json:"image_input"`
		PDFInput          CapabilitySupport            `json:"pdf_input"`
		StructuredOutputs CapabilitySupport            `json:"structured_outputs"`
		Thinking          *ThinkingCapability          `json:"thinking,omitempty"`
	}

	// AnthropicModel maps to Anthropic's native /v1/models response format.
	AnthropicModel struct {
		ID             string             `json:"id"`
		CreatedAt      string             `json:"created_at"`
		DisplayName    string             `json:"display_name"`
		Type           string             `json:"type"`
		Capabilities   *ModelCapabilities `json:"capabilities,omitempty"`
		MaxInputTokens int                `json:"max_input_tokens,omitempty"`
		MaxTokens      int                `json:"max_tokens,omitempty"`
		// Description is a tingly-box extension (not in Anthropic's wire format)
		// consumed by the frontend to show model description in the model picker.
		Description string `json:"description,omitempty"`
		// AuthType is a tingly-box extension (not in Anthropic's wire format)
		// consumed by the frontend to order model picker entries:
		// oauth -> api_key -> vmodel.
		AuthType string `json:"auth_type,omitempty"`
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
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
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
		c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
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
			logrus.WithError(err).Errorf("Anthropic beta decode error")
			c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Message decode error: %s", err.Error()),
					Type:    "invalid_request_error",
				},
			})
			return
		}
		requestModel = string(betaMessages.Model)
		reqParams = betaMessages.BetaMessageNewParams

	} else {
		if err := json.Unmarshal(bodyBytes, messages); err != nil {
			logrus.WithError(err).Errorf("Anthropic decode error")
			c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Message decode error: %s", err.Error()),
					Type:    "invalid_request_error",
				},
			})
			return
		}

		requestModel = string(messages.Model)
		reqParams = messages.MessageNewParams
	}

	// Check if this is the request requestModel name first
	rule, err = s.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}
	if rule == nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "no such rule",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	s.applyVisionProxy(c, scenarioType, rule, reqParams)

	// Select service using routing pipeline
	provider, selectedService, err = s.routingSelector.SelectService(c, scenarioType, rule, reqParams)
	if err != nil {
		logrus.WithError(err).Errorf("Select service error")
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
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
		c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
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
						maxTokens = modelInfo.MaxOutput
						break
					}
				}
			}
		}

		// Build capabilities from Anthropic API provider data
		var caps *ModelCapabilities
		if primaryProvider != nil && templateManager != nil {
			if tmpl, err := templateManager.GetTemplate(primaryProvider.Name); err == nil && tmpl != nil {
				// Only populate capabilities for known Anthropic providers
				if tmpl.VendorFamily == "anthropic" {
					yes := CapabilitySupport{Supported: true}
					no := CapabilitySupport{Supported: false}
					caps = &ModelCapabilities{
						Batch:             no,
						Citations:         yes,
						CodeExecution:     yes,
						ImageInput:        yes,
						PDFInput:          yes,
						StructuredOutputs: yes,
						ContextManagement: &ContextManagementCapability{
							Supported:             true,
							ClearThinking20251015: yes,
							ClearToolUses20250919: yes,
							Compact20260112:       yes,
						},
						Effort: &EffortCapability{
							Supported: true,
							Low:       yes,
							Medium:    yes,
							High:      yes,
							XHigh:     yes,
							Max:       yes,
						},
						Thinking: &ThinkingCapability{
							Supported: true,
							Types: &ThinkingTypes{
								Adaptive: yes,
								Enabled:  yes,
							},
						},
					}
				}
			}
		}

		logrus.Warnf("We do not set capabilities, even set: %v", caps)

		models = append(models, AnthropicModel{
			ID:          rule.RequestModel,
			CreatedAt:   "2024-01-01T00:00:00Z",
			DisplayName: displayName,
			Type:        "model",
			// Capabilities:   caps,
			MaxInputTokens: maxInputTokens,
			MaxTokens:      maxTokens,
			Description:    description,
			AuthType:       string(primaryAuthTypeForRule(cfg, rule)),
		})
	}

	sort.SliceStable(models, func(i, j int) bool {
		return authTypeSortWeight(typ.AuthType(models[i].AuthType)) <
			authTypeSortWeight(typ.AuthType(models[j].AuthType))
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
