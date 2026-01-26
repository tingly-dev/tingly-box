package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
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

	responseModel := string(req.Model)

	// Determine provider & model
	var (
		provider        *typ.Provider
		selectedService *loadbalance.Service
		rule            *typ.Rule
	)

	if scenario == "" {
		provider, selectedService, rule, err = s.DetermineProviderAndModel(responseModel)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: err.Error(),
					Type:    "invalid_request_error",
				},
			})
			return
		}
	} else {
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
		provider, selectedService, rule, err = s.DetermineProviderAndModelWithScenario(scenarioType, responseModel)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: err.Error(),
					Type:    "invalid_request_error",
				},
			})
			return
		}
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
	apiStyle := string(provider.APIStyle)
	if apiStyle == "" || apiStyle != "openai" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Responses API is only supported by OpenAI-style providers. Provider '%s' has API style: %s", provider.Name, apiStyle),
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
		s.handleResponsesStreamingRequest(c, provider, params, responseModel, actualModel, rule)
	} else {
		s.handleResponsesNonStreamingRequest(c, provider, params, responseModel, actualModel, rule)
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
	if provider.APIBase == "https://chatgpt.com/backend-api" && response.ID != "" {
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
	if provider.APIBase == "https://chatgpt.com/backend-api" {
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
	if provider.APIBase == "https://chatgpt.com/backend-api" {
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
	logrus.Infof("provider: %s (responses streaming)", provider.Name)

	// Make the request using wrapper method with provider timeout
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	stream := wrapper.Client().Responses.NewStreaming(ctx, params)

	return stream, cancel, nil
}

// forwardChatGPTBackendRequest forwards a request to ChatGPT backend API using the correct format
// Reference: https://github.com/SamSaffron/term-llm/blob/main/internal/llm/chatgpt.go
func (s *Server) forwardChatGPTBackendRequest(provider *typ.Provider, params responses.ResponseNewParams) (*responses.Response, error) {
	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get OpenAI client")
	}

	logrus.Infof("provider: %s (ChatGPT backend API)", provider.Name)

	// Make HTTP request to ChatGPT backend API
	resp, err := s.makeChatGPTBackendRequest(wrapper, provider, params, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logrus.Errorf("[ChatGPT] API error: %s", string(respBody))
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	// Read streaming response and accumulate chunks
	// ChatGPT backend API returns SSE format when stream=true
	return s.accumulateChatGPTBackendStream(resp.Body, params)
}

// accumulateChatGPTBackendStream reads SSE stream from ChatGPT backend API and accumulates into a Response
func (s *Server) accumulateChatGPTBackendStream(reader io.Reader, params responses.ResponseNewParams) (*responses.Response, error) {
	var fullOutput strings.Builder
	var inputTokens, outputTokens int
	var responseID string
	var created int64

	logrus.Infof("[ChatGPT] Reading streaming response from ChatGPT backend API")
	scanner := bufio.NewScanner(reader)
	// Increase buffer size to handle large SSE chunks (default 64KB is too small)
	scanner.Buffer(nil, bufio.MaxScanTokenSize<<9) // 32MB buffer
	chunkCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonData := strings.TrimPrefix(line, "data: ")
		if jsonData == "[DONE]" {
			logrus.Infof("[ChatGPT] Received [DONE] signal")
			break
		}

		if chunkCount < 3 {
			logrus.Infof("[ChatGPT] SSE chunk #%d: %s", chunkCount+1, jsonData)
		}

		var chunk struct {
			Type     string `json:"type"`
			Response *struct {
				ID        string `json:"id"`
				CreatedAt int64  `json:"created_at"`
				Output    []struct {
					ID      string `json:"id"`
					Type    string `json:"type"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
					Summary []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"summary"`
				} `json:"output"`
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
			} `json:"response"`
		}

		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			logrus.Warnf("[ChatGPT] Failed to parse SSE chunk: %s, data: %s", err, jsonData)
			continue
		}

		chunkCount++

		if chunk.Response != nil {
			if chunk.Response.ID != "" {
				responseID = chunk.Response.ID
			}
			if chunk.Response.CreatedAt > 0 {
				created = chunk.Response.CreatedAt
			}

			for _, item := range chunk.Response.Output {
				if item.Type == "message" {
					for _, content := range item.Content {
						if content.Type == "output_text" {
							fullOutput.WriteString(content.Text)
							logrus.Debugf("[ChatGPT] Accumulated content length: %d", fullOutput.Len())
						}
					}
				}
			}

			if chunk.Response.Usage != nil {
				if chunk.Response.Usage.InputTokens > 0 {
					inputTokens = chunk.Response.Usage.InputTokens
				}
				if chunk.Response.Usage.OutputTokens > 0 {
					outputTokens = chunk.Response.Usage.OutputTokens
				}
			}
		}
	}

	logrus.Infof("[ChatGPT] Finished reading SSE stream: %d chunks, output length: %d, tokens: %d in, %d out", chunkCount, fullOutput.Len(), inputTokens, outputTokens)

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read streaming response: %w", err)
	}

	if responseID == "" {
		responseID = "chatgpt-" + fmt.Sprintf("%d", time.Now().Unix())
	}
	if created == 0 {
		created = time.Now().Unix()
	}

	resultMap := map[string]interface{}{
		"id":         responseID,
		"object":     "response",
		"created_at": float64(created),
		"model":      string(params.Model),
		"status":     "completed",
		"usage": map[string]interface{}{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
			"total_tokens":  inputTokens + outputTokens,
		},
	}

	if fullOutput.Len() > 0 {
		resultMap["output"] = []map[string]interface{}{
			{
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]string{
					{
						"type": "output_text",
						"text": fullOutput.String(),
					},
				},
			},
		}
	}

	resultJSON, _ := json.Marshal(resultMap)
	var result responses.Response
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, fmt.Errorf("failed to construct response: %w", err)
	}

	return &result, nil
}

// handleChatGPTBackendStreamingRequest handles streaming requests for ChatGPT backend API providers
func (s *Server) handleChatGPTBackendStreamingRequest(c *gin.Context, provider *typ.Provider, params responses.ResponseNewParams, responseModel, actualModel string, rule *typ.Rule) {
	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	if wrapper == nil {
		s.trackUsage(c, rule, provider, actualModel, responseModel, 0, 0, false, "error", "no_client")
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to get OpenAI client",
				Type:    "api_error",
			},
		})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		s.trackUsage(c, rule, provider, actualModel, responseModel, 0, 0, false, "error", "streaming_unsupported")
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Streaming not supported by this connection",
				Type:    "api_error",
			},
		})
		return
	}

	// Make HTTP request to ChatGPT backend API for streaming
	// Note: ChatGPT backend API requires stream=true and uses a different request format
	// than standard OpenAI Responses API, so we make a raw HTTP request
	resp, err := s.makeChatGPTBackendRequest(wrapper, provider, params, true)
	if err != nil {
		s.trackUsage(c, rule, provider, actualModel, responseModel, 0, 0, false, "error", "request_failed")
		logrus.Errorf("[ChatGPT] Streaming request failed: %v", err)
		errorChunk := map[string]any{
			"error": map[string]any{
				"message": "Failed to create streaming request: " + err.Error(),
				"type":    "api_error",
			},
		}
		errorJSON, _ := json.Marshal(errorChunk)
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		flusher.Flush()
		return
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logrus.Errorf("[ChatGPT] API error: %s", string(respBody))
		s.trackUsage(c, rule, provider, actualModel, responseModel, 0, 0, false, "error", "api_error")
		errorChunk := map[string]any{
			"error": map[string]any{
				"message": fmt.Sprintf("API error (%d): %s", resp.StatusCode, string(respBody)),
				"type":    "api_error",
			},
		}
		errorJSON, _ := json.Marshal(errorChunk)
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		flusher.Flush()
		return
	}

	// Create SSE stream from the HTTP response using SDK's decoder
	// This properly handles all delta events from ChatGPT backend API
	stream := ssestream.NewStream[responses.ResponseStreamEventUnion](ssestream.NewDecoder(resp), nil)
	defer func() {
		if err := stream.Close(); err != nil {
			logrus.Errorf("[ChatGPT] Error closing stream: %v", err)
		}
	}()

	// Process the SSE stream using SDK-based handler
	s.processChatGPTBackendStreamSDK(c, stream, responseModel, actualModel, provider, rule, flusher)
}

// makeChatGPTBackendRequest creates and executes an HTTP request to ChatGPT backend API
// This is a shared helper function used by both streaming and non-streaming handlers
func (s *Server) makeChatGPTBackendRequest(wrapper *client.OpenAIClient, provider *typ.Provider, params responses.ResponseNewParams, _ bool) (*http.Response, error) {
	// Convert OpenAI Responses API params to ChatGPT backend API format
	chatGPTReqBody := s.convertToChatGPTBackendFormat(params, provider)

	bodyBytes, err := json.Marshal(chatGPTReqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	logrus.Infof("[ChatGPT] Sending request to ChatGPT backend API: %s", string(bodyBytes))

	// Create HTTP request to ChatGPT backend API
	reqURL := provider.APIBase + "/responses"
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers for ChatGPT backend API
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "tingly-box")

	// Add ChatGPT-Account-ID header if available from OAuth metadata
	if provider.OAuthDetail != nil && provider.OAuthDetail.ExtraFields != nil {
		if accountID, ok := provider.OAuthDetail.ExtraFields["account_id"].(string); ok && accountID != "" {
			req.Header.Set("ChatGPT-Account-ID", accountID)
		}
	}

	// Make the request (caller is responsible for closing response body and canceling context)
	// Store cancel func in a way that caller can use it - for now we'll just return the response
	_ = cancel // TODO: Return cancel func to caller
	resp, err := wrapper.HttpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// processChatGPTBackendStream reads SSE stream from ChatGPT backend API and converts to OpenAI format
func (s *Server) processChatGPTBackendStream(c *gin.Context, reader io.Reader, responseModel, actualModel string, provider *typ.Provider, rule *typ.Rule, flusher http.Flusher) {
	var inputTokens, outputTokens int64

	scanner := bufio.NewScanner(reader)
	// Increase buffer size to handle large SSE chunks (default 64KB is too small)
	scanner.Buffer(nil, bufio.MaxScanTokenSize<<9) // 32MB buffer
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and non-data lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Extract JSON data from SSE format
		jsonData := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if jsonData == "[DONE]" {
			break
		}

		// Parse SSE chunk - ChatGPT backend API format
		var chunk struct {
			Type     string `json:"type"`
			Response *struct {
				ID        string `json:"id"`
				CreatedAt int64  `json:"created_at"`
				Output    []struct {
					ID      string `json:"id"`
					Type    string `json:"type"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"output"`
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
			} `json:"response"`
		}

		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			logrus.Warnf("[ChatGPT] Streaming: Failed to parse SSE chunk: %s, data: %s", err, jsonData)
			continue
		}

		// Update metadata from response
		if chunk.Response != nil {
			// Extract usage
			if chunk.Response.Usage != nil {
				if chunk.Response.Usage.InputTokens > 0 {
					inputTokens = int64(chunk.Response.Usage.InputTokens)
				}
				if chunk.Response.Usage.OutputTokens > 0 {
					outputTokens = int64(chunk.Response.Usage.OutputTokens)
				}
			}

			// Create OpenAI Responses API event and send to client
			event := s.convertChatGPTEventToOpenAI(chunk, responseModel)
			if event != nil {
				eventJSON, err := json.Marshal(event)
				if err != nil {
					logrus.Warnf("[ChatGPT] Streaming: Failed to marshal event: %s", err)
					continue
				}
				c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(eventJSON))))
				flusher.Flush()
			}
		}
	}

	// Check for scan errors
	if err := scanner.Err(); err != nil {
		logrus.Errorf("[ChatGPT] Streaming error: %v", err)
		s.trackUsage(c, rule, provider, actualModel, responseModel, int(inputTokens), int(outputTokens), true, "error", "stream_error")
		errorChunk := map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "stream_error",
			},
		}
		errorJSON, _ := json.Marshal(errorChunk)
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		flusher.Flush()
		return
	}

	// Track successful streaming completion
	s.trackUsage(c, rule, provider, actualModel, responseModel, int(inputTokens), int(outputTokens), true, "success", "")

	// Send the final [DONE] message
	logrus.Infof("[ChatGPT] Sending final [DONE] message to client")
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
	logrus.Infof("[ChatGPT] [DONE] message sent successfully")
}

// processChatGPTBackendStreamSDK reads SSE stream from ChatGPT backend API using the SDK's streaming decoder
// and properly handles all delta events including output_text.delta, reasoning_text.delta, etc.
// It also handles conversion of reasoning items to message items for models like gpt-5.1-codex.
func (s *Server) processChatGPTBackendStreamSDK(c *gin.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion], responseModel, actualModel string, provider *typ.Provider, rule *typ.Rule, flusher http.Flusher) {
	var inputTokens, outputTokens int64

	logrus.Infof("[ChatGPT] Reading streaming response from ChatGPT backend API (SDK mode)")
	logrus.Infof("[ChatGPT] Starting to process stream events")
	chunkCount := 0

	// Track if we've seen reasoning items and need to convert them
	hasReasoningItems := false

	for stream.Next() {
		chunkCount++
		event := stream.Current()

		// Log first few chunks for debugging
		if chunkCount <= 3 {
			logrus.Infof("[ChatGPT] SSE chunk #%d: type=%s, delta=%q", chunkCount, event.Type, event.Delta)
		}

		// Get the raw JSON and potentially transform it
		rawJSON := event.RawJSON()

		// Parse the event to check if it needs transformation
		var parsedEvent map[string]interface{}
		if err := json.Unmarshal([]byte(rawJSON), &parsedEvent); err == nil {
			eventType, _ := parsedEvent["type"].(string)

			// Handle response.output_item.added events - detect reasoning items
			if eventType == "response.output_item.added" {
				if response, ok := parsedEvent["response"].(map[string]interface{}); ok {
					if output, ok := response["output"].([]interface{}); ok && len(output) > 0 {
						// Check the most recently added item
						for _, item := range output {
							if itemMap, ok := item.(map[string]interface{}); ok {
								if itemType, _ := itemMap["type"].(string); itemType == "reasoning" {
									hasReasoningItems = true
									logrus.Infof("[ChatGPT] Detected reasoning item, will convert to message")

									// Transform the reasoning item to a message item
									itemMap["type"] = "message"
									// Also change the content type from reasoning_text to output_text
									if content, ok := itemMap["content"].([]interface{}); ok {
										for _, c := range content {
											if cMap, ok := c.(map[string]interface{}); ok {
												if cType, _ := cMap["type"].(string); cType == "reasoning_text" {
													cMap["type"] = "output_text"
												}
											}
										}
									}

									// Marshal the transformed event back to JSON
									if transformedJSON, err := json.Marshal(parsedEvent); err == nil {
										rawJSON = string(transformedJSON)
										logrus.Infof("[ChatGPT] Transformed reasoning item to message item")
									}
									break
								}
							}
						}
					}
				}
			}

			// Handle reasoning_text.delta events - convert to output_text.delta
			if eventType == "reasoning_text.delta" && hasReasoningItems {
				logrus.Infof("[ChatGPT] Converting reasoning_text.delta to output_text.delta")
				parsedEvent["type"] = "output_text.delta"

				// Marshal the transformed event back to JSON
				if transformedJSON, err := json.Marshal(parsedEvent); err == nil {
					rawJSON = string(transformedJSON)
				}
			}

			// Handle response.done event - check if we need to add a synthesized message
			if eventType == "response.done" && hasReasoningItems {
				logrus.Infof("[ChatGPT] Response complete with reasoning items")
				// The reasoning items should have been converted during the stream
			}
		}

		// Log the raw JSON for first few chunks to debug
		if chunkCount <= 3 {
			logrus.Infof("[ChatGPT] Sending to client: %s", rawJSON)
		}

		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", rawJSON)))
		flusher.Flush()

		// Track usage (safe access)
		if event.Response.Usage.InputTokens > 0 {
			inputTokens = int64(event.Response.Usage.InputTokens)
		}
		if event.Response.Usage.OutputTokens > 0 {
			outputTokens = int64(event.Response.Usage.OutputTokens)
		}
	}

	logrus.Infof("[ChatGPT] Finished processing %d chunks", chunkCount)

	logrus.Infof("[ChatGPT] Finished reading SSE stream: %d chunks, tokens: %d in, %d out", chunkCount, inputTokens, outputTokens)

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Errorf("[ChatGPT] Streaming error: %v", err)
		s.trackUsage(c, rule, provider, actualModel, responseModel, int(inputTokens), int(outputTokens), true, "error", "stream_error")
		errorChunk := map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "stream_error",
			},
		}
		errorJSON, _ := json.Marshal(errorChunk)
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		flusher.Flush()
		return
	}

	// Track successful streaming completion
	s.trackUsage(c, rule, provider, actualModel, responseModel, int(inputTokens), int(outputTokens), true, "success", "")

	// Send the final [DONE] message
	logrus.Infof("[ChatGPT] Sending final [DONE] message to client")
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
	logrus.Infof("[ChatGPT] [DONE] message sent successfully")
}

// convertChatGPTEventToOpenAI converts a ChatGPT backend API event to OpenAI Responses API format
func (s *Server) convertChatGPTEventToOpenAI(chunk struct {
	Type     string `json:"type"`
	Response *struct {
		ID        string `json:"id"`
		CreatedAt int64  `json:"created_at"`
		Output    []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	} `json:"response"`
}, responseModel string) map[string]interface{} {
	if chunk.Response == nil {
		return nil
	}

	// Accumulate content from output items
	var fullContent strings.Builder
	for _, item := range chunk.Response.Output {
		if item.Type == "message" {
			for _, content := range item.Content {
				if content.Type == "output_text" {
					fullContent.WriteString(content.Text)
				}
			}
		}
	}

	// Build the event based on type
	event := map[string]interface{}{
		"type": chunk.Type,
		"response": map[string]interface{}{
			"id":         chunk.Response.ID,
			"object":     "response",
			"created_at": float64(chunk.Response.CreatedAt),
			"model":      responseModel,
		},
	}

	// Set status based on event type
	if chunk.Type == "response.created" {
		event["response"].(map[string]interface{})["status"] = "in_progress"
	} else if chunk.Type == "response.in_progress" {
		event["response"].(map[string]interface{})["status"] = "in_progress"
		// Add output if content accumulated
		if fullContent.Len() > 0 {
			event["response"].(map[string]interface{})["output"] = []map[string]interface{}{
				{
					"type":   "message",
					"role":   "assistant",
					"status": "in_progress",
					"content": []map[string]string{
						{
							"type": "output_text",
							"text": fullContent.String(),
						},
					},
				},
			}
		}
	} else if chunk.Type == "response.done" || chunk.Type == "response.completed" {
		event["response"].(map[string]interface{})["status"] = "completed"
		// Add final output
		if fullContent.Len() > 0 {
			event["response"].(map[string]interface{})["output"] = []map[string]interface{}{
				{
					"type":   "message",
					"role":   "assistant",
					"status": "completed",
					"content": []map[string]string{
						{
							"type": "output_text",
							"text": fullContent.String(),
						},
					},
				},
			}
		}
	}

	// Add usage if available
	if chunk.Response.Usage != nil {
		event["response"].(map[string]interface{})["usage"] = map[string]interface{}{
			"input_tokens":  chunk.Response.Usage.InputTokens,
			"output_tokens": chunk.Response.Usage.OutputTokens,
			"total_tokens":  chunk.Response.Usage.TotalTokens,
		}
	}

	return event
}

// convertToChatGPTBackendFormat converts OpenAI Responses API params to ChatGPT backend API format
func (s *Server) convertToChatGPTBackendFormat(params responses.ResponseNewParams, provider *typ.Provider) map[string]interface{} {
	// Build ChatGPT backend API request body
	// ChatGPT backend API requires stream to be true
	chatGPTReqBody := map[string]interface{}{
		"model":       string(params.Model),
		"stream":      true,
		"tools":       []interface{}{},
		"tool_choice": "auto",
		"store":       false,
		"include":     []string{},
	}

	// Add instructions if present, otherwise use default
	// ChatGPT backend API requires instructions to be present
	if !param.IsOmitted(params.Instructions) {
		chatGPTReqBody["instructions"] = params.Instructions.Value
	} else {
		// Use a default instructions value
		chatGPTReqBody["instructions"] = "You are a helpful AI assistant."
	}

	// Convert input to ChatGPT backend API format
	if !param.IsOmitted(params.Input.OfInputItemList) {
		inputItems := s.convertResponseInputToChatGPTFormat(params.Input.OfInputItemList)
		if inputItems != nil {
			chatGPTReqBody["input"] = inputItems
		}
	}

	// Convert tools to ChatGPT backend API format
	if !param.IsOmitted(params.Tools) && len(params.Tools) > 0 {
		tools := s.convertResponseToolsToChatGPTFormat(params.Tools)
		if tools != nil {
			chatGPTReqBody["tools"] = tools
		}
	}

	// Convert tool_choice if present
	if !param.IsOmitted(params.ToolChoice) {
		chatGPTReqBody["tool_choice"] = s.convertResponseToolChoiceToChatGPTFormat(params.ToolChoice)
	}

	// Copy other fields if present
	// Note: Codex OAuth providers do not support max_tokens, max_completion_tokens, temperature, or top_p
	// For other providers: newer OpenAI models (gpt-4o, o1, gpt-4.1) use max_completion_tokens
	// while older models use max_tokens
	if !param.IsOmitted(params.MaxOutputTokens) {
		// Skip for Codex OAuth providers
		if !s.isCodexOAuthProvider(provider) {
			model := params.Model
			maxTokensKey := "max_tokens"
			if s.requiresMaxCompletionTokens(model) {
				maxTokensKey = "max_completion_tokens"
			}
			chatGPTReqBody[maxTokensKey] = int(params.MaxOutputTokens.Value)
		}
	}
	if !param.IsOmitted(params.Temperature) {
		// Skip for Codex OAuth providers
		if !s.isCodexOAuthProvider(provider) {
			chatGPTReqBody["temperature"] = params.Temperature.Value
		}
	}
	if !param.IsOmitted(params.TopP) {
		// Skip for Codex OAuth providers
		if !s.isCodexOAuthProvider(provider) {
			chatGPTReqBody["top_p"] = params.TopP.Value
		}
	}

	return chatGPTReqBody
}

// isCodexOAuthProvider checks if the provider is a Codex OAuth provider
// Codex OAuth providers do not support max_tokens or max_completion_tokens parameters
func (s *Server) isCodexOAuthProvider(provider *typ.Provider) bool {
	if provider == nil || provider.OAuthDetail == nil {
		return false
	}
	return provider.OAuthDetail.ProviderType == "codex"
}

// requiresMaxCompletionTokens checks if the model requires max_completion_tokens instead of max_tokens
// Newer OpenAI models (gpt-4o, o1 series, gpt-4.1) use max_completion_tokens
func (s *Server) requiresMaxCompletionTokens(model string) bool {
	// Models that require max_completion_tokens
	modelsRequiringMaxCompletionTokens := []string{
		"gpt-4o",
		"gpt-4o-",
		"gpt-4o-mini",
		"gpt-4o-mini-",
		"o1-",
		"o1-2024",
		"chatgpt-4o",
		"chatgpt-4o-",
		"chatgpt-4o-mini",
		"chatgpt-4o-mini-",
		"gpt-4.1",
		"gpt-4.1-",
	}

	for _, prefix := range modelsRequiringMaxCompletionTokens {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

// convertResponseInputToChatGPTFormat converts ResponseInputParam to ChatGPT backend API format
func (s *Server) convertResponseInputToChatGPTFormat(inputItems responses.ResponseInputParam) []interface{} {
	var result []interface{}

	for _, item := range inputItems {
		// Handle message items
		if !param.IsOmitted(item.OfMessage) {
			msg := item.OfMessage
			chatGPTItem := map[string]interface{}{
				"type": "message",
				"role": string(msg.Role),
			}

			// Determine content type based on role
			contentType := "input_text"
			if string(msg.Role) == "assistant" {
				contentType = "output_text"
			}

			// Handle content - check if it's a simple string
			if !param.IsOmitted(msg.Content.OfString) {
				// Simple string content - convert to ChatGPT format
				chatGPTItem["content"] = []map[string]string{
					{"type": contentType, "text": msg.Content.OfString.Value},
				}
			} else if !param.IsOmitted(msg.Content.OfInputItemContentList) {
				// Array content - convert each content item to ChatGPT format
				var contentItems []map[string]interface{}
				for _, contentItem := range msg.Content.OfInputItemContentList {
					if !param.IsOmitted(contentItem.OfInputText) {
						contentItems = append(contentItems, map[string]interface{}{
							"type": contentType,
							"text": contentItem.OfInputText.Text,
						})
					}
					// Handle other content types as needed (images, audio, etc.)
				}
				if len(contentItems) > 0 {
					chatGPTItem["content"] = contentItems
				}
			}

			// Only add if content was successfully set
			if _, hasContent := chatGPTItem["content"]; hasContent {
				result = append(result, chatGPTItem)
			}
		}
	}

	return result
}

// convertResponseToolsToChatGPTFormat converts Tools from Responses API format to ChatGPT backend API format
func (s *Server) convertResponseToolsToChatGPTFormat(tools []responses.ToolUnionParam) []interface{} {
	if len(tools) == 0 {
		return nil
	}

	result := make([]interface{}, 0, len(tools))

	for _, tool := range tools {
		// Handle function tools (custom tools for function calling)
		if !param.IsOmitted(tool.OfFunction) {
			fn := tool.OfFunction
			toolMap := map[string]interface{}{
				"type":       "function",
				"name":       fn.Name,
				"parameters": fn.Parameters,
			}
			if !param.IsOmitted(fn.Description) {
				toolMap["description"] = fn.Description.Value
			}
			if !param.IsOmitted(fn.Strict) {
				toolMap["strict"] = fn.Strict.Value
			}
			result = append(result, toolMap)
		}
		// Add other tool types as needed (web_search, file_search, etc.)
		// For now, we only support function tools
	}

	return result
}

// convertResponseToolChoiceToChatGPTFormat converts ToolChoice from Responses API format to ChatGPT backend API format
func (s *Server) convertResponseToolChoiceToChatGPTFormat(toolChoice responses.ResponseNewParamsToolChoiceUnion) interface{} {
	// Handle different tool_choice variants
	if !param.IsOmitted(toolChoice.OfToolChoiceMode) {
		// "auto", "none", "required" modes
		return toolChoice.OfToolChoiceMode.Value
	}
	if !param.IsOmitted(toolChoice.OfFunctionTool) {
		// Specific function tool choice
		fn := toolChoice.OfFunctionTool
		return map[string]interface{}{
			"type": "function",
			"name": fn.Name,
		}
	}
	// Default to auto
	return "auto"
}

// extractInstructions extracts system message content as instructions
func (s *Server) extractInstructions(raw map[string]interface{}) string {
	// Check for instructions field directly
	if instructions, ok := raw["instructions"].(string); ok && instructions != "" {
		return instructions
	}

	// Try to extract from system messages in input
	if inputArray, ok := raw["input"].([]interface{}); ok {
		for _, item := range inputArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "message" {
					if role, ok := itemMap["role"].(string); ok && role == "system" {
						if content, ok := itemMap["content"].(string); ok {
							return content
						}
					}
				}
			}
		}
	}

	return ""
}

// convertInputToChatGPTFormat converts input items to ChatGPT backend API format
func (s *Server) convertInputToChatGPTFormat(raw map[string]interface{}) []interface{} {
	var inputItems []interface{}

	// Get input from raw params
	inputValue, ok := raw["input"]
	if !ok {
		return nil
	}

	// Handle string input (simple text prompt)
	if inputStr, ok := inputValue.(string); ok {
		inputItems = append(inputItems, map[string]interface{}{
			"type": "message",
			"role": "user",
			"content": []map[string]string{
				{"type": "input_text", "text": inputStr},
			},
		})
		return inputItems
	}

	// Handle array input (complex messages)
	if inputArray, ok := inputValue.([]interface{}); ok {
		for _, item := range inputArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				itemType, _ := itemMap["type"].(string)

				switch itemType {
				case "message":
					// Convert message format
					role, _ := itemMap["role"].(string)
					inputItem := map[string]interface{}{
						"type": "message",
						"role": role,
					}

					// Handle content as string or array
					if contentStr, ok := itemMap["content"].(string); ok {
						contentType := "input_text"
						if role == "assistant" {
							contentType = "output_text"
						}
						inputItem["content"] = []map[string]string{
							{"type": contentType, "text": contentStr},
						}
					} else if contentArray, ok := itemMap["content"].([]interface{}); ok {
						var contentItems []map[string]interface{}
						for _, c := range contentArray {
							if cMap, ok := c.(map[string]interface{}); ok {
								cType, _ := cMap["type"].(string)
								if cType == "input_text" || cType == "output_text" {
									text, _ := cMap["text"].(string)
									contentItems = append(contentItems, map[string]interface{}{
										"type": cType,
										"text": text,
									})
								}
							}
						}
						if len(contentItems) > 0 {
							inputItem["content"] = contentItems
						}
					}

					inputItems = append(inputItems, inputItem)

				case "function_call":
					// Pass through function calls
					inputItems = append(inputItems, itemMap)

				case "function_call_output":
					// Pass through function call outputs
					inputItems = append(inputItems, itemMap)
				}
			}
		}
	}

	return inputItems
}

// convertChatGPTResponseToOpenAI converts ChatGPT backend API response to OpenAI Responses API format
func (s *Server) convertChatGPTResponseToOpenAI(respBody []byte) (*responses.Response, error) {
	// Parse ChatGPT backend API response
	var chatGPTResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Status  string `json:"status"`
		Output  []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &chatGPTResp); err != nil {
		return nil, fmt.Errorf("failed to parse ChatGPT response: %w", err)
	}

	// Check for API error
	if chatGPTResp.Error != nil {
		return nil, fmt.Errorf("API error: %s (type: %s)", chatGPTResp.Error.Message, chatGPTResp.Error.Type)
	}

	// Convert response to JSON and unmarshal into OpenAI Response type
	// This is simpler than building the complex type structure manually
	respJSON, err := json.Marshal(chatGPTResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ChatGPT response: %w", err)
	}

	var openAIResp responses.Response
	if err := json.Unmarshal(respJSON, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to OpenAI response: %w", err)
	}

	return &openAIResp, nil
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

// convertChatGPTResponseToOpenAIChatCompletion converts a ChatGPT backend API response to OpenAI chat completion format
func (s *Server) convertChatGPTResponseToOpenAIChatCompletion(c *gin.Context, response responses.Response, responseModel string, inputTokens, outputTokens int64) {
	// Extract content from the response output
	var content string
	if len(response.Output) > 0 {
		for _, item := range response.Output {
			// Check if this is a message output
			if len(item.Content) > 0 {
				for _, c := range item.Content {
					if c.Type == "output_text" && c.Text != "" {
						content += c.Text
					}
				}
			}
		}
	}

	// Construct OpenAI chat completion response
	openAIResp := map[string]interface{}{
		"id":      response.ID,
		"object":  "chat.completion",
		"created": int64(response.CreatedAt),
		"model":   responseModel,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     inputTokens,
			"completion_tokens": outputTokens,
			"total_tokens":      inputTokens + outputTokens,
		},
	}

	c.JSON(http.StatusOK, openAIResp)
}
