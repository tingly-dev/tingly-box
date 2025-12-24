package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"tingly-box/internal/config"
	"tingly-box/pkg/adaptor"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"
)

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

// OpenAIChatCompletions handles OpenAI v1 chat completion requests
func (s *Server) OpenAIChatCompletions(c *gin.Context) {
	// Use the existing ChatCompletions logic for OpenAI compatibility
	s.ChatCompletions(c)
}

// ChatCompletions handles OpenAI-compatible chat completion requests
func (s *Server) ChatCompletions(c *gin.Context) {
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

	// Inspect stream flag
	var rawReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawReq); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid JSON: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	isStreaming := false
	if v, ok := rawReq["stream"].(bool); ok {
		isStreaming = v
	}

	// Parse OpenAI-style request
	var req openai.ChatCompletionNewParams
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

	// Set the rule and provider in context so middleware can use the same rule
	if rule != nil {
		c.Set("rule", rule)
	}

	actualModel := selectedService.Model
	responseModel := rule.ResponseModel
	req.Model = actualModel

	c.Set("provider", provider.Name)
	c.Set("model", actualModel)

	apiStyle := string(provider.APIStyle)
	if apiStyle == "" {
		apiStyle = "openai"
	}

	if apiStyle == "anthropic" {
		// Check if adaptor is enabled
		if !s.enableAdaptor {
			c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Request format adaptation is disabled. Cannot send OpenAI request to Anthropic-style provider '%s'. Use --adaptor flag to enable format conversion.", provider.Name),
					Type:    "adapter_disabled",
				},
			})
			return
		}

		anthropicReq := adaptor.ConvertOpenAIToAnthropicRequest(&req, int64(s.config.GetDefaultMaxTokens()))

		// 🔥 REQUIRED: forward tool_choice
		if req.ToolChoice.OfAuto.Value != "" || req.ToolChoice.OfAllowedTools != nil || req.ToolChoice.OfFunctionToolChoice != nil || req.ToolChoice.OfCustomToolChoice != nil {
			anthropicReq.ToolChoice = adaptor.ConvertOpenAIToolChoice(&req.ToolChoice)
		}

		if isStreaming {
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

			s.handleAnthropicToOpenAIStreamResponse(c, stream, responseModel)
			return
		}

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

		openaiResp := adaptor.ConvertAnthropicResponseToOpenAI(anthropicResp, responseModel)
		c.JSON(http.StatusOK, openaiResp)
		return
	}

	if isStreaming {
		s.handleStreamingRequest(c, provider, &req, responseModel)
	} else {
		s.handleNonStreamingRequest(c, provider, &req, responseModel)
	}
}

// handleNonStreamingRequest handles non-streaming chat completion requests
func (s *Server) handleNonStreamingRequest(c *gin.Context, provider *config.Provider, req *openai.ChatCompletionNewParams, responseModel string) {
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

// forwardOpenAIRequest forwards the request to the selected provider using OpenAI library
func (s *Server) forwardOpenAIRequest(provider *config.Provider, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	// Get or create OpenAI client from pool
	client := s.clientPool.GetOpenAIClient(provider)
	logrus.Infof("provider: %s", provider.Name)

	// Since  openai.ChatCompletionNewParams is a type alias to openai.ChatCompletionNewParams,
	// we can directly use it as the request parameters
	chatReq := *req

	// Make the request using OpenAI library
	chatCompletion, err := client.Chat.Completions.New(context.Background(), chatReq)
	if err != nil {
		logrus.Error(err)
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	return chatCompletion, nil
}

// forwardOpenAIStreamRequest forwards the streaming request to the selected provider using OpenAI library
func (s *Server) forwardOpenAIStreamRequest(provider *config.Provider, req *openai.ChatCompletionNewParams) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	// Get or create OpenAI client from pool
	client := s.clientPool.GetOpenAIClient(provider)
	logrus.Infof("provider: %s (streaming)", provider.Name)

	// Since  openai.ChatCompletionNewParams is a type alias to openai.ChatCompletionNewParams,
	// we can directly use it as the request parameters
	chatReq := *req

	// Make the streaming request using OpenAI library
	stream := client.Chat.Completions.NewStreaming(context.Background(), chatReq)

	return stream, nil
}

// handleStreamingRequest handles streaming chat completion requests
func (s *Server) handleStreamingRequest(c *gin.Context, provider *config.Provider, req *openai.ChatCompletionNewParams, responseModel string) {
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

// handleOpenAIStreamResponse processes the streaming response and sends it to the client
func (s *Server) handleOpenAIStreamResponse(c *gin.Context, stream *ssestream.Stream[openai.ChatCompletionChunk], responseModel string) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in streaming handler: %v", r)
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

	// Process the stream
	for stream.Next() {
		chatChunk := stream.Current()

		// Check if we have choices and they're not empty
		if len(chatChunk.Choices) == 0 {
			continue
		}

		choice := chatChunk.Choices[0]

		// Build delta map - include all fields, JSON marshaling will handle empty values
		delta := map[string]interface{}{
			"role":          choice.Delta.Role,
			"content":       choice.Delta.Content,
			"refusal":       choice.Delta.Refusal,
			"function_call": choice.Delta.FunctionCall,
			"tool_calls":    choice.Delta.ToolCalls,
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
					"finish_reason": choice.FinishReason,
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

		// Convert to JSON and send as SSE
		chunkJSON, err := json.Marshal(chunk)
		if err != nil {
			logrus.Errorf("Failed to marshal chunk: %v", err)
			continue
		}

		// Send the chunk
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(chunkJSON))))
		flusher.Flush()
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Errorf("Stream error: %v", err)

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
			c.Writer.Write([]byte(fmt.Sprintf("data: {\"error\":{\"message\":\"Failed to marshal error\",\"type\":\"internal_error\"}}\n\n")))
		} else {
			c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		}
		flusher.Flush()
		return
	}

	// Send the final [DONE] message
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}
