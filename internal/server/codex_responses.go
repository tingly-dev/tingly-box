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
	streamhandler "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// forwardChatGPTBackendRequest forwards a request to ChatGPT backend API using the correct format
// Reference: https://github.com/SamSaffron/term-llm/blob/main/internal/llm/chatgpt.go
func (s *Server) forwardChatGPTBackendRequest(provider *typ.Provider, params responses.ResponseNewParams) (*responses.Response, error) {
	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get OpenAI client")
	}

	logrus.Infof("provider: %s (ChatGPT backend API)", provider.Name)

	// Make HTTP request to ChatGPT backend API
	resp, cancel, err := s.makeChatGPTBackendRequest(wrapper, provider, params, true)
	if err != nil {
		return nil, err
	}
	defer cancel()
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
							logrus.Debugf("[ChatGPT] Accumulated content length: %d, text: %s", fullOutput.Len(), content.Text)
						} else if content.Type == "refusal" {
							logrus.Warnf("[ChatGPT] Refusal content detected: %s", content.Text)
							fullOutput.WriteString(content.Text)
						}
					}
				} else {
					logrus.Debugf("[ChatGPT] Skipping output item type: %s, id: %s", item.Type, item.ID)
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
	// Get scenario recorder and set up stream recorder
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
	}

	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	if wrapper == nil {
		s.trackUsageFromContext(c, 0, 0, "error", "no_client")
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
		s.trackUsageFromContext(c, 0, 0, "error", "streaming_unsupported")
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
	resp, cancel, err := s.makeChatGPTBackendRequest(wrapper, provider, params, true)
	if err != nil {
		s.trackUsageFromContext(c, 0, 0, "error", "request_failed")
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
	defer cancel()
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logrus.Errorf("[ChatGPT] API error: %s", string(respBody))
		s.trackUsageFromContext(c, 0, 0, "error", "api_error")
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
	sseStream := ssestream.NewStream[responses.ResponseStreamEventUnion](ssestream.NewDecoder(resp), nil)
	defer func() {
		if err := sseStream.Close(); err != nil {
			logrus.Errorf("[ChatGPT] Error closing stream: %v", err)
		}
	}()

	// Check if the original request was v1 or beta format
	// The v1 handler sets this context flag when routing through Responses API
	originalFormat := "beta"
	if fmt, exists := c.Get("original_request_format"); exists {
		if formatStr, ok := fmt.(string); ok {
			originalFormat = formatStr
		}
	}

	// Process the SSE stream using the proper handler based on original request format
	var streamErr error
	if originalFormat == "v1" {
		// Original request was v1 format, send response in v1 format
		streamErr = streamhandler.HandleResponsesToAnthropicV1StreamResponse(c, sseStream, responseModel)
	} else {
		// Original request was beta format, send response in beta format
		streamErr = streamhandler.HandleResponsesToAnthropicV1BetaStreamResponse(c, sseStream, responseModel)
	}

	if streamErr != nil {
		s.trackUsageFromContext(c, 0, 0, "error", "stream_error")
		logrus.Errorf("[ChatGPT] Stream handler error: %v", streamErr)
		if streamRec != nil {
			streamRec.RecordError(streamErr)
		}
		return
	}

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(responseModel, 0, 0) // Usage is tracked internally
		streamRec.RecordResponse(provider, actualModel)
	}
}

// makeChatGPTBackendRequest creates and executes an HTTP request to ChatGPT backend API
// This is a shared helper function used by both streaming and non-streaming handlers
// It returns the HTTP response and a cancel function that should be called when done
func (s *Server) makeChatGPTBackendRequest(wrapper *client.OpenAIClient, provider *typ.Provider, params responses.ResponseNewParams, _ bool) (*http.Response, context.CancelFunc, error) {
	// Convert OpenAI Responses API params to ChatGPT backend API format
	chatGPTReqBody := s.convertToChatGPTBackendFormat(params, provider)

	bodyBytes, err := json.Marshal(chatGPTReqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	logrus.Infof("[ChatGPT] Sending request to ChatGPT backend API: %s", string(bodyBytes))

	// Create HTTP request to ChatGPT backend API
	// Use /codex/responses path directly to avoid the rewrite rule in the transport
	reqURL := provider.APIBase + "/codex/responses"
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
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
	// Use wrapper.HttpClient() which has cookies and proper headers for Cloudflare
	// We use /codex/responses path which won't trigger the rewrite rule
	resp, err := wrapper.HttpClient().Do(req)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, cancel, nil
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
			continue
		}

		// Handle function call items (tool invocations)
		if !param.IsOmitted(item.OfFunctionCall) {
			if chatGPTItem := s.convertFunctionCallToChatGPTFormat(item.OfFunctionCall); chatGPTItem != nil {
				result = append(result, chatGPTItem)
			}
			continue
		}

		// Handle function call output items (tool results)
		if !param.IsOmitted(item.OfFunctionCallOutput) {
			if chatGPTItem := s.convertFunctionCallOutputToChatGPTFormat(item.OfFunctionCallOutput); chatGPTItem != nil {
				result = append(result, chatGPTItem)
			}
		}
	}

	return result
}

// convertFunctionCallOutputToChatGPTFormat converts function_call_output items to ChatGPT backend format
func (s *Server) convertFunctionCallOutputToChatGPTFormat(output *responses.ResponseInputItemFunctionCallOutputParam) map[string]interface{} {
	if output == nil {
		return nil
	}

	data, err := json.Marshal(output)
	if err != nil {
		logrus.Debugf("Failed to marshal function call output: %v", err)
		return nil
	}

	var chatGPTItem map[string]interface{}
	if err := json.Unmarshal(data, &chatGPTItem); err != nil {
		logrus.Debugf("Failed to unmarshal function call output: %v", err)
		return nil
	}

	return chatGPTItem
}

// convertFunctionCallToChatGPTFormat converts function_call items to ChatGPT backend format
func (s *Server) convertFunctionCallToChatGPTFormat(call *responses.ResponseFunctionToolCallParam) map[string]interface{} {
	if call == nil {
		return nil
	}

	data, err := json.Marshal(call)
	if err != nil {
		logrus.Debugf("Failed to marshal function call: %v", err)
		return nil
	}

	var chatGPTItem map[string]interface{}
	if err := json.Unmarshal(data, &chatGPTItem); err != nil {
		logrus.Debugf("Failed to unmarshal function call: %v", err)
		return nil
	}

	return chatGPTItem
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
