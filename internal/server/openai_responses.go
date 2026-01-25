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
	// Check if this is a ChatGPT backend API provider (Codex OAuth)
	// ChatGPT backend API requires a different request format than standard OpenAI Responses API
	if provider.APIBase == "https://chatgpt.com/backend-api" {
		// ChatGPT backend API streaming is not yet supported - fall back to non-streaming
		// For now, return an error indicating streaming is not supported
		return nil, nil, fmt.Errorf("streaming is not yet supported for ChatGPT backend API providers")
	}

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
	// Get HTTP client from the OpenAI client wrapper
	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get OpenAI client")
	}

	logrus.Infof("provider: %s (ChatGPT backend API)", provider.Name)

	// Convert OpenAI Responses API params to ChatGPT backend API format
	// Build the request body in the format expected by ChatGPT backend API
	chatGPTReqBody := s.convertToChatGPTBackendFormat(params)

	bodyBytes, err := json.Marshal(chatGPTReqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Log the request body for debugging
	logrus.Infof("[ChatGPT] Sending request to ChatGPT backend API: %s", string(bodyBytes))

	// Create HTTP request to ChatGPT backend API
	// The URL rewriting transport will convert this to /backend-api/codex/responses
	reqURL := provider.APIBase + "/responses"
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
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

	// Make the request
	resp, err := wrapper.HttpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
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
	var fullOutput strings.Builder
	var inputTokens, outputTokens int
	var responseID string
	var created int64

	logrus.Infof("[ChatGPT] Reading streaming response from ChatGPT backend API")
	scanner := bufio.NewScanner(resp.Body)
	chunkCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		logrus.Debugf("[ChatGPT] SSE line: %s", line)

		// Skip empty lines and non-data lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Extract JSON data from SSE format
		jsonData := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if jsonData == "[DONE]" {
			logrus.Infof("[ChatGPT] Received [DONE] signal")
			break
		}

		// Log first few chunks for debugging
		if chunkCount < 3 {
			logrus.Infof("[ChatGPT] SSE chunk #%d: %s", chunkCount+1, jsonData)
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
			Item *struct {
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
			} `json:"item"`
		}

		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			logrus.Warnf("[ChatGPT] Failed to parse SSE chunk: %s, data: %s", err, jsonData)
			continue // Skip invalid JSON chunks
		}

		chunkCount++

		// Extract metadata from response
		if chunk.Response != nil {
			if chunk.Response.ID != "" {
				responseID = chunk.Response.ID
			}
			if chunk.Response.CreatedAt > 0 {
				created = chunk.Response.CreatedAt
			}

			// Extract response content from output
			for _, item := range chunk.Response.Output {
				// Handle message items
				if item.Type == "message" {
					for _, content := range item.Content {
						if content.Type == "output_text" {
							fullOutput.WriteString(content.Text)
							logrus.Debugf("[ChatGPT] Accumulated content length: %d", fullOutput.Len())
						}
					}
				}
				// Handle reasoning items (summary)
				if item.Type == "reasoning" {
					for _, summary := range item.Summary {
						if summary.Type == "text" {
							logrus.Debugf("[ChatGPT] Reasoning summary: %s", summary.Text)
						}
					}
				}
			}

			// Extract usage from the response
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

	// Generate default ID if none was provided
	if responseID == "" {
		responseID = "chatgpt-" + fmt.Sprintf("%d", time.Now().Unix())
	}
	// Generate default created timestamp if none was provided
	if created == 0 {
		created = time.Now().Unix()
	}

	// Build OpenAI Responses API format response from accumulated data
	// Since we received a streaming response, we need to construct a non-streaming Response
	// We'll use a map instead of the Response struct to avoid serialization issues
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

	// Build the output array from accumulated text
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

	// Marshal the map to JSON and unmarshal into Response struct
	resultJSON, _ := json.Marshal(resultMap)
	var result responses.Response
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		// If unmarshal fails, return a basic error response
		return nil, fmt.Errorf("failed to construct response: %w", err)
	}

	return &result, nil
}

// convertToChatGPTBackendFormat converts OpenAI Responses API params to ChatGPT backend API format
func (s *Server) convertToChatGPTBackendFormat(params responses.ResponseNewParams) map[string]interface{} {
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

	// Add instructions if present
	if !param.IsOmitted(params.Instructions) {
		chatGPTReqBody["instructions"] = params.Instructions.Value
	}

	// Convert input to ChatGPT backend API format
	if !param.IsOmitted(params.Input.OfInputItemList) {
		inputItems := s.convertResponseInputToChatGPTFormat(params.Input.OfInputItemList)
		if inputItems != nil {
			chatGPTReqBody["input"] = inputItems
		}
	}

	// Copy other fields if present
	if !param.IsOmitted(params.MaxOutputTokens) {
		chatGPTReqBody["max_output_tokens"] = int(params.MaxOutputTokens.Value)
	}
	if !param.IsOmitted(params.Temperature) {
		chatGPTReqBody["temperature"] = params.Temperature.Value
	}
	if !param.IsOmitted(params.TopP) {
		chatGPTReqBody["top_p"] = params.TopP.Value
	}

	return chatGPTReqBody
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

			// Handle content - check if it's a simple string
			if !param.IsOmitted(msg.Content.OfString) {
				// Simple string content - convert to ChatGPT format
				contentType := "input_text"
				if string(msg.Role) == "assistant" {
					contentType = "output_text"
				}
				chatGPTItem["content"] = []map[string]string{
					{"type": contentType, "text": msg.Content.OfString.Value},
				}
			}

			result = append(result, chatGPTItem)
		}
	}

	return result
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
