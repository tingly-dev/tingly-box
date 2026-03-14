package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/transformer"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/toolinterceptor"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// cleanSystemMessages removes billing header messages from system blocks
// This is used for Claude Code scenario to filter out injected billing headers
func cleanSystemMessages(blocks []anthropic.TextBlockParam) []anthropic.TextBlockParam {
	if len(blocks) == 0 {
		return blocks
	}
	result := make([]anthropic.TextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		// Skip billing header messages
		if strings.HasPrefix(strings.TrimSpace(block.Text), "x-anthropic-billing-header:") {
			continue
		}
		result = append(result, block)
	}
	return result
}

// cleanBetaSystemMessages removes billing header messages from beta system blocks
func cleanBetaSystemMessages(blocks []anthropic.BetaTextBlockParam) []anthropic.BetaTextBlockParam {
	if len(blocks) == 0 {
		return blocks
	}
	result := make([]anthropic.BetaTextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		// Skip billing header messages
		if strings.HasPrefix(strings.TrimSpace(block.Text), "x-anthropic-billing-header:") {
			continue
		}
		result = append(result, block)
	}
	return result
}

// anthropicMessagesV1 implements standard v1 messages API
func (s *Server) anthropicMessagesV1(c *gin.Context, req protocol.AnthropicMessagesRequest, proxyModel string, provider *typ.Provider, actualModel string, rule *typ.Rule) {

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

	// Get scenario config for DisableStreamUsage flag
	scenarioType := rule.GetScenario()
	disableStreamUsage := false
	cleanHeader := false
	if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		disableStreamUsage = scenarioConfig.Flags.DisableStreamUsage
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
				req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
			case typ.ThinkingModeAdaptive:
				// Adaptive mode: convert any existing thinking config to OfEnabled
				switch {
				case req.Thinking.OfEnabled != nil:
					req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
				case req.Thinking.OfAdaptive != nil:
					req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
				}
			case typ.ThinkingModeDefault:
				// Default mode: only handle OfEnabled, don't touch OfAdaptive
				if req.Thinking.OfEnabled != nil {
					req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
				}
			}
		}
	}

	// Clean system messages if clean_header flag is enabled (for Claude Code scenario)
	if cleanHeader {
		req.MessageNewParams.System = cleanSystemMessages(req.MessageNewParams.System)
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
		preparedReq, _ := s.toolInterceptor.PrepareAnthropicRequest(provider, &req.MessageNewParams)
		req.MessageNewParams = *preparedReq
	} else if shouldStripTools {
		req.MessageNewParams.Tools = toolinterceptor.StripSearchFetchToolsAnthropic(req.MessageNewParams.Tools)
	}

	// Check provider's API style to decide which path to take
	apiStyle := provider.APIStyle

	switch apiStyle {
	case protocol.APIStyleAnthropic:
		// Use direct Anthropic SDK call
		if isStreaming {
			// Handle streaming request with request context for proper cancellation
			wrapper := s.clientPool.GetAnthropicClient(provider, string(req.MessageNewParams.Model))
			fc := NewForwardContext(c.Request.Context(), provider)
			streamResp, cancel, err := ForwardAnthropicV1Stream(fc, wrapper, req.MessageNewParams)
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
			s.handleAnthropicStreamResponseV1(c, req.MessageNewParams, streamResp, proxyModel, actualModel, provider, recorder)
		} else {
			// Handle non-streaming request
			wrapper := s.clientPool.GetAnthropicClient(provider, string(req.MessageNewParams.Model))
			fc := NewForwardContext(nil, provider)
			anthropicResp, cancel, err := ForwardAnthropicV1(fc, wrapper, req.MessageNewParams)
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
			s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

			// FIXME: now we use req model as resp model
			anthropicResp.Model = anthropic.Model(proxyModel)

			if ShouldRoundtripResponse(c, "openai") {
				roundtripped, err := RoundtripAnthropicResponseViaOpenAI(anthropicResp, proxyModel, provider, actualModel)
				if err != nil {
					stream.SendInternalError(c, "Failed to roundtrip response: "+err.Error())
					return
				}
				anthropicResp = roundtripped
			}

			// Record response if scenario recording is enabled
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
		return

	case protocol.APIStyleGoogle:
		// Convert Anthropic request to Google format
		model, googleReq, cfg := request.ConvertAnthropicToGoogleRequest(&req.MessageNewParams, 0)

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
			usage, err := stream.HandleGoogleToAnthropicStreamResponse(c, streamResp, proxyModel)
			if err != nil {
				s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, err)
				stream.SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Track usage from stream handler
			s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, nil)

		} else {
			// Handle non-streaming request
			wrapper := s.clientPool.GetGoogleClient(provider, model)
			fc := NewForwardContext(nil, provider)
			response, _, err := ForwardGoogle(fc, wrapper, model, googleReq, cfg)
			if err != nil {
				stream.SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Convert Google response to Anthropic format
			anthropicResp := nonstream.ConvertGoogleToAnthropicResponse(response, proxyModel)
			if ShouldRoundtripResponse(c, "openai") {
				roundtripped, err := RoundtripAnthropicResponseViaOpenAI(&anthropicResp, proxyModel, provider, actualModel)
				if err != nil {
					stream.SendInternalError(c, "Failed to roundtrip response: "+err.Error())
					return
				}
				anthropicResp = *roundtripped
			}

			// Track usage from response
			inputTokens := 0
			outputTokens := 0
			if response.UsageMetadata != nil {
				inputTokens = int(response.UsageMetadata.PromptTokenCount)
				outputTokens = int(response.UsageMetadata.CandidatesTokenCount)
			}
			s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

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
		logrus.Debugf("[AnthropicV1] Probe cache preferred endpoint for model=%s: %s", actualModel, preferredEndpoint)
		useResponsesAPI := preferredEndpoint == "responses"

		if useResponsesAPI {
			// Use Responses API path with direct v1 conversion (no beta intermediate)
			// Convert Anthropic v1 request to Responses API format directly
			responsesReq := request.ConvertAnthropicV1ToResponsesRequest(&req.MessageNewParams)

			// Set the rule and provider in context so middleware can use the same rule
			if rule != nil {
				c.Set("rule", rule)
			}

			// Set provider UUID in context
			c.Set("provider", provider.UUID)
			c.Set("model", actualModel)

			// Set context flag to indicate original request was v1 format
			// The ChatGPT backend streaming handler will use this to send responses in v1 format
			c.Set("original_request_format", "v1")

			logrus.Debugf("[AnthropicV1] Using direct v1->Responses API conversion for model=%s", actualModel)

			if isStreaming {
				s.handleAnthropicV1ViaResponsesAPIStreaming(c, req, proxyModel, actualModel, provider, responsesReq)
			} else {
				s.handleAnthropicV1ViaResponsesAPINonStreaming(c, req, proxyModel, actualModel, provider, responsesReq)
			}
			return
		}

		// Convert Anthropic request to OpenAI format for streaming
		openaiReq, config := request.ConvertAnthropicToOpenAIRequest(&req.MessageNewParams, true, isStreaming, disableStreamUsage)

		// Apply provider-specific transforms
		transformedReq := transformer.ApplyProviderTransforms(openaiReq, provider, actualModel, config)

		// Clean up temporary fields (e.g., x_thinking)
		request.CleanupOpenaiFields(transformedReq)

		logrus.Debugf("[AnthropicV1] Using Chat Completions API for model=%s", actualModel)
		// Use OpenAI conversion path (default behavior)
		if isStreaming {

			// Create streaming request with request context for proper cancellation
			wrapper := s.clientPool.GetOpenAIClient(provider, transformedReq.Model)
			fc := NewForwardContext(c.Request.Context(), provider)
			streamResp, cancel, err := ForwardOpenAIChatStream(fc, wrapper, transformedReq)
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
			usage, err := stream.HandleOpenAIToAnthropicStreamResponse(c, transformedReq, streamResp, proxyModel)
			if err != nil {
				s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, err)
				stream.SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Track usage from stream handler
			s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, nil)

		} else {
			// Handle non-streaming request
			wrapper := s.clientPool.GetOpenAIClient(provider, transformedReq.Model)
			fc := NewForwardContext(nil, provider)
			response, _, err := ForwardOpenAIChat(fc, wrapper, transformedReq)
			if err != nil {
				stream.SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			// Convert OpenAI response back to Anthropic format
			anthropicResp := nonstream.ConvertOpenAIToAnthropicResponse(response, proxyModel)
			if ShouldRoundtripResponse(c, "openai") {
				roundtripped, err := RoundtripAnthropicResponseViaOpenAI(&anthropicResp, proxyModel, provider, actualModel)
				if err != nil {
					stream.SendInternalError(c, "Failed to roundtrip response: "+err.Error())
					return
				}
				anthropicResp = *roundtripped
			}

			// Track usage from response
			inputTokens := int(response.Usage.PromptTokens)
			outputTokens := int(response.Usage.CompletionTokens)
			s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

			// Record response if scenario recording is enabled
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
		if recorder != nil {
			recorder.RecordError(fmt.Errorf("invalid api style: %s", apiStyle))
		}
	}
}

// handleAnthropicStreamResponseV1 processes the Anthropic streaming response and sends it to the client (v1)
func (s *Server) handleAnthropicStreamResponseV1(c *gin.Context, req anthropic.MessageNewParams, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion], respModel, actualModel string, provider *typ.Provider, recorder *ScenarioRecorder) {
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

	usageStat, err := stream.HandleAnthropicV1Stream(hc, req, streamResp)
	s.trackUsageFromContext(c, usageStat.InputTokens, usageStat.OutputTokens, err)
}

// handleAnthropicV1ViaResponsesAPINonStreaming handles non-streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and converts back to v1
func (s *Server) handleAnthropicV1ViaResponsesAPINonStreaming(c *gin.Context, req protocol.AnthropicMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder if exists
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}

	var response *responses.Response
	var err error

	// Check if this is a ChatGPT backend API provider
	if provider.APIBase == protocol.ChatGPTBackendAPIBase {
		// Use the ChatGPT backend API handler
		response, err = s.forwardChatGPTBackendRequest(provider, responsesReq)
	} else {
		// Use standard OpenAI Responses API
		wrapper := s.clientPool.GetOpenAIClient(provider, string(responsesReq.Model))
		fc := NewForwardContext(nil, provider)
		response, _, err = ForwardOpenAIResponses(fc, wrapper, responsesReq)
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

	// Track usage
	s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

	// Convert Responses API response back to Anthropic v1 format
	anthropicResp := nonstream.ConvertResponsesToAnthropicV1Response(response, proxyModel)
	if ShouldRoundtripResponse(c, "openai") {
		roundtripped, err := RoundtripAnthropicResponseViaOpenAI(&anthropicResp, proxyModel, provider, actualModel)
		if err != nil {
			stream.SendInternalError(c, "Failed to roundtrip response: "+err.Error())
			return
		}
		anthropicResp = *roundtripped
	}

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	c.JSON(http.StatusOK, anthropicResp)
}

// handleAnthropicV1ViaResponsesAPIStreaming handles streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and streams back in v1 format
func (s *Server) handleAnthropicV1ViaResponsesAPIStreaming(c *gin.Context, req protocol.AnthropicMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}
	streamRec := newStreamRecorder(recorder)

	// Check if this is a ChatGPT backend API provider
	// These providers need special handling because they use custom HTTP implementation
	if provider.APIBase == protocol.ChatGPTBackendAPIBase {
		// Use the ChatGPT backend API streaming handler
		// This handler currently sends the stream in beta format, so we need to adapt it
		s.handleChatGPTBackendStreamingRequest(c, provider, responsesReq, proxyModel, actualModel)
		return
	}

	// For standard OpenAI providers, use the OpenAI SDK
	wrapper := s.clientPool.GetOpenAIClient(provider, string(responsesReq.Model))
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
	// Use the dedicated stream handler to convert Responses API to Anthropic v1 format
	usage, err := stream.HandleResponsesToAnthropicV1Stream(c, streamResp, proxyModel)

	// Track usage from stream handler
	if err != nil {
		s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage.InputTokens, usage.OutputTokens)
		streamRec.RecordResponse(provider, actualModel)
	}
}
