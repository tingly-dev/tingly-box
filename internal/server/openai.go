package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"tingly-box/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// OpenAIChatCompletions handles OpenAI v1 chat completion requests
func (s *Server) OpenAIChatCompletions(c *gin.Context) {
	// Use the existing ChatCompletions logic for OpenAI compatibility
	s.ChatCompletions(c)
}

// ChatCompletions handles OpenAI-compatible chat completion requests
func (s *Server) ChatCompletions(c *gin.Context) {
	// Read the raw request body to check for stream parameter
	bodyBytes, err := c.GetRawData()
	if err != nil {
		logrus.Error("Failed to read request body")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to read request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Parse the request to check if streaming is requested
	var rawReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawReq); err != nil {
		logrus.Error("Invalid JSON in request body")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid JSON: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if streaming is requested
	isStreaming := false
	if stream, ok := rawReq["stream"].(bool); ok {
		isStreaming = stream
	}
	logrus.Infof("Stream requested: %v", isStreaming)

	// Parse request body into RequestWrapper
	var req RequestWrapper
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		logrus.Error("Invalid request body")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	for i := 0; i < len(req.Messages); i++ {
		role := req.Messages[i].GetRole()
		if role != nil {
			// Convert the entire message to JSON to see the content properly
			messageBytes, err := json.Marshal(req.Messages[i])
			if err == nil {
				logrus.Infof("message: %s", string(messageBytes))
			} else {
				logrus.Infof("role: %s", *role)
			}
		}
	}

	// Validate required fields
	if req.Model == "" {
		logrus.Error("No model id")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if len(req.Messages) == 0 {
		logrus.Error("No messages")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "At least one message is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Determine provider and model based on request
	provider, selectedService, rule, err := s.DetermineProviderAndModel(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Use the selected service's model
	actualModel := selectedService.Model
	responseModel := rule.ResponseModel
	req.Model = actualModel

	// Set provider and model information in context for statistics middleware
	c.Set("provider", provider.Name)
	c.Set("model", actualModel)

	// Check provider's API style to decide which path to take
	apiStyle := string(provider.APIStyle)
	if apiStyle == "" {
		apiStyle = "openai" // default to openai
	}

	if apiStyle == "anthropic" {
		// Convert to Anthropic SDK format
		anthropicReq := s.convertOpenAIToAnthropicRequest(&req, responseModel)

		if isStreaming {
			// Handle streaming request
			stream, err := s.forwardAnthropicStreamRequest(provider, anthropicReq)
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "Failed to create streaming request: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}
			// Handle streaming response and convert to OpenAI format
			s.handleAnthropicToOpenAIStreamResponse(c, stream, responseModel)
		} else {
			// Handle non-streaming request
			anthropicResp, err := s.forwardAnthropicRequest(provider, anthropicReq)
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "Failed to forward Anthropic request: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}
			// Convert Anthropic response back to OpenAI format
			openaiResp := s.convertAnthropicResponseToOpenAI(anthropicResp, responseModel)
			c.JSON(http.StatusOK, openaiResp)
		}
		return
	}

	// Handle streaming or non-streaming request for OpenAI-style providers
	if isStreaming {
		s.handleStreamingRequest(c, provider, &req, responseModel)
	} else {
		s.handleNonStreamingRequest(c, provider, &req, responseModel)
	}
}

// handleStreamingRequest handles streaming chat completion requests
func (s *Server) handleStreamingRequest(c *gin.Context, provider *config.Provider, req *RequestWrapper, responseModel string) {
	// Create streaming request
	stream, err := s.forwardOpenAIStreamRequest(provider, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to create streaming request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Handle the streaming response
	s.handleOpenAIStreamResponse(c, stream, responseModel)
}

// handleNonStreamingRequest handles non-streaming chat completion requests
func (s *Server) handleNonStreamingRequest(c *gin.Context, provider *config.Provider, req *RequestWrapper, responseModel string) {
	// Forward request to provider
	response, err := s.forwardOpenAIRequest(provider, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

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

	// Return modified response
	c.JSON(http.StatusOK, responseMap)
}

// ListModels handles the /v1/models endpoint (OpenAI compatible)
func (s *Server) ListModels(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: ErrorDetail{
			Message: "Model manager not available",
			Type:    "internal_error",
		},
	})
	return
}

// convertAnthropicResponseToOpenAI converts an Anthropic response to OpenAI format
func (s *Server) convertAnthropicResponseToOpenAI(anthropicResp *AnthropicMessagesResponse, responseModel string) map[string]interface{} {
	response := map[string]interface{}{
		"id":      anthropicResp.ID,
		"object":  "chat.completion",
		"created": 0, // Should be set to current timestamp
		"model":   responseModel,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "",
				},
				"finish_reason": anthropicResp.StopReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     anthropicResp.Usage.InputTokens,
			"completion_tokens": anthropicResp.Usage.OutputTokens,
			"total_tokens":      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	}

	// Extract content from Anthropic response
	if len(anthropicResp.Content) > 0 {
		content := ""
		for _, c := range anthropicResp.Content {
			if c.Type == "text" {
				content += c.Text
			}
		}
		response["choices"].([]map[string]interface{})[0]["message"].(map[string]interface{})["content"] = content
	}

	return response
}

// convertOpenAIToAnthropicRequest converts OpenAI RequestWrapper to Anthropic SDK format
func (s *Server) convertOpenAIToAnthropicRequest(req *RequestWrapper, model string) anthropic.MessageNewParams {
	// Convert messages
	messages := make([]anthropic.MessageParam, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role := msg.GetRole()
		if role == nil {
			continue
		}

		// Extract content from OpenAI message
		msgBytes, _ := json.Marshal(msg)
		var msgMap map[string]interface{}
		if err := json.Unmarshal(msgBytes, &msgMap); err == nil {
			contentStr, _ := msgMap["content"].(string)

			// Create content blocks
			var contentBlocks []anthropic.ContentBlockParamUnion
			if contentStr != "" {
				contentBlocks = append(contentBlocks, anthropic.NewTextBlock(contentStr))
			}

			if *role == "user" {
				messages = append(messages, anthropic.NewUserMessage(contentBlocks...))
			} else if *role == "assistant" {
				messages = append(messages, anthropic.NewAssistantMessage(contentBlocks...))
			}
		}
	}

	// Create Anthropic request parameters
	params := anthropic.MessageNewParams{
		Model:    anthropic.Model(model),
		Messages: messages,
	}

	// Set max_tokens if provided
	if req.MaxTokens.Value != 0 {
		params.MaxTokens = req.MaxTokens.Value
	}

	return params
}

// handleAnthropicToOpenAIStreamResponse processes Anthropic streaming events and converts them to OpenAI format
func (s *Server) handleAnthropicToOpenAIStreamResponse(c *gin.Context, stream *ssestream.Stream[anthropic.MessageStreamEventUnion], responseModel string) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Anthropic to OpenAI streaming handler: %v", r)
			// Try to send an error event if possible
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		// Ensure stream is always closed
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing Anthropic stream: %v", err)
			}
		}
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

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

	// Track streaming state
	var (
		chatID      = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		created     = time.Now().Unix()
		contentText = strings.Builder{}
		usage       *anthropic.MessageDeltaUsage
	)

	// Process the stream
	for stream.Next() {
		event := stream.Current()

		// Handle different event types
		switch event.Type {
		case "message_start":
			// Send initial chat completion chunk
			chunk := map[string]interface{}{
				"id":      chatID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   responseModel,
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{"role": "assistant"},
						"finish_reason": nil,
					},
				},
			}
			s.sendOpenAIStreamChunk(c, chunk, flusher)

		case "content_block_start":
			// Content block starting (usually text)
			if event.ContentBlock.Type == "text" {
				// Reset content builder for new block
				contentText.Reset()
			}

		case "content_block_delta":
			// Text delta - send as OpenAI chunk
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				text := event.Delta.Text
				contentText.WriteString(text)

				chunk := map[string]interface{}{
					"id":      chatID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   responseModel,
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"delta":         map[string]interface{}{"content": text},
							"finish_reason": nil,
						},
					},
				}
				s.sendOpenAIStreamChunk(c, chunk, flusher)
			}

		case "content_block_stop":
			// Content block finished - no specific action needed

		case "message_delta":
			// Message delta (includes usage info)
			if event.Usage.InputTokens != 0 || event.Usage.OutputTokens != 0 {
				usage = &event.Usage
			}

		case "message_stop":
			// Send final chunk with finish_reason and usage
			chunk := map[string]interface{}{
				"id":      chatID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   responseModel,
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": "stop",
					},
				},
			}

			// Add usage if available
			if usage != nil {
				chunk["usage"] = map[string]interface{}{
					"prompt_tokens":     usage.InputTokens,
					"completion_tokens": usage.OutputTokens,
					"total_tokens":      usage.InputTokens + usage.OutputTokens,
				}
			}

			s.sendOpenAIStreamChunk(c, chunk, flusher)
			// Send final [DONE] message
			c.Writer.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
			return
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Errorf("Anthropic stream error: %v", err)
		// Send error event
		errorChunk := map[string]interface{}{
			"error": map[string]interface{}{
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
}

// sendOpenAIStreamChunk helper function to send a chunk in OpenAI format
func (s *Server) sendOpenAIStreamChunk(c *gin.Context, chunk map[string]interface{}, flusher http.Flusher) {
	chunkJSON, err := json.Marshal(chunk)
	if err != nil {
		logrus.Errorf("Failed to marshal OpenAI stream chunk: %v", err)
		return
	}
	c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(chunkJSON))))
	flusher.Flush()
}
