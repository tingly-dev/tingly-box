package server

import (
	"context"
	"fmt"
	"iter"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/toolinterceptor"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// anthropicMessagesV1Beta implements beta messages API
func (s *Server) anthropicMessagesV1Beta(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, provider *typ.Provider, actualModel string, rule *typ.Rule) {
	// Extract scenario from URL path for recording
	scenario := c.Param("scenario")

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
			streamResp, cancel, err := s.forwardAnthropicStreamRequestV1Beta(c.Request.Context(), provider, req.BetaMessageNewParams, scenario)
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
			s.handleAnthropicStreamResponseV1Beta(c, req.BetaMessageNewParams, streamResp, proxyModel, actualModel, rule, provider, recorder)
		} else {
			// Handle non-streaming request
			anthropicResp, cancel, err := s.forwardAnthropicRequestV1Beta(provider, req.BetaMessageNewParams, scenario)
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
			streamResp, cancel, err := s.forwardGoogleStreamRequest(c.Request.Context(), provider, model, googleReq, cfg)
			if err != nil {
				SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			defer cancel()

			// Handle the streaming response
			err = stream.HandleGoogleToAnthropicBetaStreamResponse(c, streamResp, proxyModel)
			if err != nil {
				SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
			}

			// Track usage from stream (would be accumulated in handler)
			// For Google, usage is tracked in the stream handler

		} else {
			// Handle non-streaming request
			resp, err := s.forwardGoogleRequest(provider, model, googleReq, cfg)
			if err != nil {
				SendForwardingError(c, err)
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
			if resp.UsageMetadata != nil {
				inputTokens = int(resp.UsageMetadata.PromptTokenCount)
				outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
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
		useResponsesAPI := preferredEndpoint == "responses"

		if useResponsesAPI {
			// Use Responses API path (for Codex and other models that prefer it)
			// Convert Anthropic beta request to Responses API format
			responsesReq := request.ConvertAnthropicBetaToResponsesRequestWithProvider(&req.BetaMessageNewParams, provider, actualModel)

			// Set the rule and provider in context so middleware can use the same rule
			if rule != nil {
				c.Set("rule", rule)
			}

			// Set provider UUID in context
			c.Set("provider", provider.UUID)
			c.Set("model", actualModel)

			if isStreaming {
				s.handleAnthropicV1BetaViaResponsesAPIStreaming(c, req, proxyModel, actualModel, provider, rule, responsesReq)
			} else {
				s.handleAnthropicV1BetaViaResponsesAPINonStreaming(c, req, proxyModel, actualModel, provider, rule, responsesReq)
			}
		} else {
			// Use Chat Completions path (fallback)
			openaiReq := request.ConvertAnthropicBetaToOpenAIRequestWithProvider(&req.BetaMessageNewParams, true, provider, actualModel)

			// Set the rule and provider in context so middleware can use the same rule
			if rule != nil {
				c.Set("rule", rule)
			}

			// Set provider UUID in context
			c.Set("provider", provider.UUID)
			c.Set("model", actualModel)

			// Use OpenAI Chat Completions path
			if isStreaming {
				// Set up stream recorder
				streamRec := newStreamRecorder(recorder)
				if streamRec != nil {
					streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
				}

				// Create streaming request with request context for proper cancellation
				streamResp, _, err := s.forwardOpenAIStreamRequest(c.Request.Context(), provider, openaiReq)
				if err != nil {
					SendStreamingError(c, err)
					if streamRec != nil {
						streamRec.RecordError(err)
					}
					return
				}

				// Handle the streaming response
				err = stream.HandleOpenAIToAnthropicV1BetaStreamResponse(c, openaiReq, streamResp, proxyModel)
				if err != nil {
					SendInternalError(c, err.Error())
					if streamRec != nil {
						streamRec.RecordError(err)
					}
					return
				}

				// Finish recording and assemble response
				if streamRec != nil {
					streamRec.Finish(proxyModel, 0, 0) // Usage is tracked internally
					streamRec.RecordResponse(provider, actualModel)
				}

			} else {
				resp, err := s.forwardOpenAIRequest(provider, openaiReq)
				if err != nil {
					SendForwardingError(c, err)
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
				s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

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

// forwardAnthropicRequestV1Beta forwards request using Anthropic SDK with proper types (beta)
func (s *Server) forwardAnthropicRequestV1Beta(provider *typ.Provider, req anthropic.BetaMessageNewParams, scenario string) (*anthropic.BetaMessage, context.CancelFunc, error) {
	fc := NewForwardContext(nil, s.clientPool, provider, string(req.Model)).
		WithScenario(scenario)
	return ForwardAnthropicV1Beta(fc, req)
}

// forwardAnthropicStreamRequestV1Beta forwards streaming request using Anthropic SDK (beta)
func (s *Server) forwardAnthropicStreamRequestV1Beta(ctx context.Context, provider *typ.Provider, req anthropic.BetaMessageNewParams, scenario string) (*anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], context.CancelFunc, error) {
	fc := NewForwardContext(ctx, s.clientPool, provider, string(req.Model)).
		WithScenario(scenario)
	return ForwardAnthropicV1BetaStream(fc, req)
}

// handleAnthropicStreamResponseV1Beta processes the Anthropic beta streaming response and sends it to the client
func (s *Server) handleAnthropicStreamResponseV1Beta(c *gin.Context, req anthropic.BetaMessageNewParams, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], respModel, actualModel string, rule *typ.Rule, provider *typ.Provider, recorder *ScenarioRecorder) {
	hc := NewHandleContext(c, provider, actualModel, respModel).
		WithRecorder(recorder).
		WithServer(s)
	HandleAnthropicV1BetaStream(hc, req, streamResp)
}

// forwardGoogleRequest forwards request to Google API
func (s *Server) forwardGoogleRequest(provider *typ.Provider, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	fc := NewForwardContext(nil, s.clientPool, provider, model)
	return ForwardGoogle(fc, model, contents, config)
}

// forwardGoogleStreamRequest forwards streaming request to Google API
func (s *Server) forwardGoogleStreamRequest(ctx context.Context, provider *typ.Provider, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (iter.Seq2[*genai.GenerateContentResponse, error], context.CancelFunc, error) {
	fc := NewForwardContext(ctx, s.clientPool, provider, model)
	return ForwardGoogleStream(fc, model, contents, config)
}

// handleAnthropicV1BetaViaChatCompletions handles Anthropic v1beta request using OpenAI Chat Completions API
func (s *Server) handleAnthropicV1BetaViaChatCompletions(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule, isStreaming bool) {

}

// handleAnthropicV1BetaViaResponsesAPINonStreaming handles non-streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPINonStreaming(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, rule *typ.Rule, responsesReq responses.ResponseNewParams) {
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

	// Convert Responses API response back to Anthropic beta format
	anthropicResp := nonstream.ConvertResponsesToAnthropicBetaResponse(response, proxyModel)

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	c.JSON(http.StatusOK, anthropicResp)
}

// handleAnthropicV1BetaViaResponsesAPIStreaming handles streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPIStreaming(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, rule *typ.Rule, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
	}
	// Check if this is a ChatGPT backend API provider
	// These providers need special handling because they use custom HTTP implementation
	if provider.APIBase == protocol.ChatGPTBackendAPIBase {
		// Use the ChatGPT backend API streaming handler
		// This handler sends the stream directly to the client in OpenAI Responses API format
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
	// Use the dedicated stream handler to convert Responses API to Anthropic beta format
	err = stream.HandleResponsesToAnthropicV1BetaStreamResponse(c, streamResp, proxyModel)

	// Track usage from stream (would be accumulated in handler)
	// For now, we'll track minimal usage since the handler manages it
	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, 0, 0) // Usage is tracked internally
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}
