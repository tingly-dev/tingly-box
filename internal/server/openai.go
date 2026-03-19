package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
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
		if scenario != nil && rule.GetScenario() != *scenario {
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
	responseModel := string(proxyModel)

	cursorCompat := resolveCursorCompat(c, rule)
	applyCursorCompatFlag(&req.ChatCompletionNewParams, cursorCompat)

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, responseModel, isStreaming)

	apiStyle := provider.APIStyle
	// === Check if provider has built-in web_search ===
	hasBuiltInWebSearch := s.templateManager.ProviderHasBuiltInWebSearch(provider)

	// === Tool Interceptor: Check if enabled and should be used ===
	shouldIntercept, shouldStripTools, _ := s.resolveToolInterceptor(provider, hasBuiltInWebSearch)

	if !s.enforceToolParserSupport(c, provider, actualModel, &req.ChatCompletionNewParams) {
		return
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
		// Apply cursor_compat content normalization before converting to Anthropic format
		// This ensures rich content is flattened for all providers when cursor_compat is enabled
		if cursorCompat {
			ops.ApplyCursorCompatContentNormalization(&req.ChatCompletionNewParams)
		}
		anthropicReq := request.ConvertOpenAIToAnthropicRequest(&req.ChatCompletionNewParams, int64(maxAllowed))
		if isStreaming {
			wrapper := s.clientPool.GetAnthropicClient(provider, string(anthropicReq.Model))
			fc := NewForwardContext(c.Request.Context(), provider)
			streamResp, cancel, err := ForwardAnthropicV1Stream(fc, wrapper, anthropicReq)
			if cancel != nil {
				defer cancel()
			}
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

			// Get scenario config for DisableStreamUsage flag
			disableStreamUsage := cursorCompat
			if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
				disableStreamUsage = disableStreamUsage || scenarioConfig.Flags.DisableStreamUsage
			}

			inputTokens, outputTokens, err := stream.HandleAnthropicToOpenAIStreamResponse(c, &anthropicReq, streamResp, responseModel, disableStreamUsage)
			if err != nil {
				// Track usage with error status
				if inputTokens > 0 || outputTokens > 0 {
					tokenUsage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
					s.trackUsageWithTokenUsage(c, tokenUsage, err)
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
				tokenUsage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
				s.trackUsageWithTokenUsage(c, tokenUsage, nil)
			}
			return
		} else {
			wrapper := s.clientPool.GetAnthropicClient(provider, string(anthropicReq.Model))
			fc := NewForwardContext(nil, provider)
			anthropicResp, cancel, err := ForwardAnthropicV1(fc, wrapper, anthropicReq)
			if cancel != nil {
				defer cancel()
			}
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

			// Track usage from response
			inputTokens := int(anthropicResp.Usage.InputTokens)
			outputTokens := int(anthropicResp.Usage.OutputTokens)
			cacheTokens := int(anthropicResp.Usage.CacheReadInputTokens + anthropicResp.Usage.CacheCreationInputTokens)
			tokenUsage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
			s.trackUsageWithTokenUsage(c, tokenUsage, nil)

			// Use provider-aware conversion for provider-specific handling
			openaiResp := ConvertAnthropicToOpenAIResponseWithProvider(anthropicResp, responseModel, provider, actualModel)
			if ShouldRoundtripResponse(c, "anthropic") {
				roundtripped, err := RoundtripOpenAIMapViaAnthropic(openaiResp, responseModel, provider, actualModel)
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
			if cursorCompat {
				delete(openaiResp, "usage")
			}
			c.JSON(http.StatusOK, openaiResp)
			return
		}
	case protocol.APIStyleOpenAI:
		// Check if model prefers responses endpoint (for models like Codex)
		if selectedService.PreferCompletions() {
			// Convert chat request to responses request
			s.handleResponsesForChatRequest(c, provider, &req, responseModel, actualModel, isStreaming)
			return
		}

		// Use Transform Chain for request transformation (Consistency + Vendor transforms)
		// Note: Base transform is not needed since the request is already in OpenAI Chat format
		// Chain: Consistency Transform → Vendor Transform
		chain := transform.NewTransformChain([]transform.Transform{
			//transform.NewConsistencyTransform(transform.TargetAPIStyleOpenAIChat),
			transform.NewVendorTransform(provider.APIBase),
		})

		// Create transform context
		var scenarioFlags *typ.ScenarioFlags
		if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
			scenarioFlags = &scenarioConfig.Flags
		}

		transformCtx := &transform.TransformContext{
			OriginalRequest: &req.ChatCompletionNewParams,
			Request:         &req.ChatCompletionNewParams,
			ProviderURL:     provider.APIBase,
			ScenarioFlags:   scenarioFlags,
			IsStreaming:     isStreaming,
		}

		// Execute transform chain
		finalCtx, err := chain.Execute(transformCtx)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: "Transform chain failed: " + err.Error(),
					Type:    "invalid_request_error",
				},
			})
			return
		}

		// Get final transformed request
		transformedReq := finalCtx.Request.(*openai.ChatCompletionNewParams)

		if isStreaming {
			// Get scenario config for DisableStreamUsage flag
			disableStreamUsage := cursorCompat
			if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
				disableStreamUsage = disableStreamUsage || scenarioConfig.Flags.DisableStreamUsage
			}

			s.handleOpenAIChatStreamingRequest(c, provider, transformedReq, responseModel, shouldIntercept, shouldStripTools, disableStreamUsage)
		} else {
			s.handleNonStreamingRequest(c, provider, transformedReq, responseModel, shouldIntercept, shouldStripTools, cursorCompat)
		}
	}
}
