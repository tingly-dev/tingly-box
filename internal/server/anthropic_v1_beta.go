package server

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"time"
	"tingly-box/pkg/adaptor"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"google.golang.org/genai"

	"tingly-box/internal/loadbalance"
	"tingly-box/internal/typ"
)

// anthropicMessagesV1Beta implements beta messages API
func (s *Server) anthropicMessagesV1Beta(c *gin.Context, req AnthropicBetaMessagesRequest, proxyModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule) {
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
	case typ.APIStyleAnthropic:
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
	case typ.APIStyleGoogle:
		if !s.enableAdaptor {
			SendAdapterDisabledError(c, provider.Name)
			return
		}

		// Convert Anthropic beta request to Google format
		model, googleReq, cfg := adaptor.ConvertAnthropicBetaToGoogleRequest(&req.BetaMessageNewParams, 0)

		if isStreaming {
			// Create streaming request
			stream, err := s.forwardGoogleStreamRequest(provider, model, googleReq, cfg)
			if err != nil {
				SendStreamingError(c, err)
				return
			}

			// Handle the streaming response
			err = adaptor.HandleGoogleToAnthropicBetaStreamResponse(c, stream, proxyModel)
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

			// Convert Google response to Anthropic beta format
			anthropicResp := adaptor.ConvertGoogleToAnthropicBetaResponse(response, proxyModel)

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
	case typ.APIStyleOpenAI:
		// Check if adaptor is enabled
		if !s.enableAdaptor {
			SendAdapterDisabledError(c, provider.Name)
			return
		}

		// Convert Anthropic beta request to OpenAI format for streaming
		openaiReq := adaptor.ConvertAnthropicBetaToOpenAIRequest(&req.BetaMessageNewParams, true)

		// Use OpenAI conversion path (default behavior)
		if isStreaming {
			// Create streaming request
			stream, err := s.forwardOpenAIStreamRequest(provider, openaiReq)
			if err != nil {
				SendStreamingError(c, err)
				return
			}

			// Handle the streaming response
			err = adaptor.HandleOpenAIToAnthropicV1BetaStreamResponse(c, openaiReq, stream, proxyModel)
			if err != nil {
				SendInternalError(c, err.Error())
			}

		} else {
			response, err := s.forwardOpenAIRequest(provider, openaiReq)
			if err != nil {
				SendForwardingError(c, err)
				return
			}
			// Convert OpenAI response back to Anthropic beta format
			anthropicResp := adaptor.ConvertOpenAIToAnthropicBetaResponse(response, proxyModel)
			c.JSON(http.StatusOK, anthropicResp)
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
		count, err := countBetaTokensWithTiktoken(string(req.Model), req.Messages, req.System)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, anthropic.MessageTokensCount{
			InputTokens: int64(count),
		})
	}
}
