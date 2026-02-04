package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
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
	var req ResponseCreateRequest
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
	inputValue := GetInputValue(req.Input)
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
	rule, err = s.determineRuleWithScenario(scenarioType, req.Model)
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

	// Set the rule and provider in context
	if rule != nil {
		c.Set("rule", rule)
	}

	actualModel := selectedService.Model

	// Set provider UUID and model in context
	c.Set("provider", provider.UUID)
	c.Set("model", actualModel)

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

	// Convert request to OpenAI SDK format
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

	// Handle streaming or non-streaming
	if req.Stream {
		s.handleResponsesStreamingRequest(c, provider, params, req.Model, actualModel, rule)
	} else {
		s.handleResponsesNonStreamingRequest(c, provider, params, req.Model, actualModel, rule)
	}
}

// handleResponsesNonStreamingRequest handles non-streaming Responses API requests
func (s *Server) handleResponsesNonStreamingRequest(c *gin.Context, provider *typ.Provider, params responses.ResponseNewParams, responseModel, actualModel string, rule *typ.Rule) {
	// Forward request to provider
	response, err := s.forwardResponsesRequest(provider, params)
	if err != nil {
		// Track error with no usage
		s.trackUsage(c, rule, provider, actualModel, responseModel, 0, 0, false, "error", "forward_failed")
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

	// Track usage
	s.trackUsage(c, rule, provider, actualModel, responseModel, int(inputTokens), int(outputTokens), false, "success", "")

	// Check if this is a ChatGPT backend API provider (Codex OAuth)
	// These providers return responses in a different format that needs conversion
	if provider.APIBase == protocol.ChatGPTBackendAPIBase && response.ID != "" {
		// Convert ChatGPT backend API response to OpenAI chat completion format
		// The response was accumulated from streaming chunks in forwardChatGPTBackendRequest
		s.convertChatGPTResponseToOpenAIChatCompletion(c, *response, responseModel, inputTokens, outputTokens)
		return
	}

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
func (s *Server) handleResponsesStreamingRequest(c *gin.Context, provider *typ.Provider, params responses.ResponseNewParams, responseModel, actualModel string, rule *typ.Rule) {
	// Check if this is a ChatGPT backend API provider (Codex OAuth)
	// These providers use a custom streaming handler
	if provider.APIBase == protocol.ChatGPTBackendAPIBase {
		s.handleChatGPTBackendStreamingRequest(c, provider, params, responseModel, actualModel, rule)
		return
	}

	// Create streaming request
	stream, _, err := s.forwardResponsesStreamRequest(provider, params)
	if err != nil {
		// Track error with no usage
		s.trackUsage(c, rule, provider, actualModel, responseModel, 0, 0, false, "error", "stream_creation_failed")
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to create streaming request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Handle the streaming response
	s.handleResponsesStreamResponse(c, stream, responseModel, actualModel, rule, provider)
}

// handleResponsesStreamResponse processes the streaming response and sends it to the client
func (s *Server) handleResponsesStreamResponse(c *gin.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion], responseModel, actualModel string, rule *typ.Rule, provider *typ.Provider) {
	// Accumulate usage from stream chunks
	var inputTokens, outputTokens int64
	var hasUsage bool

	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in streaming handler: %v", r)
			if hasUsage {
				s.trackUsage(c, rule, provider, actualModel, responseModel, int(inputTokens), int(outputTokens), true, "error", "panic")
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

	// Process the stream
	for stream.Next() {
		event := stream.Current()
		event.Response.Model = responseModel

		// Accumulate usage from completed events
		if event.Response.Usage.InputTokens > 0 {
			inputTokens = event.Response.Usage.InputTokens
			hasUsage = true
		}
		if event.Response.Usage.OutputTokens > 0 {
			outputTokens = event.Response.Usage.OutputTokens
		}

		c.SSEvent("", event)
		flusher.Flush()
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Errorf("Stream error: %v", err)
		if hasUsage {
			s.trackUsage(c, rule, provider, actualModel, responseModel, int(inputTokens), int(outputTokens), true, "error", "stream_error")
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
		s.trackUsage(c, rule, provider, actualModel, responseModel, int(inputTokens), int(outputTokens), true, "success", "")
	}

	// Send the final [DONE] message
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

// forwardResponsesRequest forwards a Responses API request to the provider
func (s *Server) forwardResponsesRequest(provider *typ.Provider, params responses.ResponseNewParams) (*responses.Response, error) {
	// Check if this is a ChatGPT backend API provider (Codex OAuth)
	// ChatGPT backend API requires a different request format than standard OpenAI Responses API
	if provider.APIBase == protocol.ChatGPTBackendAPIBase {
		return s.forwardChatGPTBackendRequest(provider, params)
	}

	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	logrus.Infof("provider: %s (responses)", provider.Name)

	// Make the request using wrapper method with provider timeout
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := wrapper.Client().Responses.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create response: %w", err)
	}

	return resp, nil
}

// forwardResponsesStreamRequest forwards a streaming Responses API request to the provider
func (s *Server) forwardResponsesStreamRequest(provider *typ.Provider, params responses.ResponseNewParams) (*ssestream.Stream[responses.ResponseStreamEventUnion], context.CancelFunc, error) {
	// Note: ChatGPT backend API providers are handled separately in the Anthropic beta handler

	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	logrus.Debugf("provider: %s (responses streaming)", provider.Name)

	// Make the request using wrapper method with provider timeout
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	stream := wrapper.Client().Responses.NewStreaming(ctx, params)

	return stream, cancel, nil
}

// convertToResponsesParams converts raw JSON to OpenAI SDK params format
// This handles the model override and forwards the rest as-is
func (s *Server) convertToResponsesParams(bodyBytes []byte, actualModel string) (responses.ResponseNewParams, error) {
	var raw map[string]any
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
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
