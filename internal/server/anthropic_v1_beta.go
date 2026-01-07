package server

import (
	"context"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"tingly-box/internal/loadbalance"
	"tingly-box/internal/typ"
	"tingly-box/pkg/adaptor"
)

// anthropicMessagesBeta implements beta messages API
func (s *Server) anthropicMessagesBeta(c *gin.Context, bodyBytes []byte, rawReq map[string]interface{}, proxyModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule) {
	actualModel := selectedService.Model

	// Check if streaming is requested
	isStreaming := false
	if stream, ok := rawReq["stream"].(bool); ok {
		isStreaming = stream
	}
	logrus.Debugf("Stream requested for AnthropicMessages (beta): %v", isStreaming)

	// Parse into BetaMessageNewParams using SDK's JSON unmarshaling
	var req anthropic.BetaMessageNewParams
	if err := c.ShouldBindJSON(&req); err != nil {
		// Log the invalid request for debugging
		logrus.Debugf("Invalid JSON request received: %v\nBody: %s", err, string(bodyBytes))
		SendInvalidRequestBodyError(c, err)
		return
	}

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
	apiStyle := string(provider.APIStyle)
	if apiStyle == "" {
		apiStyle = "openai" // default to openai
	}

	if apiStyle == "anthropic" {
		// Use direct Anthropic SDK call
		if isStreaming {
			// Handle streaming request
			stream, err := s.forwardAnthropicStreamRequestBeta(provider, req)
			if err != nil {
				SendStreamingError(c, err)
				return
			}
			// Handle the streaming response
			s.handleAnthropicStreamResponseBeta(c, req, stream, proxyModel)
		} else {
			// Handle non-streaming request
			anthropicResp, err := s.forwardAnthropicRequestBeta(provider, req)
			if err != nil {
				SendForwardingError(c, err)
				return
			}
			// FIXME: now we use req model as resp model
			anthropicResp.Model = anthropic.Model(proxyModel)
			c.JSON(http.StatusOK, anthropicResp)
		}
		return
	} else {
		// Check if adaptor is enabled
		if !s.enableAdaptor {
			SendAdapterDisabledError(c, provider.Name)
			return
		}

		// Use OpenAI conversion path (default behavior)
		if isStreaming {
			// Convert Anthropic beta request to OpenAI format for streaming
			openaiReq := adaptor.ConvertAnthropicBetaToOpenAIRequest(&req, true)

			// Create streaming request
			stream, err := s.forwardOpenAIStreamRequest(provider, openaiReq)
			if err != nil {
				SendStreamingError(c, err)
				return
			}

			// Handle the streaming response
			err = adaptor.HandleOpenAIToAnthropicBetaStreamResponse(c, openaiReq, stream, proxyModel)
			if err != nil {
				SendInternalError(c, err.Error())
			}

		} else {
			// Handle non-streaming request - convert beta response to OpenAI and back
			openaiReq := adaptor.ConvertAnthropicBetaToOpenAIRequest(&req, true)
			response, err := s.forwardOpenAIRequest(provider, openaiReq)
			if err != nil {
				SendForwardingError(c, err)
				return
			}
			// Convert OpenAI response back to Anthropic beta format
			anthropicResp := adaptor.ConvertOpenAIToAnthropicBetaResponse(response, proxyModel)
			c.JSON(http.StatusOK, anthropicResp)
		}
	}
}

// forwardAnthropicRequestBeta forwards request using Anthropic SDK with proper types (beta)
func (s *Server) forwardAnthropicRequestBeta(provider *typ.Provider, req anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
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

// forwardAnthropicStreamRequestBeta forwards streaming request using Anthropic SDK (beta)
func (s *Server) forwardAnthropicStreamRequestBeta(provider *typ.Provider, req anthropic.BetaMessageNewParams) (*anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], error) {
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

// handleAnthropicStreamResponseBeta processes the Anthropic beta streaming response and sends it to the client
func (s *Server) handleAnthropicStreamResponseBeta(c *gin.Context, req anthropic.BetaMessageNewParams, stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], respModel string) {
	defer StreamRecoveryHandler(c, stream)

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

		// Convert the event to JSON and send as SSE
		if err := sendSSEvent(c, event.Type, event); err != nil {
			logrus.Debugf("Failed to marshal Anthropic beta stream event: %v", err)
			continue
		}
		flusher.Flush()
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		MarshalAndSendErrorEvent(c, err.Error(), "stream_error", "stream_failed")
		flusher.Flush()
		return
	}

	// Send completion event
	SendFinishEvent(c)
	flusher.Flush()
}

// anthropicCountTokensBeta implements beta count_tokens
func (s *Server) anthropicCountTokensBeta(c *gin.Context, bodyBytes []byte, rawReq map[string]interface{}, model string, provider *typ.Provider, selectedService *loadbalance.Service) {
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
