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
	"github.com/tingly-dev/tingly-box/internal/mcpruntime"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// handleNonStreamingRequest handles non-streaming chat completion requests with MCP runtime support.
func (s *Server) handleNonStreamingRequest(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, stripUsage bool) {
	req := s.injectMCPToolsIntoOpenAIRequest(c.Request.Context(), originalReq)

	// Forward request to provider
	wrapper := s.clientPool.GetOpenAIClient(provider, req.Model)
	fc := NewForwardContext(nil, provider)
	response, _, err := ForwardOpenAIChat(fc, wrapper, req)
	if err != nil {
		// Track error with no usage
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		s.trackUsageWithTokenUsage(c, usage, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// === POST-RESPONSE MCP tool execution loop ===
	if len(response.Choices) > 0 {
		choice := response.Choices[0]
		if len(choice.Message.ToolCalls) > 0 {
			if hasOnlyMCPToolCalls(choice.Message.ToolCalls) {
				finalResponse, err := s.handleMCPToolCalls(c.Request.Context(), provider, req, response)
				if err != nil {
					usage := protocol.NewTokenUsageWithCache(0, 0, 0)
					s.trackUsageWithTokenUsage(c, usage, err)
					sendErrorResponse(c, http.StatusInternalServerError, fmt.Errorf("Failed to handle tool calls: %w", err), "api_error")
					return
				}

				// Extract usage from final response
				inputTokens := int(finalResponse.Usage.PromptTokens)
				outputTokens := int(finalResponse.Usage.CompletionTokens)
				cacheTokens := int(finalResponse.Usage.PromptTokensDetails.CachedTokens)
				usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
				s.trackUsageWithTokenUsage(c, usage, nil)

				responseJSON, _ := json.Marshal(finalResponse)
				var responseMap map[string]interface{}
				json.Unmarshal(responseJSON, &responseMap)
				responseMap["model"] = responseModel
				if stripUsage {
					delete(responseMap, "usage")
				}
				c.JSON(http.StatusOK, responseMap)
				return
			}
		}
	}

	// Extract usage from response
	inputTokens := int(response.Usage.PromptTokens)
	outputTokens := int(response.Usage.CompletionTokens)
	cacheTokens := int(response.Usage.PromptTokensDetails.CachedTokens)

	// Track usage
	usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
	s.trackUsageWithTokenUsage(c, usage, nil)

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
	if stripUsage {
		delete(responseMap, "usage")
	}

	if ShouldRoundtripResponse(c, "anthropic") {
		roundtripped, err := RoundtripOpenAIResponseViaAnthropic(response, responseModel, provider, req.Model)
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
		if stripUsage {
			delete(responseMap, "usage")
		}
	}

	// Return modified response
	c.JSON(http.StatusOK, responseMap)
}

func hasOnlyMCPToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) bool {
	if len(toolCalls) == 0 {
		return false
	}
	for _, tc := range toolCalls {
		if !mcpruntime.IsMCPToolName(tc.Function.Name) {
			return false
		}
	}
	return true
}

// handleMCPToolCalls executes MCP-prefixed tool calls and loops until model returns a non-MCP tool step.
func (s *Server) handleMCPToolCalls(ctx context.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, toolCallResponse *openai.ChatCompletion) (*openai.ChatCompletion, error) {
	if s.mcpRuntime == nil {
		return nil, fmt.Errorf("mcp runtime is not initialized")
	}

	newMessages := make([]openai.ChatCompletionMessageParamUnion, len(originalReq.Messages))
	copy(newMessages, originalReq.Messages)
	resp := toolCallResponse

	const maxRounds = 6
	for round := 0; round < maxRounds; round++ {
		if len(resp.Choices) == 0 || len(resp.Choices[0].Message.ToolCalls) == 0 {
			return resp, nil
		}
		if !hasOnlyMCPToolCalls(resp.Choices[0].Message.ToolCalls) {
			return resp, nil
		}

		newMessages = append(newMessages, resp.Choices[0].Message.ToParam())
		for _, tc := range resp.Choices[0].Message.ToolCalls {
			result, err := s.mcpRuntime.CallTool(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				logrus.WithError(err).Warnf("mcp: openai tool call failed name=%s arguments=%s", tc.Function.Name, tc.Function.Arguments)
				result = fmt.Sprintf(`{"error":"%s"}`, err.Error())
			}
			newMessages = append(newMessages, openai.ToolMessage(result, tc.ID))
		}

		followUpReq := *originalReq
		followUpReq.Messages = newMessages
		followUpReq.StreamOptions = openai.ChatCompletionStreamOptionsParam{}
		followUpReq = *s.injectMCPToolsIntoOpenAIRequest(ctx, &followUpReq)

		wrapper := s.clientPool.GetOpenAIClient(provider, string(followUpReq.Model))
		fc := NewForwardContext(nil, provider)
		nextResp, _, err := ForwardOpenAIChat(fc, wrapper, &followUpReq)
		if err != nil {
			return nil, fmt.Errorf("failed to get follow-up response after mcp tool execution: %w", err)
		}
		resp = nextResp
	}
	return resp, nil
}

// handleOpenAIChatStreamingRequest handles streaming chat completion requests.
func (s *Server) handleOpenAIChatStreamingRequest(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, disableStreamUsage bool) {
	req := s.injectMCPToolsIntoOpenAIRequest(c.Request.Context(), originalReq)
	if hasDeclaredMCPTools(req) {
		reqForMCP := *req
		reqForMCP.StreamOptions = openai.ChatCompletionStreamOptionsParam{}

		wrapper := s.clientPool.GetOpenAIClient(provider, req.Model)
		fc := NewForwardContext(nil, provider)
		resp, _, err := ForwardOpenAIChat(fc, wrapper, &reqForMCP)
		if err != nil {
			usage := protocol.NewTokenUsageWithCache(0, 0, 0)
			s.trackUsageWithTokenUsage(c, usage, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to forward request: " + err.Error(),
					Type:    "api_error",
				},
			})
			return
		}

		if len(resp.Choices) > 0 && len(resp.Choices[0].Message.ToolCalls) > 0 && hasOnlyMCPToolCalls(resp.Choices[0].Message.ToolCalls) {
			resp, err = s.handleMCPToolCalls(c.Request.Context(), provider, &reqForMCP, resp)
			if err != nil {
				usage := protocol.NewTokenUsageWithCache(0, 0, 0)
				s.trackUsageWithTokenUsage(c, usage, err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "Failed to handle MCP tool calls: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}
		}

		usage := protocol.NewTokenUsageWithCache(
			int(resp.Usage.PromptTokens),
			int(resp.Usage.CompletionTokens),
			int(resp.Usage.PromptTokensDetails.CachedTokens),
		)
		s.trackUsageWithTokenUsage(c, usage, nil)
		streamSingleOpenAICompletion(c, resp, responseModel, disableStreamUsage)
		return
	}

	wrapper := s.clientPool.GetOpenAIClient(provider, req.Model)
	fc := NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := ForwardOpenAIChatStream(fc, wrapper, req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		// Track error with no usage
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		s.trackUsageWithTokenUsage(c, usage, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to create streaming request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Create handle context and handle stream
	hc := protocol.NewHandleContext(c, responseModel)
	hc.DisableStreamUsage = disableStreamUsage

	// Record TTFT when the first streaming chunk arrives
	firstTokenRecorded := false
	hc.WithOnStreamEvent(func(_ interface{}) error {
		if !firstTokenRecorded {
			SetFirstTokenTime(c)
			firstTokenRecorded = true
		}
		return nil
	})

	usage, err := stream.HandleOpenAIChatStream(hc, streamResp, req)

	// Track usage from stream handler
	s.trackUsageWithTokenUsage(c, usage, err)
}

func hasDeclaredMCPTools(req *openai.ChatCompletionNewParams) bool {
	if req == nil || len(req.Tools) == 0 {
		return false
	}
	for _, t := range req.Tools {
		fn := t.GetFunction()
		if fn == nil {
			continue
		}
		if mcpruntime.IsMCPToolName(fn.Name) {
			return true
		}
	}
	return false
}

func streamSingleOpenAICompletion(c *gin.Context, resp *openai.ChatCompletion, responseModel string, disableStreamUsage bool) {
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cache-Control")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

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

	chunk := map[string]interface{}{
		"id":      resp.ID,
		"object":  "chat.completion.chunk",
		"created": resp.Created,
		"model":   responseModel,
		"choices": []map[string]interface{}{},
	}

	for _, choice := range resp.Choices {
		delta := map[string]interface{}{
			"role":    "assistant",
			"content": choice.Message.Content,
		}
		if len(choice.Message.ToolCalls) > 0 {
			delta["tool_calls"] = choice.Message.ToolCalls
		}
		chunk["choices"] = append(chunk["choices"].([]map[string]interface{}), map[string]interface{}{
			"index":         choice.Index,
			"delta":         delta,
			"finish_reason": choice.FinishReason,
			"logprobs":      nil,
		})
	}

	if !disableStreamUsage {
		chunk["usage"] = map[string]interface{}{
			"prompt_tokens":     resp.Usage.PromptTokens,
			"completion_tokens": resp.Usage.CompletionTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		}
	}

	chunkJSON, _ := json.Marshal(chunk)
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", chunkJSON))
	flusher.Flush()
	c.Writer.WriteString("data: [DONE]\n\n")
	flusher.Flush()
}

func (s *Server) handleOpenAIStreamResponse(c *gin.Context, streamResp *ssestream.Stream[openai.ChatCompletionChunk], req *openai.ChatCompletionNewParams, responseModel string, disableStreamUsage bool) {
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
				usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
				s.trackUsageWithTokenUsage(c, usage, fmt.Errorf("panic: %v", r))
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
		if streamResp != nil {
			if err := streamResp.Close(); err != nil {
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
		if !streamResp.Next() {
			// Stream ended
			return false
		}

		chatChunk := streamResp.Current()
		obfuscationValue := stream.GenerateObfuscationString() // Generate obfuscation value once per stream

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

		// Add usage if present (usually only in the last chunk) and not disabled
		if !disableStreamUsage && (chatChunk.Usage.PromptTokens != 0 || chatChunk.Usage.CompletionTokens != 0) {
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
	if err := streamResp.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) {
			logrus.Debug("OpenAI stream canceled by client")
			// Estimate usage if we don't have it
			if !hasUsage {
				inputTokens, _ = token.EstimateInputTokens(req)
				outputTokens = token.EstimateOutputTokens(contentBuilder.String())
			}
			usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
			s.trackUsageWithTokenUsage(c, usage, err)
			return
		}

		logrus.Errorf("Stream error: %v", err)

		// If no usage from stream, estimate it
		if !hasUsage {
			inputTokens, _ = token.EstimateInputTokens(req)
			outputTokens = token.EstimateOutputTokens(contentBuilder.String())
		}

		// Track usage with error status
		usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
		s.trackUsageWithTokenUsage(c, usage, err)

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

		// Send estimated usage as final chunk (only if not disabled)
		if !disableStreamUsage {
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
	}

	usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
	s.trackUsageWithTokenUsage(c, usage, nil)

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
	return typ.CanUseScenarioInPath(scenario)
}
