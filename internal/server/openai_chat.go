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
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleOpenAIChatCompletions handles OpenAI v1 chat completion requests
func (s *Server) HandleOpenAIChatCompletions(c *gin.Context) {

	scenario := c.Param("scenario")

	// Read raw body
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to read request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Parse OpenAI-style request
	var req protocol.OpenAIChatCompletionRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate
	responseModel := req.Model
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "At least one message is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Determine provider & model
	var (
		provider        *typ.Provider
		selectedService *loadbalance.Service
		rule            *typ.Rule
	)

	// Convert string to RuleScenario and validate
	scenarioType := typ.RuleScenario(scenario)
	if !isValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	//if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) {
	//	c.JSON(http.StatusBadRequest, ErrorResponse{
	//		Error: ErrorDetail{
	//			Message: fmt.Sprintf("scenario %s does not support OpenAI chat completions", scenario),
	//			Type:    "invalid_request_error",
	//		},
	//	})
	//	return
	//}

	// Check if this is the request model name first
	rule, err = s.determineRuleWithScenario(c, scenarioType, req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	s.applyVisionProxy(c, scenarioType, rule, &req.ChatCompletionNewParams)

	// Select service using routing pipeline
	provider, selectedService, err = s.routingSelector.SelectService(c, scenarioType, rule, &req.ChatCompletionNewParams)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	actualModel := selectedService.Model
	req.Model = actualModel

	// Virtual-model providers are served by the in-process vmodel handler.
	// Resolution went through the normal routing pipeline so rules/scenarios
	// still apply, but no outbound HTTP is performed. req.Model has already
	// been rewritten to actualModel above, so re-marshaling the request body
	// ensures the vmodel registry lookup uses the rule's resolved ID rather
	// than the client-facing requestModel.
	//
	// NOTE: this path intentionally skips outbound dispatch helpers (pre-chain,
	// guardrails, post-recording). Usage/quota tracking for vmodel is tracked
	// separately.
	if provider.IsVirtual() && s.virtualModelService != nil {
		rewritten, err := json.Marshal(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to prepare virtual-model request: " + err.Error(),
					Type:    "internal_error",
				},
			})
			return
		}
		c.Request.Body = io.NopCloser(strings.NewReader(string(rewritten)))
		c.Request.ContentLength = int64(len(rewritten))
		s.virtualModelService.GetHandler().ChatCompletions(c)
		return
	}

	s.OpenAIChatCompletion(c, req, responseModel, provider, scenarioType, rule)
}

func (s *Server) OpenAIChatCompletion(c *gin.Context, req protocol.OpenAIChatCompletionRequest, responseModel string, provider *typ.Provider, scenarioType typ.RuleScenario, rule *typ.Rule) {
	// Resolve fusion endpoint: when the provider has an OpenAI-compatible
	// fusion URL configured, route there natively to avoid a transform.
	provider = s.resolveProviderForClient(provider, protocol.APIStyleOpenAI)

	isStreaming := req.Stream
	actualModel := req.Model
	maxAllowed := s.templateManager.GetMaxTokensForModelByProvider(provider, actualModel)

	// Get scenario config for flags injection
	scenarioConfig := s.config.GetScenarioConfig(scenarioType)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, &req.ChatCompletionNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, responseModel, isStreaming)

	apiStyle := provider.APIStyle
	transform.AlignToolMessagesForOpenAI(&req.ChatCompletionNewParams)

	// === Cap max_tokens at model's maximum ===
	if req.MaxTokens.Valid() && req.MaxTokens.Value > int64(maxAllowed) {
		req.MaxTokens.Value = int64(maxAllowed)
	}

	// === Determine target API type ===
	apiStyle = provider.APIStyle
	target := protocol.TypeOpenAIChat
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicBeta
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		// Need flags for endpoint resolution, but we'll re-resolve with scenario after target is determined
		tempFlags := resolveRuleFlags(c, rule)
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, tempFlags, IncomingAPIChat)
		if routeErr != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: routeErr.Error(),
					Type:    "invalid_request_error",
					Code:    "unsupported_endpoint",
				},
			})
			return
		}
		target = resolvedTarget
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Unsupported API style: %s %s", provider.Name, apiStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// === Resolve flags with scenario injection ===
	ruleFlags := resolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeOpenAIChat, target)

	// === Transform via pipeline ===
	reqCtx, err := s.transformOpenAIChat(c, req, target, provider, isStreaming, nil, scenarioType, rulePreBaseTransforms(ruleFlags), ruleExtraTransforms(ruleFlags)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Transform failed: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}
	reqCtx.Extra["cursor_compat"] = ruleFlags.CursorCompat
	reqCtx.Extra["skip_usage"] = ruleFlags.SkipUsage

	// === Dispatch via transform chain ===
	reqCtx.RequestModel = actualModel
	reqCtx.ResponseModel = responseModel
	s.dispatchWithPriorityFailover(c, rule, provider, actualModel,
		func(p *typ.Provider, _ string) {
			s.dispatchChainResult(c, reqCtx, rule, p, isStreaming, nil)
		})
}

// nonstreamOpenAIChat handles non-streaming chat completion requests with MCP runtime support.
func (s *Server) nonstreamOpenAIChat(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, stripUsage bool) {
	req := originalReq

	// Forward request to provider
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
	fc := forwarding.NewForwardContext(nil, provider)
	response, _, err := forwarding.ForwardOpenAIChat(fc, wrapper, req)
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

// streamOpenAIChat handles streaming chat completion requests.
func (s *Server) streamOpenAIChat(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, disableStreamUsage bool) {
	req := originalReq

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, req)
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
	protocol.RunLoop(c, func(w io.Writer) bool {
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

func extractOpenAIMessages(messages []openai.ChatCompletionMessageParamUnion) []map[string]any {
	if len(messages) == 0 {
		return nil
	}
	b, _ := json.Marshal(messages)
	var out []map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}
