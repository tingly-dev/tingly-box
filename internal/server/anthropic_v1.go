package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/toolinterceptor"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

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

	// Ensure max_tokens is set (Anthropic API requires this)
	// and cap it at the model's maximum allowed value
	if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil {

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
			streamResp, cancel, err := s.forwardAnthropicStreamRequestV1(c.Request.Context(), provider, req.MessageNewParams)
			if err != nil {
				s.trackUsageFromContext(c, 0, 0, err)
				SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			defer cancel()
			// Handle the streaming response
			s.handleAnthropicStreamResponseV1(c, req.MessageNewParams, streamResp, proxyModel, actualModel, rule, provider, recorder)
		} else {
			// Handle non-streaming request
			anthropicResp, cancel, err := s.forwardAnthropicRequestV1(provider, req.MessageNewParams)
			if err != nil {
				s.trackUsageFromContext(c, 0, 0, err)
				SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			defer cancel()

			// Track usage from response
			inputTokens := int(anthropicResp.Usage.InputTokens)
			outputTokens := int(anthropicResp.Usage.OutputTokens)
			s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

			// FIXME: now we use req model as resp model
			anthropicResp.Model = anthropic.Model(proxyModel)

			if shouldRoundtripResponse(c, "openai") {
				roundtripped, err := roundtripAnthropicResponseViaOpenAI(anthropicResp, proxyModel, provider, actualModel)
				if err != nil {
					SendInternalError(c, "Failed to roundtrip response: "+err.Error())
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
			streamResp, _, err := s.forwardGoogleStreamRequest(c.Request.Context(), provider, model, googleReq, cfg)
			if err != nil {
				SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Handle the streaming response
			usage, err := stream.HandleGoogleToAnthropicStreamResponse(c, streamResp, proxyModel)
			if err != nil {
				s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, err)
				SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Track usage from stream handler
			s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, nil)

		} else {
			// Handle non-streaming request
			response, err := s.forwardGoogleRequest(provider, model, googleReq, cfg)
			if err != nil {
				SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Convert Google response to Anthropic format
			anthropicResp := nonstream.ConvertGoogleToAnthropicResponse(response, proxyModel)
			if shouldRoundtripResponse(c, "openai") {
				roundtripped, err := roundtripAnthropicResponseViaOpenAI(&anthropicResp, proxyModel, provider, actualModel)
				if err != nil {
					SendInternalError(c, "Failed to roundtrip response: "+err.Error())
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
			responsesReq := request.ConvertAnthropicV1ToResponsesRequestWithProvider(&req.MessageNewParams, provider, actualModel)

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
				s.handleAnthropicV1ViaResponsesAPIStreaming(c, req, proxyModel, actualModel, provider, rule, responsesReq)
			} else {
				s.handleAnthropicV1ViaResponsesAPINonStreaming(c, req, proxyModel, actualModel, provider, rule, responsesReq)
			}
			return
		}

		logrus.Debugf("[AnthropicV1] Using Chat Completions API for model=%s", actualModel)
		// Use OpenAI conversion path (default behavior)
		if isStreaming {
			// Convert Anthropic request to OpenAI format for streaming
			openaiReq := request.ConvertAnthropicToOpenAIRequestWithProvider(&req.MessageNewParams, true, provider, actualModel)

			// Create streaming request with request context for proper cancellation
			streamResp, _, err := s.forwardOpenAIStreamRequest(c.Request.Context(), provider, openaiReq)
			if err != nil {
				SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Handle the streaming response
			usage, err := stream.HandleOpenAIToAnthropicStreamResponse(c, openaiReq, streamResp, proxyModel)
			if err != nil {
				s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, err)
				SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Track usage from stream handler
			s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, nil)

		} else {
			// Handle non-streaming request
			// Convert Anthropic request to OpenAI format with provider transforms
			openaiReq := request.ConvertAnthropicToOpenAIRequestWithProvider(&req.MessageNewParams, true, provider, actualModel)
			response, err := s.forwardOpenAIRequest(provider, openaiReq)
			if err != nil {
				SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			// Convert OpenAI response back to Anthropic format
			anthropicResp := nonstream.ConvertOpenAIToAnthropicResponse(response, proxyModel)
			if shouldRoundtripResponse(c, "openai") {
				roundtripped, err := roundtripAnthropicResponseViaOpenAI(&anthropicResp, proxyModel, provider, actualModel)
				if err != nil {
					SendInternalError(c, "Failed to roundtrip response: "+err.Error())
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

// forwardAnthropicRequestV1 forwards request using Anthropic SDK with proper types (v1)
func (s *Server) forwardAnthropicRequestV1(provider *typ.Provider, req anthropic.MessageNewParams) (*anthropic.Message, context.CancelFunc, error) {
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))
	fc := NewForwardContext(nil, provider)
	return ForwardAnthropicV1(fc, wrapper, req)
}

// forwardAnthropicStreamRequestV1 forwards streaming request using Anthropic SDK (v1)
func (s *Server) forwardAnthropicStreamRequestV1(ctx context.Context, provider *typ.Provider, req anthropic.MessageNewParams) (*anthropicstream.Stream[anthropic.MessageStreamEventUnion], context.CancelFunc, error) {
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))
	fc := NewForwardContext(ctx, provider)
	return ForwardAnthropicV1Stream(fc, wrapper, req)
}

// handleAnthropicStreamResponseV1 processes the Anthropic streaming response and sends it to the client (v1)
func (s *Server) handleAnthropicStreamResponseV1(c *gin.Context, req anthropic.MessageNewParams, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion], respModel, actualModel string, rule *typ.Rule, provider *typ.Provider, recorder *ScenarioRecorder) {
	hc := NewHandleContext(c, provider, actualModel, respModel).
		WithRecorder(recorder)

	usageStat, err := HandleAnthropicV1Stream(hc, req, streamResp)
	s.trackUsageFromContext(c, usageStat.InputTokens, usageStat.OutputTokens, err)
}

// handleAnthropicV1ViaResponsesAPINonStreaming handles non-streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and converts back to v1
func (s *Server) handleAnthropicV1ViaResponsesAPINonStreaming(c *gin.Context, req protocol.AnthropicMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, rule *typ.Rule, responsesReq responses.ResponseNewParams) {
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
		response, err = s.forwardResponsesRequest(provider, responsesReq)
	}

	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		SendForwardingError(c, err)
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
	if shouldRoundtripResponse(c, "openai") {
		roundtripped, err := roundtripAnthropicResponseViaOpenAI(&anthropicResp, proxyModel, provider, actualModel)
		if err != nil {
			SendInternalError(c, "Failed to roundtrip response: "+err.Error())
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
func (s *Server) handleAnthropicV1ViaResponsesAPIStreaming(c *gin.Context, req protocol.AnthropicMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, rule *typ.Rule, responsesReq responses.ResponseNewParams) {
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
		s.handleChatGPTBackendStreamingRequest(c, provider, responsesReq, proxyModel, actualModel, rule)
		return
	}

	// For standard OpenAI providers, use the OpenAI SDK
	streamResp, cancel, err := s.forwardResponsesStreamRequest(c.Request.Context(), provider, responsesReq)
	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		SendStreamingError(c, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}
	defer cancel()

	// Handle the streaming response
	// Use the dedicated stream handler to convert Responses API to Anthropic v1 format
	usage, err := stream.HandleResponsesToAnthropicV1StreamResponse(c, streamResp, proxyModel)

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
