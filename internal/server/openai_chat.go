package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/toolinterceptor"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// handleNonStreamingRequest handles non-streaming chat completion requests
func (s *Server) handleNonStreamingRequest(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, rule *typ.Rule, shouldIntercept, shouldStripTools bool) {
	// === PRE-REQUEST INTERCEPTION: Strip tools before sending to provider ===
	req := originalReq
	if shouldIntercept {
		preparedReq, _ := s.toolInterceptor.PrepareOpenAIRequest(provider, originalReq)
		req = preparedReq
	} else if shouldStripTools {
		req = toolinterceptor.StripSearchFetchToolsOpenAI(originalReq)
	}

	// Forward request to provider
	wrapper := s.clientPool.GetOpenAIClient(provider, string(req.Model))
	fc := NewForwardContext(nil, provider)
	response, err := ForwardOpenAIChat(fc, wrapper, req)
	if err != nil {
		// Track error with no usage
		s.trackUsageFromContext(c, 0, 0, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// === POST-RESPONSE INTERCEPTION: Handle tool calls from provider ===
	if shouldIntercept && len(response.Choices) > 0 {
		choice := response.Choices[0]
		if len(choice.Message.ToolCalls) > 0 {
			// Check if any tool calls should be intercepted
			hasInterceptedCalls := false
			for _, tc := range choice.Message.ToolCalls {
				fn := tc.Function
				if fn.Name != "" && toolinterceptor.ShouldInterceptTool(fn.Name) {
					hasInterceptedCalls = true
					break
				}
			}

			if hasInterceptedCalls {
				// Execute intercepted tool calls locally and get final response
				finalResponse, err := s.handleInterceptedToolCalls(provider, originalReq, response)
				if err != nil {
					s.trackUsageFromContext(c, 0, 0, err)
					c.JSON(http.StatusInternalServerError, ErrorResponse{
						Error: ErrorDetail{
							Message: "Failed to handle tool calls: " + err.Error(),
							Type:    "api_error",
						},
					})
					return
				}

				// Extract usage from final response
				inputTokens := int(finalResponse.Usage.PromptTokens)
				outputTokens := int(finalResponse.Usage.CompletionTokens)
				s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

				// Convert to JSON and return
				responseJSON, _ := json.Marshal(finalResponse)
				var responseMap map[string]interface{}
				json.Unmarshal(responseJSON, &responseMap)
				responseMap["model"] = responseModel
				c.JSON(http.StatusOK, responseMap)
				return
			}
		}
	}

	// Extract usage from response
	inputTokens := int(response.Usage.PromptTokens)
	outputTokens := int(response.Usage.CompletionTokens)

	// Track usage
	s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

	// Convert response to JSON map for modification
	responseJSON, err := json.Marshal(response)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to marshal response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to process response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Update response model if configured
	responseMap["model"] = responseModel

	if nonstream.ShouldRoundtripResponse(c, "anthropic") {
		roundtripped, err := nonstream.RoundtripOpenAIResponseViaAnthropic(response, responseModel, provider, req.Model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to roundtrip response: " + err.Error(),
					Type:    "api_error",
				},
			})
			return
		}
		responseMap = roundtripped
		responseMap["model"] = responseModel
	}

	// Return modified response
	c.JSON(http.StatusOK, responseMap)
}

// handleInterceptedToolCalls executes intercepted tool calls locally and returns final response
func (s *Server) handleInterceptedToolCalls(provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, toolCallResponse *openai.ChatCompletion) (*openai.ChatCompletion, error) {
	logrus.Debugf("Handling %d intercepted tool calls for provider %s", len(toolCallResponse.Choices[0].Message.ToolCalls), provider.Name)

	// Build new messages list with original messages
	newMessages := make([]openai.ChatCompletionMessageParamUnion, len(originalReq.Messages))
	copy(newMessages, originalReq.Messages)

	// Add assistant message with tool calls
	newMessages = append(newMessages, toolCallResponse.Choices[0].Message.ToParam())

	// Execute each intercepted tool call
	for _, tc := range toolCallResponse.Choices[0].Message.ToolCalls {
		fn := tc.Function
		// Check if this tool should be intercepted
		if !toolinterceptor.ShouldInterceptTool(fn.Name) {
			continue
		}

		// Execute the tool using the interceptor
		result := s.toolInterceptor.ExecuteTool(provider, fn.Name, fn.Arguments)

		// Add tool result message
		var toolResultMsg openai.ChatCompletionMessageParamUnion
		if result.IsError {
			toolResultMsg = openai.ToolMessage(
				fmt.Sprintf("Error: %s", result.Error),
				tc.ID,
			)
		} else {
			toolResultMsg = openai.ToolMessage(
				result.Content,
				tc.ID,
			)
		}
		newMessages = append(newMessages, toolResultMsg)
		logrus.Debugf("Executed tool %s locally: %s", fn.Name, fn.Arguments)
	}

	// Create new request with updated messages
	followUpReq := *originalReq
	followUpReq.Messages = newMessages
	followUpReq = *toolinterceptor.StripSearchFetchToolsOpenAI(&followUpReq)

	// Forward to provider for final response (may contain more tool calls or final answer)
	wrapper := s.clientPool.GetOpenAIClient(provider, string(followUpReq.Model))
	fc := NewForwardContext(nil, provider)
	finalResponse, err := ForwardOpenAIChat(fc, wrapper, &followUpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get final response after tool execution: %w", err)
	}

	return finalResponse, nil
}

// handleOpenAIChatStreamingRequest handles streaming chat completion requests
func (s *Server) handleOpenAIChatStreamingRequest(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, shouldIntercept, shouldStripTools bool) {
	// === PRE-REQUEST INTERCEPTION: Strip tools before sending to provider ===
	req := originalReq
	if shouldIntercept {
		preparedReq, _ := s.toolInterceptor.PrepareOpenAIRequest(provider, originalReq)
		req = preparedReq
	} else if shouldStripTools {
		req = toolinterceptor.StripSearchFetchToolsOpenAI(originalReq)
	}

	wrapper := s.clientPool.GetOpenAIClient(provider, string(req.Model))
	fc := NewForwardContext(c.Request.Context(), provider)
	stream, _, err := ForwardOpenAIChatStream(fc, wrapper, req)
	if err != nil {
		// Track error with no usage
		s.trackUsageFromContext(c, 0, 0, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to create streaming request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Create handle context and handle stream
	hc := NewHandleContext(c, responseModel)
	usage, err := HandleOpenAIChatStream(hc, stream, req)

	// Track usage from stream handler
	s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, err)
}

// handleOpenAIStreamResponse processes the streaming response and sends it to the client
func (s *Server) handleOpenAIStreamResponse(c *gin.Context, stream *ssestream.Stream[openai.ChatCompletionChunk], req *openai.ChatCompletionNewParams, responseModel string, rule *typ.Rule, provider *typ.Provider) {
	// Accumulate usage from stream chunks
	var inputTokens, outputTokens int
	var hasUsage bool
	var contentBuilder strings.Builder
	var firstChunkID string // Store the first chunk ID for usage estimation

	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in streaming handler: %v", r)
			// Track panic as error with any usage we accumulated
			if hasUsage {
				s.trackUsageFromContext(c, inputTokens, outputTokens, fmt.Errorf("panic: %v", r))
			}
			// Try to send an error event if possible
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.SSEvent("", map[string]interface{}{
					"error": map[string]interface{}{
						"message": "Internal streaming error",
						"type":    "internal_error",
					},
				})
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		// Ensure stream is always closed
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing stream: %v", err)
			}
		}
	}()

	// Set SSE headers (mimicking OpenAI response headers)
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cache-Control")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// Create a flusher to ensure immediate sending of data
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Streaming not supported by this connection",
				Type:    "api_error",
				Code:    "streaming_unsupported",
			},
		})
		return
	}

	// Process the stream with context cancellation checking
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			logrus.Debug("Client disconnected, stopping OpenAI stream")
			return false
		default:
		}

		// Try to get next chunk
		if !stream.Next() {
			// Stream ended
			return false
		}

		chatChunk := stream.Current()
		obfuscationValue := generateObfuscationString() // Generate obfuscation value once per stream

		// Store the first chunk ID for usage estimation
		if firstChunkID == "" && chatChunk.ID != "" {
			firstChunkID = chatChunk.ID
		}

		// Accumulate usage from chunks (if present)
		if chatChunk.Usage.PromptTokens != 0 {
			inputTokens = int(chatChunk.Usage.PromptTokens)
			hasUsage = true
		}

		if chatChunk.Usage.CompletionTokens != 0 {
			outputTokens = int(chatChunk.Usage.CompletionTokens)
			hasUsage = true
		}

		// Check if we have choices and they're not empty
		if len(chatChunk.Choices) == 0 {
			return true
		}

		choice := chatChunk.Choices[0]

		// Accumulate content for estimation
		if choice.Delta.Content != "" {
			contentBuilder.WriteString(choice.Delta.Content)
		}

		// Build delta map - only include non-empty fields to avoid validation errors
		delta := map[string]interface{}{}
		if choice.Delta.Role != "" {
			delta["role"] = choice.Delta.Role
		}
		if choice.Delta.Content != "" {
			delta["content"] = choice.Delta.Content
		} else {
			delta["content"] = ""
		}
		if choice.Delta.Refusal != "" {
			delta["refusal"] = choice.Delta.Refusal
		} else {
			delta["refusal"] = nil
		}
		if choice.Delta.JSON.FunctionCall.Valid() {
			delta["function_call"] = choice.Delta.FunctionCall
		}
		if len(choice.Delta.ToolCalls) > 0 {
			delta["tool_calls"] = choice.Delta.ToolCalls
		}

		finishReason := &choice.FinishReason
		if finishReason != nil && *finishReason == "" {
			finishReason = nil
		}

		// Prepare the chunk in OpenAI format
		chunk := map[string]interface{}{
			"id":      chatChunk.ID,
			"object":  "chat.completion.chunk",
			"created": chatChunk.Created,
			"model":   responseModel,
			"choices": []map[string]interface{}{
				{
					"index":         choice.Index,
					"delta":         delta,
					"finish_reason": finishReason,
					"logprobs":      choice.Logprobs,
				},
			},
		}

		// Add usage if present (usually only in the last chunk)
		if chatChunk.Usage.PromptTokens != 0 || chatChunk.Usage.CompletionTokens != 0 {
			chunk["usage"] = chatChunk.Usage
		}

		// Add system fingerprint if present
		if chatChunk.SystemFingerprint != "" {
			chunk["system_fingerprint"] = chatChunk.SystemFingerprint
		}

		// Add service_tier if present
		if chatChunk.ServiceTier != "" {
			chunk["service_tier"] = chatChunk.ServiceTier
		} else {
			chunk["service_tier"] = "default"
		}

		// Add obfuscation if present in extra fields, otherwise use generated value
		if obfuscationField, ok := chatChunk.JSON.ExtraFields["obfuscation"]; ok && obfuscationField.Valid() {
			var upstreamObfuscation string
			if err := json.Unmarshal([]byte(obfuscationField.Raw()), &upstreamObfuscation); err == nil {
				chunk["obfuscation"] = upstreamObfuscation
			} else {
				chunk["obfuscation"] = obfuscationValue
			}
		} else {
			chunk["obfuscation"] = obfuscationValue
		}

		// Convert to JSON and send as SSE
		chunkJSON, err := json.Marshal(chunk)
		if err != nil {
			logrus.Errorf("Failed to marshal chunk: %v", err)
			return true // Continue on marshal error
		}

		// Send the chunk
		// MENTION: Must keep extra space
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", chunkJSON))
		flusher.Flush()
		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if IsContextCanceled(err) || errors.Is(err, context.Canceled) {
			logrus.Debug("OpenAI stream canceled by client")
			// Estimate usage if we don't have it
			if !hasUsage {
				inputTokens, _ = token.EstimateInputTokens(req)
				outputTokens = token.EstimateOutputTokens(contentBuilder.String())
			}
			s.trackUsageFromContext(c, inputTokens, outputTokens, err)
			return
		}

		logrus.Errorf("Stream error: %v", err)

		// If no usage from stream, estimate it
		if !hasUsage {
			inputTokens, _ = token.EstimateInputTokens(req)
			outputTokens = token.EstimateOutputTokens(contentBuilder.String())
		}

		// Track usage with error status
		s.trackUsageFromContext(c, inputTokens, outputTokens, err)

		// Send error event
		errorChunk := map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}

		errorJSON, marshalErr := json.Marshal(errorChunk)
		if marshalErr == nil {
			c.SSEvent("", string(errorJSON))
		} else {
			c.SSEvent("", map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Failed to marshal error",
					"type":    "internal_error",
				},
			})
		}
		flusher.Flush()
		return
	}

	// Track successful streaming completion
	// If no usage from stream, estimate it and send to client
	if !hasUsage {
		inputTokens, _ = token.EstimateInputTokens(req)
		outputTokens = token.EstimateOutputTokens(contentBuilder.String())

		// Use the first chunk ID, or generate one if not available
		chunkID := firstChunkID
		if chunkID == "" {
			chunkID = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		}

		// Send estimated usage as final chunk
		usageChunk := map[string]interface{}{
			"id":      chunkID,
			"object":  "chat.completion.chunk",
			"created": 0,
			"model":   responseModel,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": nil,
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     inputTokens,
				"completion_tokens": outputTokens,
				"total_tokens":      inputTokens + outputTokens,
			},
		}

		usageChunkJSON, err := json.Marshal(usageChunk)
		if err == nil {
			c.SSEvent("", usageChunkJSON)
			flusher.Flush()
		}
	}

	s.trackUsageFromContext(c, inputTokens, outputTokens, nil)

	// Send the final [DONE] message
	// MENTION: must keep extra space
	c.SSEvent("", " [DONE]")
	flusher.Flush()
}

// ListModelsByScenario handles the /v1/models endpoint for scenario-based routing
func (s *Server) ListModelsByScenario(c *gin.Context) {
	scenario := c.Param("scenario")

	// Convert string to RuleScenario and validate
	scenarioType := typ.RuleScenario(scenario)
	if !isValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("invalid scenario: %s", scenario),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Route to appropriate handler based on scenario
	switch scenarioType {
	case typ.ScenarioAnthropic, typ.ScenarioClaudeCode:
		s.AnthropicListModelsForScenario(c, scenarioType)
	default:
		// OpenAI is the default
		s.OpenAIListModelsForScenario(c, scenarioType)
	}
}

// handleResponsesForChatRequest handles chat completion requests by converting them to Responses API requests
// This is used for models that prefer the Responses API over the Chat Completions API
func (s *Server) handleResponsesForChatRequest(c *gin.Context, provider *typ.Provider, req *protocol.OpenAIChatCompletionRequest, responseModel, actualModel string, rule *typ.Rule, isStreaming bool) {
	// Convert chat completion request to responses request
	params := s.convertChatCompletionToResponsesParams(req, actualModel)

	if isStreaming {
		s.handleResponsesStreamingRequest(c, provider, params, responseModel, actualModel, rule)
	} else {
		s.handleResponsesNonStreamingRequest(c, provider, params, responseModel, actualModel, rule)
	}
}

// convertChatCompletionToResponsesParams converts a chat completion request to responses API params
func (s *Server) convertChatCompletionToResponsesParams(req *protocol.OpenAIChatCompletionRequest, actualModel string) responses.ResponseNewParams {
	// Build input items from chat messages
	inputItems := s.convertMessagesToResponseInputItems(req.Messages)

	params := responses.ResponseNewParams{
		Model:       actualModel,
		Input:       responses.ResponseNewParamsInputUnion{OfInputItemList: responses.ResponseInputParam(inputItems)},
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxOutputTokens: func() param.Opt[int64] {
			if req.MaxTokens.Valid() {
				return param.NewOpt(req.MaxTokens.Value)
			}
			return param.Opt[int64]{}
		}(),
	}

	// Add instructions from system message if present
	instructionsFound := false
	for _, msg := range req.Messages {
		if !param.IsOmitted(msg.OfSystem) {
			systemMsg := msg.OfSystem
			if !param.IsOmitted(systemMsg.Content.OfString) {
				params.Instructions = systemMsg.Content.OfString
				instructionsFound = true
				break
			}
		}
	}

	// If no system message (no instructions), add a default instruction
	// This is required by ChatGPT backend API for Codex OAuth providers
	if !instructionsFound {
		params.Instructions = param.NewOpt("You are a helpful AI assistant.")
	}

	return params
}

// convertMessagesToResponseInputItems converts chat messages to response input items
func (s *Server) convertMessagesToResponseInputItems(messages []openai.ChatCompletionMessageParamUnion) responses.ResponseInputParam {
	var inputItems responses.ResponseInputParam

	for _, msg := range messages {
		switch {
		case !param.IsOmitted(msg.OfUser):
			userMsg := msg.OfUser
			if !param.IsOmitted(userMsg.Content.OfString) {
				content := userMsg.Content.OfString.Value
				inputItem := responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: param.NewOpt(content),
						},
					},
				}
				inputItems = append(inputItems, inputItem)
			}

		case !param.IsOmitted(msg.OfAssistant):
			assistantMsg := msg.OfAssistant
			if !param.IsOmitted(assistantMsg.Content.OfString) {
				content := assistantMsg.Content.OfString.Value
				inputItem := responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleAssistant,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: param.NewOpt(content),
						},
					},
				}
				inputItems = append(inputItems, inputItem)
			}
		}
	}

	return inputItems
}

// isValidRuleScenario checks if the given scenario is a valid RuleScenario
func isValidRuleScenario(scenario typ.RuleScenario) bool {
	switch scenario {
	case typ.ScenarioOpenAI, typ.ScenarioAnthropic, typ.ScenarioClaudeCode, typ.ScenarioOpenCode, typ.ScenarioXcode:
		return true
	default:
		return false
	}
}
