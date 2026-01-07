package server

import (
	"context"
	"encoding/json"
	"fmt"
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

// sendSSEvent sends a generic SSE event with JSON data
func sendSSEvent(c *gin.Context, eventType string, data interface{}) error {
	eventJSON, err := json.Marshal(data)
	if err != nil {
		logrus.Debugf("Failed to marshal SSE event: %v", err)
		return err
	}
	c.SSEvent(eventType, string(eventJSON))
	return nil
}

// anthropicMessagesV1 implements standard v1 messages API
func (s *Server) anthropicMessagesV1(c *gin.Context, bodyBytes []byte, rawReq map[string]interface{}, proxyModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule) {
	actualModel := selectedService.Model

	// Check if streaming is requested
	isStreaming := false
	if stream, ok := rawReq["stream"].(bool); ok {
		isStreaming = stream
	}
	logrus.Debugf("Stream requested for AnthropicMessages: %v", isStreaming)

	// Parse into MessageNewParams using SDK's JSON unmarshaling
	var req anthropic.MessageNewParams
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
			stream, err := s.forwardAnthropicStreamRequestV1(provider, req)
			if err != nil {
				SendStreamingError(c, err)
				return
			}
			// Handle the streaming response
			s.handleAnthropicStreamResponseV1(c, req, stream, proxyModel)
		} else {
			// Handle non-streaming request
			anthropicResp, err := s.forwardAnthropicRequestV1(provider, req)
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
			// Convert Anthropic request to OpenAI format for streaming
			openaiReq := adaptor.ConvertAnthropicToOpenAIRequest(&req, true)

			da, _ := json.MarshalIndent(req, "", "    ")
			fmt.Printf("requst to openai: %s\n", da)

			do, _ := json.MarshalIndent(openaiReq, "", "    ")
			fmt.Printf("requst to openai: %s\n", do)

			// Create streaming request
			stream, err := s.forwardOpenAIStreamRequest(provider, openaiReq)
			if err != nil {
				SendStreamingError(c, err)
				return
			}

			// Handle the streaming response
			err = adaptor.HandleOpenAIToAnthropicStreamResponse(c, openaiReq, stream, proxyModel)
			if err != nil {
				SendInternalError(c, err.Error())
			}

		} else {
			// Handle non-streaming request
			openaiReq := adaptor.ConvertAnthropicToOpenAIRequest(&req, true)
			response, err := s.forwardOpenAIRequest(provider, openaiReq)
			if err != nil {
				SendForwardingError(c, err)
				return
			}
			// Convert OpenAI response back to Anthropic format
			anthropicResp := adaptor.ConvertOpenAIToAnthropicResponse(response, proxyModel)
			c.JSON(http.StatusOK, anthropicResp)
		}
	}
}

// forwardAnthropicRequestV1 forwards request using Anthropic SDK with proper types (v1)
func (s *Server) forwardAnthropicRequestV1(provider *typ.Provider, req anthropic.MessageNewParams) (*anthropic.Message, error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	// Make the request using Anthropic SDK with timeout (provider.Timeout is in seconds)
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	message, err := wrapper.MessagesNew(ctx, req)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// forwardAnthropicStreamRequestV1 forwards streaming request using Anthropic SDK (v1)
func (s *Server) forwardAnthropicStreamRequestV1(provider *typ.Provider, req anthropic.MessageNewParams) (*anthropicstream.Stream[anthropic.MessageStreamEventUnion], error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	logrus.Debugln("Creating Anthropic streaming request")

	// Use background context for streaming
	// The stream will manage its own lifecycle and timeout
	// We don't use a timeout here because streaming responses can take longer
	ctx := context.Background()
	stream := wrapper.MessagesNewStreaming(ctx, req)

	return stream, nil
}

// handleAnthropicStreamResponseV1 processes the Anthropic streaming response and sends it to the client (v1)
func (s *Server) handleAnthropicStreamResponseV1(c *gin.Context, req anthropic.MessageNewParams, stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion], respModel string) {
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
		event.Message.Model = anthropic.Model(respModel)

		// Convert the event to JSON and send as SSE
		if err := sendSSEvent(c, event.Type, event); err != nil {
			logrus.Debugf("Failed to marshal Anthropic stream event: %v", err)
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

// anthropicCountTokensV1 implements standard v1 count_tokens
func (s *Server) anthropicCountTokensV1(c *gin.Context, bodyBytes []byte, rawReq map[string]interface{}, model string, provider *typ.Provider, selectedService *loadbalance.Service) {
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
	if apiStyle == "anthropic" {
		message, err := wrapper.MessagesCountTokens(ctx, req)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, message)
	} else {
		count, err := countTokensWithTiktoken(string(req.Model), req.Messages, req.System.OfTextBlockArray)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, anthropic.MessageTokensCount{
			InputTokens: int64(count),
		})
	}
}
