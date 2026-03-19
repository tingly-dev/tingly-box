package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	. "github.com/tingly-dev/tingly-box/internal/protocol/stream"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ResponsesCreate handles POST /v1/responses
func (s *Server) ResponsesCreate(c *gin.Context) {
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

	// Parse request (minimal parsing for validation)
	var req protocol.ResponseCreateRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate required fields
	if param.IsOmitted(req.Model) || string(req.Model) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if input is provided (either string or array)
	inputValue := protocol.GetInputValue(req.Input)
	if inputValue == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Input is required",
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
	provider, selectedService, err = s.DetermineProviderAndModelWithScenario(scenarioType, rule, req)
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
	maxAllowed := s.templateManager.GetMaxTokensForModelByProvider(provider, actualModel)

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, req.Model, req.Stream)

	// Convert request to OpenAI SDK format first so fallback conversions can reuse it.
	params, err := s.convertToResponsesParams(bodyBytes, actualModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to convert request: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if scenarioType == typ.ScenarioCodex && provider.APIBase != protocol.CodexAPIBase {
		preferredEndpoint := NewAdaptiveProbe(s).GetPreferredEndpoint(provider, actualModel)
		if preferredEndpoint != "responses" {
			s.handleCodexResponsesFallback(c, provider, params, req.Model, actualModel, maxAllowed, req.Stream)
			return
		}
	}

	// Check provider API style - only OpenAI-style providers support Responses API
	if provider.APIStyle != protocol.APIStyleOpenAI {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Responses API is only supported by OpenAI-style providers. Provider '%s' has API style: %s", provider.Name, provider.APIStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// For direct /responses requests, verify that the selected provider actually
	// supports the Responses API unless it's the known ChatGPT backend special case.
	if provider.APIBase != protocol.CodexAPIBase {
		preferredEndpoint := NewAdaptiveProbe(s).GetPreferredEndpoint(provider, actualModel)
		if preferredEndpoint != "responses" {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Selected provider '%s' for model '%s' does not support the Responses API; preferred endpoint is '%s'", provider.Name, actualModel, preferredEndpoint),
					Type:    "invalid_request_error",
					Code:    "responses_not_supported",
				},
			})
			return
		}
	}

	// Use Transform Chain for request transformation (Consistency + Vendor transforms)
	// Note: Base transform is not needed since the request is already in Responses API format
	// Chain: Consistency Transform → Vendor Transform
	chain := transform.NewTransformChain([]transform.Transform{
		//transform.NewConsistencyTransform(transform.TargetAPIStyleOpenAIResponses),
		transform.NewVendorTransform(provider.APIBase),
	})

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := s.config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		scenarioFlags = &scenarioConfig.Flags
	}

	transformCtx := &transform.TransformContext{
		OriginalRequest: &params,
		Request:         &params,
		ProviderURL:     provider.APIBase,
		ScenarioFlags:   scenarioFlags,
		IsStreaming:     req.Stream,
	}

	// Execute transform chain
	finalCtx, err := chain.Execute(transformCtx)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Transform chain failed: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Get final transformed request
	transformedParams := finalCtx.Request.(*responses.ResponseNewParams)

	// Handle streaming or non-streaming
	if req.Stream {
		s.handleResponsesStreamingRequest(c, provider, *transformedParams, req.Model, actualModel)
	} else {
		s.handleResponsesNonStreamingRequest(c, provider, *transformedParams, req.Model, actualModel)
	}
}

func (s *Server) handleCodexResponsesFallback(c *gin.Context, provider *typ.Provider, params responses.ResponseNewParams, responseModel, actualModel string, maxAllowed int, isStreaming bool) {
	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		chatReq := request.ConvertOpenAIResponsesToChat(params, int64(maxAllowed))
		if isStreaming {
			wrapper := s.clientPool.GetOpenAIClient(provider, chatReq.Model)
			fc := NewForwardContext(c.Request.Context(), provider)
			chatStream, cancel, err := ForwardOpenAIChatStream(fc, wrapper, chatReq)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{Message: "Failed to create streaming request: " + err.Error(), Type: "api_error"},
				})
				return
			}
			usage, err := HandleOpenAIChatToResponsesStream(c, chatStream, responseModel)
			s.trackUsageWithTokenUsage(c, usage, err)
			return
		}

		wrapper := s.clientPool.GetOpenAIClient(provider, string(chatReq.Model))
		fc := NewForwardContext(nil, provider)
		chatResp, _, err := ForwardOpenAIChat(fc, wrapper, chatReq)
		if err != nil {
			s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{Message: "Failed to forward request: " + err.Error(), Type: "api_error"},
			})
			return
		}

		inputTokens := int64(chatResp.Usage.PromptTokens)
		outputTokens := int64(chatResp.Usage.CompletionTokens)
		cacheTokens := int64(0) // Chat API doesn't provide cache information
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), int(cacheTokens)), nil)
		c.JSON(http.StatusOK, buildResponsesPayloadFromChat(chatResp, responseModel, actualModel))
		return

	case protocol.APIStyleAnthropic:
		anthropicReq := request.ConvertOpenAIResponsesToAnthropicRequest(params, int64(maxAllowed))
		if isStreaming {
			wrapper := s.clientPool.GetAnthropicClient(provider, string(anthropicReq.Model))
			fc := NewForwardContext(c.Request.Context(), provider)
			anthropicStream, cancel, err := ForwardAnthropicV1Stream(fc, wrapper, anthropicReq)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{Message: "Failed to create streaming request: " + err.Error(), Type: "api_error"},
				})
				return
			}

			hc := protocol.NewHandleContext(c, responseModel)
			usage, err := HandleAnthropicToOpenAIResponsesStream(hc, anthropicStream, responseModel)
			s.trackUsageWithTokenUsage(c, usage, err)
			return
		}

		wrapper := s.clientPool.GetAnthropicClient(provider, string(anthropicReq.Model))
		fc := NewForwardContext(nil, provider)
		anthropicResp, cancel, err := ForwardAnthropicV1(fc, wrapper, anthropicReq)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{Message: "Failed to forward request: " + err.Error(), Type: "api_error"},
			})
			return
		}

		cacheTokens := int(anthropicResp.Usage.CacheReadInputTokens)
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(anthropicResp.Usage.InputTokens), int(anthropicResp.Usage.OutputTokens), cacheTokens), nil)
		c.JSON(http.StatusOK, buildResponsesPayloadFromAnthropic(anthropicResp, responseModel, actualModel))
		return

	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Codex fallback does not support provider API style: %s", provider.APIStyle),
				Type:    "invalid_request_error",
				Code:    "codex_fallback_unsupported_provider",
			},
		})
	}
}

func buildResponsesPayloadFromChat(resp *openai.ChatCompletion, responseModel, actualModel string) map[string]any {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	messageContent := ""
	if len(resp.Choices) > 0 {
		messageContent = resp.Choices[0].Message.Content
	}

	output := []map[string]any{}
	if messageContent != "" {
		output = append(output, map[string]any{
			"type":         "output_text",
			"text":         messageContent,
			"output_index": 0,
		})
	}

	return map[string]any{
		"id":     resp.ID,
		"object": "response",
		"model":  model,
		"status": "completed",
		"output": output,
		"usage": map[string]any{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
			"total_tokens":  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		},
	}
}

func buildResponsesPayloadFromAnthropic(resp *anthropic.Message, responseModel, actualModel string) map[string]any {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	output := []map[string]any{}
	outputIndex := 0
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text == "" {
				continue
			}
			output = append(output, map[string]any{
				"type":         "output_text",
				"text":         block.Text,
				"output_index": outputIndex,
			})
			outputIndex++
		case "tool_use":
			argsJSON := "{}"
			if block.Input != nil {
				if raw, err := json.Marshal(block.Input); err == nil {
					argsJSON = string(raw)
				}
			}
			output = append(output, map[string]any{
				"type":         "function_call",
				"id":           block.ID,
				"name":         block.Name,
				"arguments":    argsJSON,
				"output_index": outputIndex,
			})
			outputIndex++
		}
	}

	return map[string]any{
		"id":     resp.ID,
		"object": "response",
		"model":  model,
		"status": "completed",
		"output": output,
		"usage": map[string]any{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
			"total_tokens":  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

// handleResponsesNonStreamingRequest handles non-streaming Responses API requests
func (s *Server) handleResponsesNonStreamingRequest(c *gin.Context, provider *typ.Provider, params responses.ResponseNewParams, responseModel, actualModel string) {
	// Forward request to provider
	var response *responses.Response
	var err error
	var cancel context.CancelFunc

	wrapper := s.clientPool.GetOpenAIClient(provider, string(params.Model))
	fc := NewForwardContext(nil, provider)
	response, cancel, err = ForwardOpenAIResponses(fc, wrapper, params)
	if cancel != nil {
		defer cancel()
	}

	if err != nil {
		// Track error with no usage
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Extract usage from response
	inputTokens := int64(response.Usage.InputTokens)
	outputTokens := int64(response.Usage.OutputTokens)
	cacheTokens := int64(response.Usage.InputTokensDetails.CachedTokens)

	// Track usage
	s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), int(cacheTokens)), nil)

	// Override model in response if needed
	if responseModel != actualModel {
		// Create a copy of the response with updated model
		responseJSON, _ := json.Marshal(response)
		var responseMap map[string]any
		if err := json.Unmarshal(responseJSON, &responseMap); err == nil {
			responseMap["model"] = responseModel
			c.JSON(http.StatusOK, responseMap)
			return
		}
	}

	// Return response as-is
	c.JSON(http.StatusOK, response)
}

// handleResponsesStreamingRequest handles streaming Responses API requests
func (s *Server) handleResponsesStreamingRequest(c *gin.Context, provider *typ.Provider, params responses.ResponseNewParams, responseModel, actualModel string) {
	// Create streaming request with request context for proper cancellation
	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	fc := NewForwardContext(c.Request.Context(), provider)
	stream, cancel, err := ForwardOpenAIResponsesStream(fc, wrapper, params)
	if cancel != nil {
		defer cancel()
	}
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

	// Handle the streaming response
	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := HandleOpenAIResponsesStream(hc, stream, responseModel)

	// Track usage from stream handler
	s.trackUsageWithTokenUsage(c, usage, err)
}

// handleResponsesStreamResponse processes the streaming response and sends it to the client
func (s *Server) handleResponsesStreamResponse(c *gin.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion], responseModel, actualModel string) {
	// Accumulate usage from stream chunks
	var inputTokens, outputTokens int64
	var hasUsage bool

	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in streaming handler: %v", r)
			if hasUsage {
				s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), 0), fmt.Errorf("panic: %v", r))
			}
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing stream: %v", err)
			}
		}
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

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
			logrus.Debug("Client disconnected, stopping Responses stream")
			return false
		default:
		}

		// Try to get next event
		if !stream.Next() {
			// Stream ended
			return false
		}

		event := stream.Current()

		// Accumulate usage from completed events
		if event.Response.Usage.InputTokens > 0 {
			inputTokens = event.Response.Usage.InputTokens
			hasUsage = true
		}
		if event.Response.Usage.OutputTokens > 0 {
			outputTokens = event.Response.Usage.OutputTokens
		}

		// Use the event type as the SSE event type (e.g., "response.created", "response.output_text.delta")
		SSEventOpenAI(c, event.Type, event, responseModel)
		flusher.Flush()
		return true
	})

	// Check for stream errors
	if err := stream.Err(); err != nil {
		// Check if it was a client cancellation
		if errors.Is(err, context.Canceled) {
			logrus.Debug("Responses stream canceled by client")
			if hasUsage {
				s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), 0), err)
			}
			return
		}

		logrus.Errorf("Stream error: %v", err)
		if hasUsage {
			s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), 0), err)
		}

		errorChunk := map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}

		errorJSON, marshalErr := json.Marshal(errorChunk)
		if marshalErr != nil {
			logrus.Errorf("Failed to marshal error chunk: %v", marshalErr)
			c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Failed to marshal error\",\"type\":\"internal_error\"}}\n\n"))
		} else {
			c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		}
		flusher.Flush()
		return
	}

	// Track successful streaming completion
	if hasUsage {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), 0), nil)
	}

	// Send the final [DONE] message
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

// SSEventOpenAI sends an SSE event with the given data
// If the data is a ResponseStreamEventUnion, it uses the raw JSON to avoid
// serializing empty fields from the union type
// If modelOverride is provided and the event contains a response object with a model field,
// it will be overridden in the JSON output
func SSEventOpenAI(c *gin.Context, t string, data any, modelOverride ...string) error {
	var jsonBytes []byte

	// For ResponseStreamEventUnion, use RawJSON() to avoid serializing all empty union fields
	if event, ok := data.(responses.ResponseStreamEventUnion); ok {
		rawJSON := event.RawJSON()
		if rawJSON != "" {
			jsonBytes = []byte(rawJSON)
			// Apply model override if provided
			if len(modelOverride) > 0 && modelOverride[0] != "" {
				var parsed map[string]any
				if err := json.Unmarshal(jsonBytes, &parsed); err == nil {
					// Check if this event has a response field with a model
					if response, ok := parsed["response"].(map[string]any); ok {
						if model, ok := response["model"].(string); ok && model != "" {
							response["model"] = modelOverride[0]
							modified, err := json.Marshal(parsed)
							if err == nil {
								jsonBytes = modified
							}
						}
					}
				}
			}
		} else {
			// Fallback to regular marshaling if RawJSON is empty
			var err error
			jsonBytes, err = json.Marshal(event)
			if err != nil {
				return err
			}
		}
	} else {
		var err error
		jsonBytes, err = json.Marshal(data)
		if err != nil {
			return err
		}
	}

	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", jsonBytes))
	return nil
}

// convertToResponsesParams converts raw JSON to OpenAI SDK params format
// This handles the model override and forwards the rest as-is
func (s *Server) convertToResponsesParams(bodyBytes []byte, actualModel string) (responses.ResponseNewParams, error) {
	// Preprocess to add type fields to input items (needed for union deserialization)
	processedData, err := protocol.AddTypeFieldToInputItems(bodyBytes)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	var raw map[string]any
	if err := json.Unmarshal(processedData, &raw); err != nil {
		return responses.ResponseNewParams{}, err
	}

	// Override the model
	raw["model"] = actualModel

	// Marshal back to JSON and unmarshal into ResponseNewParams
	modifiedJSON, err := json.Marshal(raw)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	var params responses.ResponseNewParams
	if err := json.Unmarshal(modifiedJSON, &params); err != nil {
		return responses.ResponseNewParams{}, err
	}

	return params, nil
}

// ResponsesGet handles GET /v1/responses/{id}
func (s *Server) ResponsesGet(c *gin.Context) {
	responseID := c.Param("id")

	if responseID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Response ID is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Phase 1: We don't store responses, so return not found
	// In future phases, we would retrieve from storage
	c.JSON(http.StatusNotFound, ErrorResponse{
		Error: ErrorDetail{
			Message: "Response retrieval is not supported in this version. Responses are not stored server-side.",
			Type:    "invalid_request_error",
			Code:    "response_not_found",
		},
	})
}
