package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// anthropicMessagesV1 implements standard v1 messages API
func (s *Server) anthropicMessagesV1(c *gin.Context, req protocol.AnthropicMessagesRequest, proxyModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule) {
	actualModel := selectedService.Model

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

	// Set the rule and provider in context so middleware can use the same rule
	if rule != nil {
		c.Set("rule", rule)
	}

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
	c.Set("model", selectedService.Model)

	// Check provider's API style to decide which path to take
	apiStyle := provider.APIStyle

	switch apiStyle {
	case protocol.APIStyleAnthropic:
		// Use direct Anthropic SDK call
		if isStreaming {
			// Handle streaming request
			streamResp, err := s.forwardAnthropicStreamRequestV1(provider, req.MessageNewParams, scenario)
			if err != nil {
				s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "stream_creation_failed")
				SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			// Handle the streaming response
			s.handleAnthropicStreamResponseV1(c, req.MessageNewParams, streamResp, proxyModel, actualModel, rule, provider, recorder)
		} else {
			// Handle non-streaming request
			anthropicResp, err := s.forwardAnthropicRequestV1(provider, req.MessageNewParams, scenario)
			if err != nil {
				s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "forward_failed")
				SendForwardingError(c, err)
				return
			}

			// Track usage from response
			inputTokens := int(anthropicResp.Usage.InputTokens)
			outputTokens := int(anthropicResp.Usage.OutputTokens)
			s.trackUsage(c, rule, provider, actualModel, proxyModel, inputTokens, outputTokens, false, "success", "")

			// FIXME: now we use req model as resp model
			anthropicResp.Model = anthropic.Model(proxyModel)
			c.JSON(http.StatusOK, anthropicResp)
		}
		return

	case protocol.APIStyleGoogle:
		// Check if adaptor is enabled
		if !s.enableAdaptor {
			SendAdapterDisabledError(c, provider.Name)
			return
		}

		// Convert Anthropic request to Google format
		model, googleReq, cfg := request.ConvertAnthropicToGoogleRequest(&req.MessageNewParams, 0)

		if isStreaming {
			// Create streaming request
			streamResp, err := s.forwardGoogleStreamRequest(provider, model, googleReq, cfg)
			if err != nil {
				SendStreamingError(c, err)
				return
			}

			// Handle the streaming response
			err = stream.HandleGoogleToAnthropicStreamResponse(c, streamResp, proxyModel)
			if err != nil {
				SendInternalError(c, err.Error())
			}

			// Track usage from stream (would be accumulated in handler)
			// For Google, usage is tracked in the stream handler

		} else {
			// Handle non-streaming request
			response, err := s.forwardGoogleRequest(provider, model, googleReq, cfg)
			if err != nil {
				SendForwardingError(c, err)
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
			s.trackUsage(c, rule, provider, actualModel, proxyModel, inputTokens, outputTokens, false, "success", "")

			c.JSON(http.StatusOK, anthropicResp)
		}

	case protocol.APIStyleOpenAI:
		// Check if adaptor is enabled
		if !s.enableAdaptor {
			SendAdapterDisabledError(c, provider.Name)
			return
		}

		// Check if model prefers Responses API (for models like Codex)
		// This is used for ChatGPT backend API which only supports Responses API
		useResponsesAPI := selectedService.PreferCompletions()
		logrus.Infof("[AnthropicV1] Checking Responses API for model=%s, provider=%s, PreferCompletions=%v", actualModel, provider.Name, useResponsesAPI)

		// Also check the probe cache if not already determined
		if !useResponsesAPI {
			preferredEndpoint := s.GetPreferredEndpointForModel(provider, actualModel)
			logrus.Infof("[AnthropicV1] Probe cache preferred endpoint for model=%s: %s", actualModel, preferredEndpoint)
			useResponsesAPI = preferredEndpoint == "responses"
		}

		if useResponsesAPI {
			// Responses API requires Beta format - convert v1 to beta and route to beta handler
			betaParams, err := convertV1RequestToBeta(&req.MessageNewParams)
			if err != nil {
				c.JSON(http.StatusBadRequest, ErrorResponse{
					Error: ErrorDetail{
						Message: fmt.Sprintf("Failed to convert request to Beta format: %v", err),
						Type:    "invalid_request_error",
					},
				})
				return
			}
			betaReq := protocol.AnthropicBetaMessagesRequest{
				Stream:                req.Stream,
				BetaMessageNewParams:  betaParams,
			}
			// Set context flag to indicate original request was v1 format
			// The beta handler will use this to convert responses back to v1 format
			c.Set("original_request_format", "v1")
			logrus.Infof("[AnthropicV1] Converting v1 to beta format for model=%s", actualModel)
			s.anthropicMessagesV1Beta(c, betaReq, proxyModel, provider, selectedService, rule)
			return
		}

		logrus.Infof("[AnthropicV1] Using Chat Completions API for model=%s", actualModel)
		// Use OpenAI conversion path (default behavior)
		if isStreaming {
			// Convert Anthropic request to OpenAI format for streaming
			openaiReq := request.ConvertAnthropicToOpenAIRequestWithProvider(&req.MessageNewParams, true, provider, actualModel)

			// Create streaming request
			streamResp, err := s.forwardOpenAIStreamRequest(provider, openaiReq)
			if err != nil {
				SendStreamingError(c, err)
				return
			}

			// Handle the streaming response
			err = stream.HandleOpenAIToAnthropicStreamResponse(c, openaiReq, streamResp, proxyModel)
			if err != nil {
				SendInternalError(c, err.Error())
			}

		} else {
			// Handle non-streaming request
			// Convert Anthropic request to OpenAI format with provider transforms
			openaiReq := request.ConvertAnthropicToOpenAIRequestWithProvider(&req.MessageNewParams, true, provider, actualModel)
			response, err := s.forwardOpenAIRequest(provider, openaiReq)
			if err != nil {
				SendForwardingError(c, err)
				return
			}
			// Convert OpenAI response back to Anthropic format
			anthropicResp := nonstream.ConvertOpenAIToAnthropicResponse(response, proxyModel)
			c.JSON(http.StatusOK, anthropicResp)
		}
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
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
func (s *Server) forwardAnthropicStreamRequestV1(provider *typ.Provider, req anthropic.MessageNewParams, scenario string) (*anthropicstream.Stream[anthropic.MessageStreamEventUnion], error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	logrus.Debugln("Creating Anthropic streaming request")

	// Create context with scenario for recording
	ctx := context.WithValue(context.Background(), client.ScenarioContextKey, scenario)

	// Use background context for streaming
	// The stream will manage its own lifecycle and timeout
	// We don't use a timeout here because streaming responses can take longer
	streamResp := wrapper.MessagesNewStreaming(ctx, req)

	return streamResp, nil
}

// handleAnthropicStreamResponseV1 processes the Anthropic streaming response and sends it to the client (v1)
func (s *Server) handleAnthropicStreamResponseV1(c *gin.Context, req anthropic.MessageNewParams, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion], respModel, actualModel string, rule *typ.Rule, provider *typ.Provider, recorder *ScenarioRecorder) {
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
		// Track usage with error status
		if hasUsage {
			s.trackUsage(c, rule, provider, actualModel, respModel, inputTokens, outputTokens, true, "error", "stream_error")
		}
		MarshalAndSendErrorEvent(c, err.Error(), "stream_error", "stream_failed")
		// Record error
		streamRec.RecordError(err)
		return
	}

	// Track successful streaming completion
	if hasUsage {
		s.trackUsage(c, rule, provider, actualModel, respModel, inputTokens, outputTokens, true, "success", "")
	}

	// Send completion event
	SendFinishEvent(c)

	// Record the response after stream completes
	streamRec.RecordResponse(provider, actualModel)
}

// anthropicCountTokensV1 implements standard v1 count_tokens
func (s *Server) anthropicCountTokensV1(c *gin.Context, bodyBytes []byte, rawReq map[string]interface{}, model string, provider *typ.Provider, selectedService *loadbalance.Service) {
	// Use the selected service's model
	actualModel := selectedService.Model

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", selectedService.Model)

	// Check provider's API style to decide which path to take
	apiStyle := provider.APIStyle

	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, actualModel)

	// Make the request using Anthropic SDK with timeout (provider.Timeout is in seconds)
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Parse into MessageCountTokensParams using SDK's JSON unmarshaling
	var req anthropic.MessageCountTokensParams
	if err := c.ShouldBindJSON(&req); err != nil {
		// Log the invalid request for debugging
		logrus.Debugf("Invalid JSON request received: %v\nBody: %s", err, string(bodyBytes))
		SendInvalidRequestBodyError(c, err)
		return
	}

	req.Model = anthropic.Model(actualModel)

	// If the provider uses Anthropic API style, use the actual count_tokens endpoint
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
		message, err := wrapper.MessagesCountTokens(ctx, req)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, message)
	case protocol.APIStyleOpenAI:
		count, err := token.CountTokensWithTiktoken(string(req.Model), req.Messages, req.System.OfTextBlockArray)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, anthropic.MessageTokensCount{
			InputTokens: int64(count),
		})
	}
}

// trackUsage records token usage using the UsageTracker
func (s *Server) trackUsage(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model, requestModel string, inputTokens, outputTokens int, streamed bool, status, errorCode string) {
	tracker := s.NewUsageTracker()
	tracker.RecordUsage(c, rule, provider, model, requestModel, inputTokens, outputTokens, streamed, status, errorCode)
}

// convertV1RequestToBeta converts Anthropic v1 MessageNewParams to Beta format
// Since v1 and beta share the same JSON representation, we can marshal v1 to JSON and unmarshal as beta
func convertV1RequestToBeta(v1Req *anthropic.MessageNewParams) (anthropic.BetaMessageNewParams, error) {
	// Marshal v1 request to JSON
	jsonBytes, err := json.Marshal(v1Req)
	if err != nil {
		return anthropic.BetaMessageNewParams{}, fmt.Errorf("failed to marshal v1 request: %w", err)
	}

	// Unmarshal as Beta format
	var betaReq anthropic.BetaMessageNewParams
	if err := json.Unmarshal(jsonBytes, &betaReq); err != nil {
		return anthropic.BetaMessageNewParams{}, fmt.Errorf("failed to unmarshal as beta request: %w", err)
	}

	return betaReq, nil
}
