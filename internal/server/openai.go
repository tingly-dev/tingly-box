package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
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
		if scenario != nil && rule.GetScenario() != *scenario {
			continue
		}

		ownedBy := "tingly-box"
		services := rule.GetServices()
		if len(services) > 0 {
			providerDesc := make([]string, 0, len(services))
			for i := range services {
				svc := services[i]
				if svc.Active {
					provider, err := cfg.GetProviderByUUID(svc.Provider)
					if err == nil {
						providerDesc = append(providerDesc, provider.Name)
					} else {
						providerDesc = append(providerDesc, svc.Provider)
					}
				}
			}
			if len(providerDesc) > 0 {
				ownedBy += " via " + fmt.Sprintf("%v", providerDesc)
			}
		}

		models = append(models, OpenAIModel{
			ID:      rule.RequestModel,
			Object:  "model",
			Created: 0,
			OwnedBy: ownedBy,
		})
	}

	c.JSON(http.StatusOK, OpenAIModelsResponse{
		Object: "list",
		Data:   models,
	})
}

// OpenAIChatCompletions handles OpenAI v1 chat completion requests
func (s *Server) OpenAIChatCompletions(c *gin.Context) {

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

	isStreaming := req.Stream

	// Validate
	proxyModel := req.Model
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
	rule, err = s.determineRuleWithScenario(scenarioType, req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}
	provider, selectedService, err = s.DetermineProviderAndModelWithScenario(scenarioType, rule, &req.ChatCompletionNewParams)
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
	maxAllowed := s.templateManager.GetMaxTokensForModelByProvider(provider, actualModel)
	responseModel := proxyModel

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, responseModel, isStreaming)

	apiStyle := provider.APIStyle
	// === Check if provider has built-in web_search ===
	hasBuiltInWebSearch := s.templateManager.ProviderHasBuiltInWebSearch(provider)

	// === Tool Interceptor: Check if enabled and should be used ===
	var interceptorConfig *typ.ToolInterceptorConfig
	if s.toolInterceptor != nil {
		interceptorConfig = s.toolInterceptor.GetConfigForProvider(provider)
	}
	shouldIntercept := interceptorConfig != nil && (interceptorConfig.PreferLocalSearch || !hasBuiltInWebSearch)
	shouldStripTools := interceptorConfig == nil && !hasBuiltInWebSearch

	if interceptorConfig != nil && interceptorConfig.PreferLocalSearch {
		logrus.Infof("Tool interceptor active for provider %s (prefer_local_search enabled)", provider.Name)
	} else if shouldIntercept {
		logrus.Infof("Tool interceptor active for provider %s (no built-in web_search)", provider.Name)
	} else if shouldStripTools {
		logrus.Infof("Tool interceptor disabled and provider %s has no built-in web_search; stripping search/fetch tools", provider.Name)
	} else if hasBuiltInWebSearch {
		logrus.Infof("Provider %s has built-in web_search, using native implementation", provider.Name)
	}

	switch apiStyle {
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Unsupported API style: %s %s", provider.Name, apiStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	case protocol.APIStyleAnthropic:
		anthropicReq := request.ConvertOpenAIToAnthropicRequest(&req.ChatCompletionNewParams, int64(maxAllowed))

		// 🔥 REQUIRED: forward tool_choice
		if req.ToolChoice.OfAuto.Value != "" || req.ToolChoice.OfAllowedTools != nil || req.ToolChoice.OfFunctionToolChoice != nil || req.ToolChoice.OfCustomToolChoice != nil {
			anthropicReq.ToolChoice = request.ConvertOpenAIToAnthropicToolChoice(&req.ToolChoice)
		}

		if isStreaming {
			streamResp, cancel, err := s.forwardAnthropicStreamRequestV1(c.Request.Context(), provider, anthropicReq, scenario)
			if err != nil {
				// Track error with no usage
				s.trackUsageFromContext(c, 0, 0, err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "Failed to create streaming request: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}
			defer cancel()

			inputTokens, outputTokens, err := stream.HandleAnthropicToOpenAIStreamResponse(c, &anthropicReq, streamResp, responseModel)
			if err != nil {
				// Track usage with error status
				if inputTokens > 0 || outputTokens > 0 {
					s.trackUsageFromContext(c, inputTokens, outputTokens, err)
				}
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "Failed to create streaming request: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}

			// Track successful streaming completion
			if inputTokens > 0 || outputTokens > 0 {
				s.trackUsageFromContext(c, inputTokens, outputTokens, nil)
			}
			return
		} else {
			anthropicResp, cancel, err := s.forwardAnthropicRequestV1(provider, anthropicReq, scenario)
			if err != nil {
				// Track error with no usage
				s.trackUsageFromContext(c, 0, 0, err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "Failed to forward Anthropic request: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}
			defer cancel()

			// Track usage from response
			inputTokens := int(anthropicResp.Usage.InputTokens)
			outputTokens := int(anthropicResp.Usage.OutputTokens)
			s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

			// Use provider-aware conversion for provider-specific handling
			openaiResp := nonstream.ConvertAnthropicToOpenAIResponseWithProvider(anthropicResp, responseModel, provider, actualModel)
			if shouldRoundtripResponse(c, "anthropic") {
				roundtripped, err := roundtripOpenAIMapViaAnthropic(openaiResp, responseModel, provider, actualModel)
				if err != nil {
					c.JSON(http.StatusInternalServerError, ErrorResponse{
						Error: ErrorDetail{
							Message: "Failed to roundtrip response: " + err.Error(),
							Type:    "api_error",
						},
					})
					return
				}
				openaiResp = roundtripped
			}
			c.JSON(http.StatusOK, openaiResp)
			return
		}
	case protocol.APIStyleOpenAI:
		// Check if model prefers responses endpoint (for models like Codex)
		if selectedService.PreferCompletions() {
			// Convert chat request to responses request
			s.handleResponsesForChatRequest(c, provider, &req, responseModel, actualModel, rule, isStreaming)
			return
		}

		if isStreaming {
			s.handleStreamingRequest(c, provider, &req.ChatCompletionNewParams, responseModel, rule, shouldIntercept, shouldStripTools)
		} else {
			s.handleNonStreamingRequest(c, provider, &req.ChatCompletionNewParams, responseModel, rule, shouldIntercept, shouldStripTools)
		}
	}
}
