package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// anthropicMessagesV1 implements standard v1 messages API
func (s *Server) anthropicMessagesV1(c *gin.Context, req protocol.AnthropicMessagesRequest, proxyModel string, provider *typ.Provider, actualModel string, rule *typ.Rule) {

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

	// Check provider's API style to decide which path to take
	apiStyle := provider.APIStyle

	switch apiStyle {
	case protocol.APIStyleAnthropic:
		// Use direct Anthropic SDK call
		if isStreaming {
			// Handle streaming request with request context for proper cancellation
			streamResp, cancel, err := s.forwardAnthropicStreamRequestV1(c.Request.Context(), provider, req.MessageNewParams, scenario)
			if err != nil {
				s.trackUsageFromContext(c, 0, 0, "error", "stream_creation_failed")
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
			anthropicResp, err := s.forwardAnthropicRequestV1(provider, req.MessageNewParams, scenario)
			if err != nil {
				s.trackUsageFromContext(c, 0, 0, "error", "forward_failed")
				SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Track usage from response
			inputTokens := int(anthropicResp.Usage.InputTokens)
			outputTokens := int(anthropicResp.Usage.OutputTokens)
			s.trackUsageFromContext(c, inputTokens, outputTokens, "success", "")

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
			err = stream.HandleGoogleToAnthropicStreamResponse(c, streamResp, proxyModel)
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

			// Track usage from response
			inputTokens := 0
			outputTokens := 0
			if response.UsageMetadata != nil {
				inputTokens = int(response.UsageMetadata.PromptTokenCount)
				outputTokens = int(response.UsageMetadata.CandidatesTokenCount)
			}
			s.trackUsageFromContext(c, inputTokens, outputTokens, "success", "")

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
			err = stream.HandleOpenAIToAnthropicStreamResponse(c, openaiReq, streamResp, proxyModel)
			if err != nil {
				SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
			}

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

			// Track usage from response
			inputTokens := int(response.Usage.PromptTokens)
			outputTokens := int(response.Usage.CompletionTokens)
			s.trackUsageFromContext(c, inputTokens, outputTokens, "success", "")

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
func (s *Server) forwardAnthropicRequestV1(provider *typ.Provider, req anthropic.MessageNewParams, scenario string) (*anthropic.Message, error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	// Create context with scenario for recording
	ctx := context.WithValue(context.Background(), client.ScenarioContextKey, scenario)

	// Make the request using Anthropic SDK with timeout (provider.Timeout is in seconds)
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	message, err := wrapper.MessagesNew(ctx, req)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// forwardAnthropicStreamRequestV1 forwards streaming request using Anthropic SDK (v1)
func (s *Server) forwardAnthropicStreamRequestV1(ctx context.Context, provider *typ.Provider, req anthropic.MessageNewParams, scenario string) (*anthropicstream.Stream[anthropic.MessageStreamEventUnion], context.CancelFunc, error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	logrus.Debugln("Creating Anthropic streaming request")

	// Use request context with timeout for streaming
	// The context will be canceled if client disconnects
	// Also add scenario for recording
	timeout := time.Duration(provider.Timeout) * time.Second
	streamCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	streamCtx = context.WithValue(streamCtx, client.ScenarioContextKey, scenario)

	streamResp := wrapper.MessagesNewStreaming(streamCtx, req)

	return streamResp, cancel, nil
}

// handleAnthropicStreamResponseV1 processes the Anthropic streaming response and sends it to the client (v1)
func (s *Server) handleAnthropicStreamResponseV1(c *gin.Context, req anthropic.MessageNewParams, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion], respModel, actualModel string, rule *typ.Rule, provider *typ.Provider, recorder *ScenarioRecorder) {
	// Ensure stream is always closed
	defer func() {
		if streamResp != nil {
			if err := streamResp.Close(); err != nil {
				logrus.Errorf("Error closing Anthropic v1 stream: %v", err)
			}
		}
	}()

	// Accumulate usage from stream
	var inputTokens, outputTokens int
	var hasUsage bool

	// Create stream recorder for unified recording
	streamRec := newStreamRecorder(recorder)

	// Set SSE headers
	SetupSSEHeaders(c)

	// Check SSE support
	if !CheckSSESupport(c) {
		return
	}

	// Use gin.Stream for cleaner streaming handling
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping Anthropic v1 stream")
			return false
		default:
		}

		if !streamResp.Next() {
			return false
		}

		event := streamResp.Current()
		event.Message.Model = anthropic.Model(respModel)

		// Record event using streamRecorder
		streamRec.RecordV1Event(&event)

		// Accumulate usage from message_stop event
		if event.Usage.InputTokens > 0 {
			inputTokens = int(event.Usage.InputTokens)
			hasUsage = true
		}
		if event.Usage.OutputTokens > 0 {
			outputTokens = int(event.Usage.OutputTokens)
			hasUsage = true
		}

		// Convert the event to JSON and send as SSE
		c.SSEvent(event.Type, event)
		return true
	})

	// Finish recording and assemble response
	streamRec.Finish(respModel, inputTokens, outputTokens)

	// Check for stream errors
	if err := streamResp.Err(); err != nil {
		// Check if it was a client cancellation
		if IsContextCanceled(err) || errors.Is(err, context.Canceled) {
			logrus.Debug("Anthropic v1 stream canceled by client")
			// Track usage with canceled status
			if hasUsage {
				s.trackUsageFromContext(c, inputTokens, outputTokens, "canceled", "client_disconnected")
			}
			// Record error
			streamRec.RecordError(err)
			return
		}

		// Track usage with error status
		if hasUsage {
			s.trackUsageFromContext(c, inputTokens, outputTokens, "error", "stream_error")
		}
		MarshalAndSendErrorEvent(c, err.Error(), "stream_error", "stream_failed")
		// Record error
		streamRec.RecordError(err)
		return
	}

	// Track successful streaming completion
	if hasUsage {
		s.trackUsageFromContext(c, inputTokens, outputTokens, "success", "")
	}

	// Send completion event
	SendFinishEvent(c)

	// Record the response after stream completes
	streamRec.RecordResponse(provider, actualModel)
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
		s.trackUsageFromContext(c, 0, 0, "error", "forward_failed")
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
	s.trackUsageFromContext(c, inputTokens, outputTokens, "success", "")

	// Convert Responses API response back to Anthropic v1 format
	anthropicResp := nonstream.ConvertResponsesToAnthropicV1Response(response, proxyModel)

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
		s.trackUsageFromContext(c, 0, 0, "error", "stream_creation_failed")
		SendStreamingError(c, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}
	defer cancel()

	// Handle the streaming response
	// Use the dedicated stream handler to convert Responses API to Anthropic v1 format
	err = stream.HandleResponsesToAnthropicV1StreamResponse(c, streamResp, proxyModel)

	// Track usage from stream (would be accumulated in handler)
	if err != nil {
		s.trackUsageFromContext(c, 0, 0, "error", "stream_error")
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
