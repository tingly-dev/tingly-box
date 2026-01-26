package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"iter"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// anthropicMessagesV1Beta implements beta messages API
func (s *Server) anthropicMessagesV1Beta(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule) {
	actualModel := selectedService.Model

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
			stream, err := s.forwardAnthropicStreamRequestV1Beta(provider, req.BetaMessageNewParams)
			if err != nil {
				s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "stream_creation_failed")
				SendStreamingError(c, err)
				return
			}
			// Handle the streaming response
			s.handleAnthropicStreamResponseV1Beta(c, req.BetaMessageNewParams, stream, proxyModel, actualModel, rule, provider)
		} else {
			// Handle non-streaming request
			anthropicResp, err := s.forwardAnthropicRequestV1Beta(provider, req.BetaMessageNewParams)
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
		if !s.enableAdaptor {
			SendAdapterDisabledError(c, provider.Name)
			return
		}

		// Convert Anthropic beta request to Google format
		model, googleReq, cfg := request.ConvertAnthropicBetaToGoogleRequest(&req.BetaMessageNewParams, 0)

		if isStreaming {
			// Create streaming request
			streamResp, err := s.forwardGoogleStreamRequest(provider, model, googleReq, cfg)
			if err != nil {
				SendStreamingError(c, err)
				return
			}

			// Handle the streaming response
			err = stream.HandleGoogleToAnthropicBetaStreamResponse(c, streamResp, proxyModel)
			if err != nil {
				SendInternalError(c, err.Error())
			}

			// Track usage from stream (would be accumulated in handler)
			// For Google, usage is tracked in the stream handler

		} else {
			// Handle non-streaming request
			resp, err := s.forwardGoogleRequest(provider, model, googleReq, cfg)
			if err != nil {
				SendForwardingError(c, err)
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

		// Also check the probe cache if not already determined
		if !useResponsesAPI {
			preferredEndpoint := s.GetPreferredEndpointForModel(provider, actualModel)
			useResponsesAPI = (preferredEndpoint == "responses")
		}

		if useResponsesAPI {
			// Use Responses API path (for Codex and other models that prefer it)
			s.handleAnthropicV1BetaViaResponsesAPI(c, req, proxyModel, actualModel, provider, selectedService, rule, isStreaming)
		} else {
			// Use Chat Completions path (fallback)
			s.handleAnthropicV1BetaViaChatCompletions(c, req, proxyModel, actualModel, provider, selectedService, rule, isStreaming)
		}
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
	}
}

// forwardAnthropicRequestV1Beta forwards request using Anthropic SDK with proper types (beta)
func (s *Server) forwardAnthropicRequestV1Beta(provider *typ.Provider, req anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	// Make the request using Anthropic SDK with timeout (provider.Timeout is in seconds)
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	message, err := wrapper.BetaMessagesNew(ctx, req)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// forwardAnthropicStreamRequestV1Beta forwards streaming request using Anthropic SDK (beta)
func (s *Server) forwardAnthropicStreamRequestV1Beta(provider *typ.Provider, req anthropic.BetaMessageNewParams) (*anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	logrus.Debugln("Creating Anthropic beta streaming request")

	// Use background context for streaming
	// The stream will manage its own lifecycle and timeout
	// We don't use a timeout here because streaming responses can take longer
	ctx := context.Background()
	stream := wrapper.BetaMessagesNewStreaming(ctx, req)

	return stream, nil
}

// handleAnthropicStreamResponseV1Beta processes the Anthropic beta streaming response and sends it to the client
func (s *Server) handleAnthropicStreamResponseV1Beta(c *gin.Context, req anthropic.BetaMessageNewParams, stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], respModel, actualModel string, rule *typ.Rule, provider *typ.Provider) {
	// Accumulate usage from stream
	var inputTokens, outputTokens int
	var hasUsage bool

	// Set SSE headers
	SetupSSEHeaders(c)

	// Check SSE support
	if !CheckSSESupport(c) {
		return
	}

	flusher, _ := c.Writer.(http.Flusher)

	// Process the stream
	for stream.Next() {
		event := stream.Current()
		event.Message.Model = anthropic.Model(respModel)

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
		if err := sendSSEvent(c, event.Type, event); err != nil {
			logrus.Debugf("Failed to marshal Anthropic beta stream event: %v", err)
			continue
		}
		flusher.Flush()
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Track usage with error status
		if hasUsage {
			s.trackUsage(c, rule, provider, actualModel, respModel, inputTokens, outputTokens, true, "error", "stream_error")
		}
		MarshalAndSendErrorEvent(c, err.Error(), "stream_error", "stream_failed")
		flusher.Flush()
		return
	}

	// Track successful streaming completion
	if hasUsage {
		s.trackUsage(c, rule, provider, actualModel, respModel, inputTokens, outputTokens, true, "success", "")
	}

	// Send completion event
	SendFinishEvent(c)
	flusher.Flush()
}

// forwardGoogleRequest forwards request to Google API
func (s *Server) forwardGoogleRequest(provider *typ.Provider, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	// Get or create Google client wrapper from pool
	wrapper := s.clientPool.GetGoogleClient(provider, model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
	}

	// Make the request with timeout
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	response, err := wrapper.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// forwardGoogleStreamRequest forwards streaming request to Google API
func (s *Server) forwardGoogleStreamRequest(provider *typ.Provider, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (iter.Seq2[*genai.GenerateContentResponse, error], error) {
	// Get or create Google client wrapper from pool
	wrapper := s.clientPool.GetGoogleClient(provider, model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
	}

	logrus.Debugln("Creating Google streaming request")

	// Use background context for streaming
	ctx := context.Background()
	stream := wrapper.GenerateContentStream(ctx, model, contents, config)

	return stream, nil
}

// anthropicCountTokensV1Beta implements beta count_tokens
func (s *Server) anthropicCountTokensV1Beta(c *gin.Context, bodyBytes []byte, rawReq map[string]interface{}, model string, provider *typ.Provider, selectedService *loadbalance.Service) {
	// Use the selected service's model
	actualModel := selectedService.Model

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", selectedService.Model)

	// Check provider's API style to decide which path to take
	apiStyle := string(provider.APIStyle)
	if apiStyle == "" {
		apiStyle = "openai" // default to openai
	}

	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, actualModel)

	// Make the request using Anthropic SDK with timeout (provider.Timeout is in seconds)
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Parse into BetaMessageCountTokensParams using SDK's JSON unmarshaling
	var req anthropic.BetaMessageCountTokensParams
	if err := c.ShouldBindJSON(&req); err != nil {
		// Log the invalid request for debugging
		logrus.Debugf("Invalid JSON request received: %v\nBody: %s", err, string(bodyBytes))
		SendInvalidRequestBodyError(c, err)
		return
	}

	req.Model = anthropic.Model(actualModel)

	// If the provider uses Anthropic API style, use the actual count_tokens endpoint
	if apiStyle == "anthropic" {
		message, err := wrapper.BetaMessagesCountTokens(ctx, req)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}

		c.JSON(http.StatusOK, message)
	} else {
		count, err := token.CountBetaTokensWithTiktoken(string(req.Model), req.Messages, req.System)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, anthropic.MessageTokensCount{
			InputTokens: int64(count),
		})
	}
}

// handleAnthropicV1BetaViaChatCompletions handles Anthropic v1beta request using OpenAI Chat Completions API
func (s *Server) handleAnthropicV1BetaViaChatCompletions(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule, isStreaming bool) {
	// Convert Anthropic beta request to OpenAI format
	openaiReq := request.ConvertAnthropicBetaToOpenAIRequestWithProvider(&req.BetaMessageNewParams, true, provider, actualModel)

	// Use OpenAI Chat Completions path
	if isStreaming {
		// Create streaming request
		streamResp, err := s.forwardOpenAIStreamRequest(provider, openaiReq)
		if err != nil {
			SendStreamingError(c, err)
			return
		}

		// Handle the streaming response
		err = stream.HandleOpenAIToAnthropicV1BetaStreamResponse(c, openaiReq, streamResp, proxyModel)
		if err != nil {
			SendInternalError(c, err.Error())
		}

	} else {
		resp, err := s.forwardOpenAIRequest(provider, openaiReq)
		if err != nil {
			SendForwardingError(c, err)
			return
		}
		// Convert OpenAI response back to Anthropic beta format
		anthropicResp := nonstream.ConvertOpenAIToAnthropicBetaResponse(resp, proxyModel)
		c.JSON(http.StatusOK, anthropicResp)
	}
}

// handleAnthropicV1BetaViaResponsesAPI handles Anthropic v1beta request using OpenAI Responses API
func (s *Server) handleAnthropicV1BetaViaResponsesAPI(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule, isStreaming bool) {
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
		s.handleAnthropicV1BetaViaResponsesAPIStreaming(c, req, proxyModel, actualModel, provider, selectedService, rule, responsesReq)
	} else {
		s.handleAnthropicV1BetaViaResponsesAPINonStreaming(c, req, proxyModel, actualModel, provider, selectedService, rule, responsesReq)
	}
}

// handleAnthropicV1BetaViaResponsesAPINonStreaming handles non-streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPINonStreaming(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule, responsesReq responses.ResponseNewParams) {
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
		s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "forward_failed")
		SendForwardingError(c, err)
		return
	}

	// Extract usage from response
	inputTokens := int(response.Usage.InputTokens)
	outputTokens := int(response.Usage.OutputTokens)

	// Track usage
	s.trackUsage(c, rule, provider, actualModel, proxyModel, inputTokens, outputTokens, false, "success", "")

	// Convert Responses API response back to Anthropic beta format
	anthropicResp := nonstream.ConvertResponsesToAnthropicBetaResponse(response, proxyModel)
	c.JSON(http.StatusOK, anthropicResp)
}

// handleAnthropicV1BetaViaResponsesAPIStreaming handles streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPIStreaming(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule, responsesReq responses.ResponseNewParams) {
	// Check if this is a ChatGPT backend API provider
	// These providers need special handling because they use custom HTTP implementation
	if provider.APIBase == protocol.ChatGPTBackendAPIBase {
		// Use the ChatGPT backend API streaming handler
		// This handler sends the stream directly to the client in OpenAI Responses API format
		s.handleChatGPTBackendStreamingRequest(c, provider, responsesReq, proxyModel, actualModel, rule)
		return
	}

	// For standard OpenAI providers, use the OpenAI SDK
	streamResp, cancel, err := s.forwardResponsesStreamRequest(provider, responsesReq)
	if err != nil {
		s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "stream_creation_failed")
		SendStreamingError(c, err)
		return
	}
	defer cancel()

	// Handle the streaming response
	// Use the dedicated stream handler to convert Responses API to Anthropic beta format
	err = stream.HandleResponsesToAnthropicV1BetaStreamResponse(c, streamResp, proxyModel)

	// Track usage from stream (would be accumulated in handler)
	// For now, we'll track minimal usage since the handler manages it
	if err != nil {
		s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "stream_error")
		return
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}
