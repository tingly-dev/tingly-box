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

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/toolinterceptor"
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
		// === Check if provider has built-in web_search ===
		hasBuiltInWebSearch := s.templateManager.ProviderHasBuiltInWebSearch(provider)

		// === Tool Interceptor: Check if enabled and should be used ===
		// Only intercept if provider does NOT have built-in web_search
		shouldIntercept := !hasBuiltInWebSearch && s.toolInterceptor != nil && s.toolInterceptor.IsEnabledForProvider(provider)

		if shouldIntercept {
			logrus.Infof("Tool interceptor active for provider %s (no built-in web_search)", provider.Name)
		} else if hasBuiltInWebSearch {
			logrus.Infof("Provider %s has built-in web_search, using native implementation", provider.Name)
		}

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
			// Handle non-streaming request with tool interception (only if needed)
			anthropicResp, err := s.forwardAnthropicRequestV1(provider, req.MessageNewParams, scenario)
			if err != nil {
				s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "forward_failed")
				SendForwardingError(c, err)
				return
			}

			// === Tool Interception: Only intercept if provider doesn't have built-in web_search ===
			if shouldIntercept && len(anthropicResp.Content) > 0 {
				hasInterceptedTools := false
				for _, block := range anthropicResp.Content {
					if block.Type == "tool_use" && toolinterceptor.ShouldInterceptTool(block.Name) {
						hasInterceptedTools = true
						break
					}
				}

				if hasInterceptedTools {
					// Execute intercepted tools locally and get final response
					finalResponse, err := s.handleInterceptedAnthropicToolCalls(provider, &req.MessageNewParams, anthropicResp, actualModel, scenario)
					if err != nil {
						s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "tool_interception_failed")
						SendForwardingError(c, fmt.Errorf("failed to handle tool calls: %w", err))
						return
					}

					// Track usage from final response
					inputTokens := int(finalResponse.Usage.InputTokens)
					outputTokens := int(finalResponse.Usage.OutputTokens)
					s.trackUsage(c, rule, provider, actualModel, proxyModel, inputTokens, outputTokens, false, "success", "")

					// Return final response
					finalResponse.Model = anthropic.Model(proxyModel)
					c.JSON(http.StatusOK, finalResponse)
					return
				}
			}

			// Track usage from response (no tool interception occurred)
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

	// Create stream assembler (pure assembler, no recorder dependency)
	assembler := stream.NewAnthropicStreamAssembler()

	// Enable streaming on recorder if provided
	if recorder != nil {
		recorder.EnableStreaming()
	}

	// Set SSE headers
	SetupSSEHeaders(c)

	// Check SSE support
	if !CheckSSESupport(c) {
		return
	}

	flusher, _ := c.Writer.(http.Flusher)

	// Process the stream
	for streamResp.Next() {
		event := streamResp.Current()
		event.Message.Model = anthropic.Model(respModel)

		// Record raw chunk to recorder if provided
		if recorder != nil {
			recorder.RecordStreamChunk(event.Type, event)
			// Assemble the response
			assembler.RecordV1Event(&event)
		}

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
		flusher.Flush()
	}

	// Set assembled response on recorder if provided
	if recorder != nil {
		// Finish assembling - get the assembled message
		assembled := assembler.Finish(respModel, inputTokens, outputTokens)
		if assembled != nil {
			recorder.SetAssembledResponse(assembled)
		}
	}

	// Check for stream errors
	if err := streamResp.Err(); err != nil {
		// Track usage with error status
		if hasUsage {
			s.trackUsage(c, rule, provider, actualModel, respModel, inputTokens, outputTokens, true, "error", "stream_error")
		}
		MarshalAndSendErrorEvent(c, err.Error(), "stream_error", "stream_failed")
		flusher.Flush()
		// Record error
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	// Track successful streaming completion
	if hasUsage {
		s.trackUsage(c, rule, provider, actualModel, respModel, inputTokens, outputTokens, true, "success", "")
	}

	// Send completion event
	SendFinishEvent(c)
	flusher.Flush()

	// Record the response after stream completes
	if recorder != nil {
		recorder.RecordResponse()
	}
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

// handleInterceptedAnthropicToolCalls executes intercepted Anthropic tool calls locally and returns final response
func (s *Server) handleInterceptedAnthropicToolCalls(provider *typ.Provider, originalReq *anthropic.MessageNewParams, toolCallResponse *anthropic.Message, actualModel string, scenario string) (*anthropic.Message, error) {
	logrus.Infof("Handling %d intercepted Anthropic tool calls for provider %s", len(toolCallResponse.Content), provider.Name)

	// Build new messages list with original messages
	newMessages := make([]anthropic.MessageParam, len(originalReq.Messages))
	copy(newMessages, originalReq.Messages)

	// Add assistant message with tool_use blocks
	asstContentBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(toolCallResponse.Content))
	for _, block := range toolCallResponse.Content {
		if block.Type == "text" {
			asstContentBlocks = append(asstContentBlocks, anthropic.NewTextBlock(block.Text))
		} else if block.Type == "tool_use" {
			toolUseParam := anthropic.ToolUseBlockParam{
				ID:    block.ID,
				Name:  block.Name,
				Input: json.RawMessage(block.Input),
			}
			asstContentBlocks = append(asstContentBlocks, anthropic.ContentBlockParamUnion{OfToolUse: &toolUseParam})
		}
	}
	newMessages = append(newMessages, anthropic.NewAssistantMessage(asstContentBlocks...))

	// Execute each intercepted tool_use block
	for _, block := range toolCallResponse.Content {
		if block.Type != "tool_use" {
			continue
		}

		// Check if this tool should be intercepted
		if !toolinterceptor.ShouldInterceptTool(block.Name) {
			// This shouldn't happen since we checked before calling this function
			continue
		}

		// Execute the tool using the interceptor
		result := s.toolInterceptor.ExecuteTool(provider, block.Name, string(block.Input))

		// Add tool result block
		var toolResultContent string
		var isError bool
		if result.IsError {
			toolResultContent = fmt.Sprintf("Error: %s", result.Error)
			isError = true
		} else {
			toolResultContent = result.Content
			isError = false
		}

		toolResultBlock := anthropic.NewToolResultBlock(block.ID, toolResultContent, isError)
		newMessages = append(newMessages, anthropic.NewUserMessage(toolResultBlock))
		logrus.Infof("Executed Anthropic tool %s locally", block.Name)
	}

	// Create new request with updated messages
	followUpReq := *originalReq
	followUpReq.Messages = newMessages

	// Forward to provider for final response
	finalResponse, err := s.forwardAnthropicRequestV1(provider, followUpReq, scenario)
	if err != nil {
		return nil, fmt.Errorf("failed to get final response after tool execution: %w", err)
	}

	return finalResponse, nil
}
