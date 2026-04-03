package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// OpenAIListModels handles the /v1/models endpoint (OpenAI compatible)
func (s *Server) OpenAIListModels(c *gin.Context) {
	s.openAIListModelsWithScenario(c, nil)
}

// OpenAIListModelsForScenario handles scenario-scoped model listing for OpenAI format
func (s *Server) OpenAIListModelsForScenario(c *gin.Context, scenario typ.RuleScenario) {
	s.openAIListModelsWithScenario(c, &scenario)
}

func (s *Server) openAIListModelsWithScenario(c *gin.Context, scenario *typ.RuleScenario) {
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

	var models []OpenAIModel
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		if scenario != nil && !shouldIncludeRuleInModelList(*scenario, rule.GetScenario()) {
			continue
		}

		// Get timestamp from provider's LastUpdated field
		var created int64
		services := rule.GetServices()
		providerDesc := make([]string, 0, len(services))
		for i := range services {
			svc := services[i]
			// Skip nil services (defensive check after DB migration)
			if svc == nil {
				logrus.Debugf("Skipping nil service in rule %s during model list", rule.UUID)
				continue
			}
			if svc.Active {
				provider, err := cfg.GetProviderByUUID(svc.Provider)
				if err == nil {
					providerDesc = append(providerDesc, provider.Name)
					// Parse LastUpdated timestamp if available
					if provider.LastUpdated != "" {
						if t, err := time.Parse(time.RFC3339, provider.LastUpdated); err == nil {
							created = t.Unix()
						}
					}
				} else {
					providerDesc = append(providerDesc, svc.Provider)
				}
			}
		}

		// Build owned_by field
		ownedBy := "tingly-box"
		if len(providerDesc) > 0 {
			ownedBy += " via " + fmt.Sprintf("%v", providerDesc)
		}

		models = append(models, OpenAIModel{
			ID:      rule.RequestModel,
			Object:  "model",
			Created: created,
			OwnedBy: ownedBy,
		})
	}

	c.JSON(http.StatusOK, OpenAIModelsResponse{
		Object: "list",
		Data:   models,
	})
}

// HandleOpenAIChatCompletions handles OpenAI v1 chat completion requests
func (s *Server) HandleOpenAIChatCompletions(c *gin.Context) {

	scenario := c.Param("scenario")

	// Read raw body
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to read request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Parse OpenAI-style request
	var req protocol.OpenAIChatCompletionRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate
	responseModel := req.Model
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

	// Determine provider & model
	var (
		provider        *typ.Provider
		selectedService *loadbalance.Service
		rule            *typ.Rule
	)

	// Convert string to RuleScenario and validate
	scenarioType := typ.RuleScenario(scenario)
	if !isValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if this is the request model name first
	rule, err = s.determineRuleWithScenario(c, scenarioType, req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Select service using routing pipeline
	provider, selectedService, err = s.routingSelector.SelectService(c, scenarioType, rule, &req.ChatCompletionNewParams)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	actualModel := selectedService.Model
	req.Model = actualModel

	s.OpenAIChatCompletion(c, req, responseModel, provider, scenarioType, rule)
}

func (s *Server) OpenAIChatCompletion(c *gin.Context, req protocol.OpenAIChatCompletionRequest, responseModel string, provider *typ.Provider, scenarioType typ.RuleScenario, rule *typ.Rule) {
	isStreaming := req.Stream
	actualModel := req.Model
	maxAllowed := s.templateManager.GetMaxTokensForModelByProvider(provider, actualModel)
	cursorCompat := resolveCursorCompat(c, rule)
	applyCursorCompatFlag(&req.ChatCompletionNewParams, cursorCompat)

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, responseModel, isStreaming)

	apiStyle := provider.APIStyle
	// === Check if provider has built-in web_search ===
	hasBuiltInWebSearch := s.templateManager.ProviderHasBuiltInWebSearch(provider)

	// === Tool Interceptor: Check if enabled and should be used ===
	shouldIntercept, shouldStripTools, _ := s.resolveToolInterceptor(provider, hasBuiltInWebSearch)

	// === Cursor compat content normalization (before transform) ===
	if cursorCompat {
		ops.ApplyCursorCompatContentNormalization(&req.ChatCompletionNewParams)
	}
	transform.AlignToolMessagesForOpenAI(&req.ChatCompletionNewParams)

	// === Cap max_tokens at model's maximum ===
	if req.MaxTokens.Valid() && req.MaxTokens.Value > int64(maxAllowed) {
		req.MaxTokens.Value = int64(maxAllowed)
	}

	// === Determine target API type ===
	apiStyle = provider.APIStyle
	target := protocol.TypeOpenAIChat
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicBeta
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		if s.GetPreferredEndpointForModel(provider, actualModel) == "responses" {
			target = protocol.TypeOpenAIResponses
		}
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Unsupported API style: %s %s", provider.Name, apiStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// === Transform via pipeline ===
	reqCtx, err := s.transformOpenAIChat(c, req, target, provider, isStreaming, nil, scenarioType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Transform failed: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}
	reqCtx.Extra["cursor_compat"] = cursorCompat
	reqCtx.Extra["should_intercept"] = shouldIntercept
	reqCtx.Extra["should_strip_tools"] = shouldStripTools

	// === Dispatch via transform chain ===
	reqCtx.RequestModel = actualModel
	reqCtx.ResponseModel = responseModel
	s.dispatchChainResult(c, reqCtx, rule, provider, isStreaming, nil)
}
