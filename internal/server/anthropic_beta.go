package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/toolinterceptor"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// anthropicMessagesV1Beta implements beta messages API
func (s *Server) anthropicMessagesV1Beta(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, provider *typ.Provider, actualModel string, rule *typ.Rule) {

	// Get scenario recorder if exists (set by AnthropicMessages)
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}

	// Check if streaming is requested
	isStreaming := req.Stream

	req.Model = anthropic.Model(actualModel)

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, proxyModel, isStreaming)

	// === Check if provider has built-in web_search ===
	hasBuiltInWebSearch := s.templateManager.ProviderHasBuiltInWebSearch(provider)

	// === Tool Interceptor: Check if enabled and should be used ===
	shouldIntercept, shouldStripTools, _ := s.resolveToolInterceptor(provider, hasBuiltInWebSearch)

	// Get scenario config for flags
	scenarioType := rule.GetScenario()
	cleanHeader := false
	if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		cleanHeader = scenarioConfig.Flags.CleanHeader

		// Apply thinking mode from scenario config
		// The thinking mode controls how extended thinking is enabled
		thinkingMode := scenarioConfig.Flags.ThinkingMode
		if thinkingMode != "" {
			// Map effort level to budget_tokens
			effort := scenarioConfig.Flags.ThinkingEffort
			if effort == typ.ThinkingEffortDefault {
				effort = typ.ThinkingEffortMedium // fallback to medium
			}
			budgetTokens, ok := typ.ThinkingBudgetMapping[effort]
			if !ok {
				budgetTokens = typ.ThinkingBudgetMapping[typ.ThinkingEffortMedium]
			}
			if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil {
				budgetTokens = *thinkBudget
			}

			switch typ.ThinkingMode(thinkingMode) {
			case typ.ThinkingModeForce:
				// Force mode: always enable thinking regardless of client config
				req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budgetTokens)
			case typ.ThinkingModeAdaptive:
				// Adaptive mode: convert any existing thinking config to OfEnabled
				switch {
				case req.Thinking.OfEnabled != nil:
					req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budgetTokens)
				case req.Thinking.OfAdaptive != nil:
					req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budgetTokens)
				}
			case typ.ThinkingModeDefault:
				// Default mode: only handle OfEnabled, don't touch OfAdaptive
				if req.Thinking.OfEnabled != nil {
					req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budgetTokens)
				}
			default:
				req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
			}
		}
	}

	// Clean system messages if clean_header flag is enabled (for Claude Code scenario)
	if cleanHeader {
		req.BetaMessageNewParams.System = cleanBetaSystemMessages(req.BetaMessageNewParams.System)
	}

	// Ensure max_tokens is set (Anthropic API requires this)
	// and cap it at the model's maximum allowed value
	if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil {
		// for thinking, max tokens should be larger than thinking budget
	} else {
		if req.MaxTokens == 0 {
			req.MaxTokens = int64(s.config.GetDefaultMaxTokens())
		}
		// Cap max_tokens at the model's maximum to prevent API errors
		maxAllowed := s.templateManager.GetMaxTokensForModel(provider.Name, actualModel)
		if req.MaxTokens > int64(maxAllowed) {
			req.MaxTokens = int64(maxAllowed)
		}
	}

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", actualModel)

	// === PRE-REQUEST INTERCEPTION: Strip tools before sending to provider ===
	if shouldIntercept {
		preparedReq, _ := s.toolInterceptor.PrepareAnthropicBetaRequest(provider, &req.BetaMessageNewParams)
		req.BetaMessageNewParams = *preparedReq
	} else if shouldStripTools {
		req.BetaMessageNewParams.Tools = toolinterceptor.StripSearchFetchToolsAnthropicBeta(req.BetaMessageNewParams.Tools)
	}

	// Check provider's API style to decide which path to take
	apiStyle := provider.APIStyle

	switch apiStyle {
	case protocol.APIStyleAnthropic:
		// Use direct Anthropic SDK call
		if isStreaming {
			// Handle streaming request with request context for proper cancellation
			wrapper := s.clientPool.GetAnthropicClient(provider, string(req.BetaMessageNewParams.Model))
			fc := NewForwardContext(c.Request.Context(), provider)
			streamResp, cancel, err := ForwardAnthropicV1BetaStream(fc, wrapper, req.BetaMessageNewParams)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				s.trackUsageFromContext(c, 0, 0, err)
				stream.SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			// Handle the streaming response
			s.handleAnthropicStreamResponseV1Beta(c, req.BetaMessageNewParams, streamResp, proxyModel, actualModel, provider, recorder)
		} else {
			// Handle non-streaming request
			wrapper := s.clientPool.GetAnthropicClient(provider, string(req.BetaMessageNewParams.Model))
			fc := NewForwardContext(nil, provider)
			anthropicResp, cancel, err := ForwardAnthropicV1Beta(fc, wrapper, req.BetaMessageNewParams)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				s.trackUsageFromContext(c, 0, 0, err)
				stream.SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Track usage from response
			inputTokens := int(anthropicResp.Usage.InputTokens)
			outputTokens := int(anthropicResp.Usage.OutputTokens)
			cacheTokens := int(anthropicResp.Usage.CacheReadInputTokens)
			usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
			s.trackUsageWithTokenUsage(c, usage, nil)

			// FIXME: now we use req model as resp model
			anthropicResp.Model = anthropic.Model(proxyModel)

			// Record response if scenario recording is enabled
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
		return
	case protocol.APIStyleGoogle:

		// Convert Anthropic beta request to Google format
		model, googleReq, cfg := request.ConvertAnthropicBetaToGoogleRequest(&req.BetaMessageNewParams, 0)

		if isStreaming {
			// Create streaming request with request context for proper cancellation
			wrapper := s.clientPool.GetGoogleClient(provider, model)
			fc := NewForwardContext(c.Request.Context(), provider)
			streamResp, cancel, err := ForwardGoogleStream(fc, wrapper, model, googleReq, cfg)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				stream.SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Handle the streaming response
			usage, err := stream.HandleGoogleToAnthropicBetaStreamResponse(c, streamResp, proxyModel)
			if err != nil {
				s.trackUsageWithTokenUsage(c, usage, err)
				stream.SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Track usage from stream handler
			s.trackUsageWithTokenUsage(c, usage, nil)

		} else {
			// Handle non-streaming request
			wrapper := s.clientPool.GetGoogleClient(provider, model)
			fc := NewForwardContext(nil, provider)
			resp, _, err := ForwardGoogle(fc, wrapper, model, googleReq, cfg)
			if err != nil {
				stream.SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Convert Google response to Anthropic beta format
			anthropicResp := nonstream.ConvertGoogleToAnthropicBetaResponse(resp, proxyModel)

			// Track usage from response
			inputTokens := 0
			outputTokens := 0
			cacheTokens := 0
			if resp.UsageMetadata != nil {
				inputTokens = int(resp.UsageMetadata.PromptTokenCount)
				outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
				cacheTokens = int(resp.UsageMetadata.CachedContentTokenCount)
			}
			usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
			s.trackUsageWithTokenUsage(c, usage, nil)

			// Record response if scenario recording is enabled
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
	case protocol.APIStyleOpenAI:
		// Check if model prefers Responses API (for models like Codex)
		// This is used for ChatGPT backend API which only supports Responses API
		preferredEndpoint := s.GetPreferredEndpointForModel(provider, actualModel)
		useResponsesAPI := preferredEndpoint == "responses"

		if useResponsesAPI {
			// Use Responses API path with Transform Chain
			// Set the rule and provider in context so middleware can use the same rule
			if rule != nil {
				c.Set("rule", rule)
			}

			// Set provider UUID in context
			c.Set("provider", provider.UUID)
			c.Set("model", actualModel)

			// Use Transform Chain for request transformation
			// Chain: Base Transform → Consistency Transform → Vendor Transform
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(transform.TargetAPIStyleOpenAIResponses),
				//transform.NewConsistencyTransform(transform.TargetAPIStyleOpenAIResponses),
				transform.NewVendorTransform(provider.APIBase),
			})

			// Create transform context
			var scenarioFlags *typ.ScenarioFlags
			if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
				scenarioFlags = &scenarioConfig.Flags
			}

			transformCtx := &transform.TransformContext{
				OriginalRequest: &req.BetaMessageNewParams,
				Request:         &req.BetaMessageNewParams, // Original Anthropic beta request
				ProviderURL:     provider.APIBase,
				ScenarioFlags:   scenarioFlags,
				IsStreaming:     isStreaming,
			}

			// Execute transform chain
			finalCtx, err := chain.Execute(transformCtx)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Get final transformed request
			transformedReq := finalCtx.Request.(*responses.ResponseNewParams)

			if isStreaming {
				req.Stream = true
				s.handleAnthropicV1BetaViaResponsesAPIStreaming(c, req, proxyModel, actualModel, provider, *transformedReq)
			} else {
				if provider.APIBase == protocol.CodexAPIBase {
					req.Stream = true
					s.handleAnthropicV1BetaViaResponsesAPIAssembly(c, req, proxyModel, actualModel, provider, *transformedReq)
				} else {
					s.handleAnthropicV1BetaViaResponsesAPINonStreaming(c, req, proxyModel, actualModel, provider, *transformedReq)
				}
			}
		} else {
			// Set the rule and provider in context so middleware can use the same rule
			if rule != nil {
				c.Set("rule", rule)
			}

			// Set provider UUID in context
			c.Set("provider", provider.UUID)
			c.Set("model", actualModel)

			// Use Transform Chain for request transformation
			// Chain: Base Transform → Consistency Transform → Vendor Transform
			chain := transform.NewTransformChain([]transform.Transform{
				transform.NewBaseTransform(transform.TargetAPIStyleOpenAIChat),
				//transform.NewConsistencyTransform(transform.TargetAPIStyleOpenAIChat),
				transform.NewVendorTransform(provider.APIBase),
			})

			// Create transform context
			var scenarioFlags *typ.ScenarioFlags
			if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
				scenarioFlags = &scenarioConfig.Flags
			}

			transformCtx := &transform.TransformContext{
				OriginalRequest: &req.BetaMessageNewParams,
				Request:         &req.BetaMessageNewParams, // Original Anthropic beta request
				ProviderURL:     provider.APIBase,
				ScenarioFlags:   scenarioFlags,
				IsStreaming:     isStreaming,
			}

			// Execute transform chain
			finalCtx, err := chain.Execute(transformCtx)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Get final transformed request
			transformedReq := finalCtx.Request.(*openai.ChatCompletionNewParams)

			// Clean up temporary fields (e.g., x_thinking)
			request.CleanupOpenaiFields(transformedReq)

			// Use OpenAI Chat Completions path
			if isStreaming {
				// Set up stream recorder
				streamRec := newStreamRecorder(recorder)
				if streamRec != nil {
					streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
				}

				// Create streaming request with request context for proper cancellation
				wrapper := s.clientPool.GetOpenAIClient(provider, transformedReq.Model)
				fc := NewForwardContext(c.Request.Context(), provider)
				streamResp, cancel, err := ForwardOpenAIChatStream(fc, wrapper, transformedReq)
				if cancel != nil {
					defer cancel()
				}
				if err != nil {
					stream.SendStreamingError(c, err)
					if streamRec != nil {
						streamRec.RecordError(err)
					}
					return
				}

				// Handle the streaming response
				usage, err := stream.HandleOpenAIToAnthropicBetaStream(c, transformedReq, streamResp, proxyModel)
				if err != nil {
					s.trackUsageWithTokenUsage(c, usage, err)
					stream.SendInternalError(c, err.Error())
					if streamRec != nil {
						streamRec.RecordError(err)
					}
					return
				}

				// Track usage from stream handler
				s.trackUsageWithTokenUsage(c, usage, nil)

				// Finish recording and assemble response
				if streamRec != nil {
					streamRec.Finish(proxyModel, usage.InputTokens, usage.OutputTokens)
					streamRec.RecordResponse(provider, actualModel)
				}

			} else {
				wrapper := s.clientPool.GetOpenAIClient(provider, transformedReq.Model)
				fc := NewForwardContext(nil, provider)
				resp, _, err := ForwardOpenAIChat(fc, wrapper, transformedReq)
				if err != nil {
					stream.SendForwardingError(c, err)
					if recorder != nil {
						recorder.RecordError(err)
					}
					return
				}
				// Convert OpenAI response back to Anthropic beta format
				anthropicResp := nonstream.ConvertOpenAIToAnthropicBetaResponse(resp, proxyModel)

				// Track usage from response
				inputTokens := int(resp.Usage.PromptTokens)
				outputTokens := int(resp.Usage.CompletionTokens)
				cacheTokens := int(resp.Usage.PromptTokensDetails.CachedTokens)
				usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
				s.trackUsageWithTokenUsage(c, usage, nil)

				// Record response if scenario recording is enabled
				if recorder != nil {
					recorder.SetAssembledResponse(anthropicResp)
					recorder.RecordResponse(provider, actualModel)
				}
				c.JSON(http.StatusOK, anthropicResp)
			}
		}
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
		if recorder != nil {
			recorder.RecordError(fmt.Errorf("invalid api style: %s", apiStyle))
		}
	}
}

// handleAnthropicStreamResponseV1Beta processes the Anthropic beta streaming response and sends it to the client
func (s *Server) handleAnthropicStreamResponseV1Beta(c *gin.Context, req anthropic.BetaMessageNewParams, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], respModel, actualModel string, provider *typ.Provider, recorder *ScenarioRecorder) {
	hc := protocol.NewHandleContext(c, respModel)

	// Add recorder hooks if recorder is available
	if recorder != nil {
		onEvent, onComplete, onError := NewRecorderHooksWithModel(recorder, actualModel, provider)
		if onEvent != nil {
			hc.WithOnStreamEvent(onEvent)
		}
		if onComplete != nil {
			hc.WithOnStreamComplete(onComplete)
		}
		if onError != nil {
			hc.WithOnStreamError(onError)
		}
	}

	usageStat, err := stream.HandleAnthropicV1BetaStream(hc, req, streamResp)
	s.trackUsageWithTokenUsage(c, usageStat, err)
}

// handleAnthropicV1BetaViaResponsesAPINonStreaming handles non-streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPINonStreaming(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder if exists
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}

	var response *responses.Response
	var err error
	var cancel context.CancelFunc

	// Use standard OpenAI Responses API
	wrapper := s.clientPool.GetOpenAIClient(provider, responsesReq.Model)
	fc := NewForwardContext(nil, provider)

	response, cancel, err = ForwardOpenAIResponses(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}

	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		stream.SendForwardingError(c, err)
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	// Extract usage from response
	inputTokens := int(response.Usage.InputTokens)
	outputTokens := int(response.Usage.OutputTokens)
	cacheTokens := int(response.Usage.InputTokensDetails.CachedTokens)
	usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)

	// Track usage
	s.trackUsageWithTokenUsage(c, usage, nil)

	anthropicResp := nonstream.ConvertResponsesToAnthropicBetaResponse(response, proxyModel)
	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	c.JSON(http.StatusOK, anthropicResp)

}

// handleAnthropicV1BetaViaResponsesAPIStreaming handles streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPIStreaming(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
	}

	// For standard OpenAI providers, use the OpenAI SDK
	wrapper := s.clientPool.GetOpenAIClient(provider, responsesReq.Model)
	fc := NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		stream.SendStreamingError(c, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	// Handle the streaming response
	// Use the dedicated stream handler to convert Responses API to Anthropic beta format
	usage, err := stream.HandleResponsesToAnthropicBetaStream(c, streamResp, proxyModel)

	// Track usage from stream handler
	if err != nil {
		s.trackUsageWithTokenUsage(c, usage, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usage, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage.InputTokens, usage.OutputTokens)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// handleAnthropicV1BetaViaResponsesAPIStreaming handles streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPIAssembly(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
	}

	// For standard OpenAI providers, use the OpenAI SDK
	wrapper := s.clientPool.GetOpenAIClient(provider, responsesReq.Model)
	fc := NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		stream.SendStreamingError(c, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	// Handle the streaming response
	// Use the dedicated stream handler to convert Responses API to Anthropic beta format
	usage, err := stream.HandleResponsesToAnthropicBetaAssembly(c, streamResp, proxyModel)

	// Track usage from stream handler
	if err != nil {
		s.trackUsageWithTokenUsage(c, usage, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usage, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage.InputTokens, usage.OutputTokens)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}
