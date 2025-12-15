package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"tingly-box/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
)

const (
	// DefaultMaxTokens is the default max_tokens value for Anthropic API requests
	DefaultMaxTokens = 4000
)

// AnthropicMessages handles Anthropic v1 messages API requests
func (s *Server) AnthropicMessages(c *gin.Context) {
	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
	} else {
		// Store the body back for parsing
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	// Parse the request to check if streaming is requested
	var rawReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawReq); err != nil {
		log.Printf("Invalid JSON in request body: %v", err)
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
	log.Printf("Stream requested for AnthropicMessages: %v", isStreaming)

	// Parse into MessageNewParams using SDK's JSON unmarshaling
	var req anthropic.MessageNewParams
	if err := c.ShouldBindJSON(&req); err != nil {
		// Log the invalid request for debugging
		log.Printf("Invalid JSON request received: %v\nBody: %s", err, string(bodyBytes))
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Get model from request
	model := string(req.Model)
	if model == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Determine provider and model based on request
	provider, selectedService, _, err := s.DetermineProviderAndModel(model)
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
	req.Model = anthropic.Model(actualModel)

	// Ensure max_tokens is set (Anthropic API requires this)
	if req.MaxTokens == 0 {
		req.MaxTokens = DefaultMaxTokens
	}

	// Set provider and model information in context for statistics middleware
	c.Set("provider", provider.Name)
	c.Set("model", selectedService.Model)

	// Check provider's API style to decide which path to take
	apiStyle := string(provider.APIStyle)
	if apiStyle == "" {
		apiStyle = "openai" // default to openai
	}

	if apiStyle == "anthropic" {
		// Use direct Anthropic SDK call
		if isStreaming {
			// Handle streaming request
			stream, err := s.forwardAnthropicStreamRequest(provider, req)
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
			s.handleAnthropicStreamResponse(c, stream)
		} else {
			// Handle non-streaming request
			anthropicResp, err := s.forwardAnthropicRequest(provider, req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "Failed to forward Anthropic request: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
		return
	} else {
		// Check if adaptor is enabled
		if !s.enableAdaptor {
			c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Request format adaptation is disabled. Cannot send Anthropic request to OpenAI-style provider '%s'. Use --adaptor flag to enable format conversion.", provider.Name),
					Type:    "adapter_disabled",
				},
			})
			return
		}

		// Use OpenAI conversion path (default behavior)
		if isStreaming {
			// Convert Anthropic request to OpenAI format for streaming
			openaiReq := s.convertAnthropicToOpenAI(&req)
			// Handle streaming request using OpenAI path
			s.handleStreamingRequest(c, provider, openaiReq, selectedService.Model)
		} else {
			// Handle non-streaming request
			openaiReq := s.convertAnthropicToOpenAI(&req)
			response, err := s.forwardOpenAIRequest(provider, openaiReq)
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "Failed to forward request: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}
			// Convert OpenAI response back to Anthropic format
			anthropicResp := s.convertOpenAIToAnthropic(response, model)
			c.JSON(http.StatusOK, anthropicResp)
		}
	}
}

// AnthropicModels handles Anthropic v1 models endpoint
func (s *Server) AnthropicModels(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: ErrorDetail{
			Message: "Model manager not available",
			Type:    "internal_error",
		},
	})
	return
}

// AnthropicCountTokens handles Anthropic v1 count_tokens endpoint
func (s *Server) AnthropicCountTokens(c *gin.Context) {
	// Check if beta parameter is set to true
	beta := c.Query("beta") == "true"
	if !beta {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "The count_tokens endpoint requires beta=true parameter",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
	} else {
		// Store the body back for parsing
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	// Parse the request to check if streaming is requested
	var rawReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawReq); err != nil {
		log.Printf("Invalid JSON in request body: %v", err)
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
	log.Printf("Stream requested for AnthropicMessages: %v", isStreaming)

	// Parse into MessageNewParams using SDK's JSON unmarshaling
	var req anthropic.MessageCountTokensParams
	if err := c.ShouldBindJSON(&req); err != nil {
		// Log the invalid request for debugging
		log.Printf("Invalid JSON request received: %v\nBody: %s", err, string(bodyBytes))
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Get model from request
	model := string(req.Model)
	if model == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Determine provider and model based on request
	provider, selectedService, _, err := s.DetermineProviderAndModel(model)
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
	req.Model = anthropic.Model(actualModel)

	// Set provider and model information in context for statistics middleware
	c.Set("provider", provider.Name)
	c.Set("model", selectedService.Model)

	// Check provider's API style to decide which path to take
	apiStyle := string(provider.APIStyle)
	if apiStyle == "" {
		apiStyle = "openai" // default to openai
	}

	// If the provider uses Anthropic API style, use the actual count_tokens endpoint
	if apiStyle == "anthropic" {
		// Get or create Anthropic client from pool
		client := s.clientPool.GetAnthropicClient(provider)

		// Make the request using Anthropic SDK with timeout
		ctx, cancel := context.WithTimeout(context.Background(), config.RequestTimeout)
		defer cancel()
		message, err := client.Messages.CountTokens(ctx, req)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: "Invalid request body: " + err.Error(),
					Type:    "invalid_request_error",
				},
			})
			return
		}

		c.JSON(http.StatusOK, message)
	} else {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: ErrorDetail{
				Message: "Do not support: " + err.Error(),
				Type:    "not_support",
			},
		})
	}
}

// Use official Anthropic SDK types directly
type (
	// Request types
	AnthropicMessagesRequest = anthropic.MessageNewParams
	AnthropicMessage         = anthropic.MessageParam

	// Response types
	AnthropicMessagesResponse = anthropic.Message
	AnthropicUsage            = anthropic.Usage

	// Model types - SDK doesn't provide a models list, so we define our own
	AnthropicModel struct {
		ID           string   `json:"id"`
		Object       string   `json:"object"`
		Created      int64    `json:"created"`
		DisplayName  string   `json:"display_name"`
		Type         string   `json:"type"`
		MaxTokens    int      `json:"max_tokens"`
		Capabilities []string `json:"capabilities"`
	}
	AnthropicModelsResponse struct {
		Data []AnthropicModel `json:"data"`
	}
)

// forwardAnthropicRequestRaw forwards request from raw map using Anthropic SDK
func (s *Server) forwardAnthropicRequestRaw(provider *config.Provider, rawReq map[string]interface{}, model string) (*anthropic.Message, error) {
	// Get or create Anthropic client from pool
	client := s.clientPool.GetAnthropicClient(provider)
	log.Printf("Anthropic API Token Length: %d", len(provider.Token))

	// Extract and convert messages from raw request
	messagesData, ok := rawReq["messages"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid messages format")
	}

	messages := make([]anthropic.MessageParam, 0, len(messagesData))
	for _, msgData := range messagesData {
		msg, ok := msgData.(map[string]interface{})
		if !ok {
			continue
		}

		role, ok := msg["role"].(string)
		if !ok {
			continue
		}

		// Handle content which can be string or array
		var contentBlocks []anthropic.ContentBlockParamUnion
		if contentData, exists := msg["content"]; exists {
			if contentStr, ok := contentData.(string); ok {
				// Simple string content
				contentBlocks = append(contentBlocks, anthropic.NewTextBlock(contentStr))
			} else if contentArray, ok := contentData.([]interface{}); ok {
				// Array of content blocks
				for _, blockData := range contentArray {
					if block, ok := blockData.(map[string]interface{}); ok {
						if blockType, ok := block["type"].(string); ok && blockType == "text" {
							if text, ok := block["text"].(string); ok {
								contentBlocks = append(contentBlocks, anthropic.NewTextBlock(text))
							}
						}
					}
				}
			}
		}

		if role == "user" {
			messages = append(messages, anthropic.NewUserMessage(contentBlocks...))
		} else if role == "assistant" {
			messages = append(messages, anthropic.NewAssistantMessage(contentBlocks...))
		}
	}

	// Build request parameters
	params := anthropic.MessageNewParams{
		Model:    anthropic.Model(model),
		Messages: messages,
	}

	// Set max_tokens if provided, otherwise use default
	if maxTokens, ok := rawReq["max_tokens"]; ok {
		if maxTokensFloat, ok := maxTokens.(float64); ok {
			params.MaxTokens = int64(maxTokensFloat)
		}
	} else {
		// Set default max_tokens if not provided (Anthropic API requires this)
		params.MaxTokens = DefaultMaxTokens
	}

	// Make the request using Anthropic SDK with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.RequestTimeout)
	defer cancel()
	message, err := client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// forwardAnthropicRequest forwards request using Anthropic SDK with proper types
func (s *Server) forwardAnthropicRequest(provider *config.Provider, req anthropic.MessageNewParams) (*anthropic.Message, error) {
	// Get or create Anthropic client from pool
	client := s.clientPool.GetAnthropicClient(provider)

	// Make the request using Anthropic SDK with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.RequestTimeout)
	defer cancel()
	message, err := client.Messages.New(ctx, req)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// forwardAnthropicStreamRequest forwards streaming request using Anthropic SDK
func (s *Server) forwardAnthropicStreamRequest(provider *config.Provider, req anthropic.MessageNewParams) (*ssestream.Stream[anthropic.MessageStreamEventUnion], error) {
	// Get or create Anthropic client from pool
	client := s.clientPool.GetAnthropicClient(provider)

	log.Printf("Creating Anthropic streaming request")

	// Use background context for streaming
	// The stream will manage its own lifecycle and timeout
	// We don't use a timeout here because streaming responses can take longer
	ctx := context.Background()
	stream := client.Messages.NewStreaming(ctx, req)

	return stream, nil
}

// handleAnthropicStreamResponse processes the Anthropic streaming response and sends it to the client
func (s *Server) handleAnthropicStreamResponse(c *gin.Context, stream *ssestream.Stream[anthropic.MessageStreamEventUnion]) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in Anthropic streaming handler: %v", r)
			// Try to send an error event if possible
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		// Ensure stream is always closed
		if stream != nil {
			if err := stream.Close(); err != nil {
				log.Printf("Error closing Anthropic stream: %v", err)
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
		event := stream.Current()

		// Convert the event to JSON
		eventJSON, err := json.Marshal(event)
		if err != nil {
			log.Printf("Failed to marshal Anthropic stream event: %v", err)
			continue
		}

		// Send the event as SSE
		// Anthropic streaming uses server-sent events format
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(eventJSON))))
		flusher.Flush()
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		log.Printf("Anthropic stream error: %v", err)

		// Send error event
		errorEvent := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}

		errorJSON, marshalErr := json.Marshal(errorEvent)
		if marshalErr != nil {
			log.Printf("Failed to marshal Anthropic error event: %v", marshalErr)
			c.Writer.Write([]byte("event: error\ndata: {\"error\":{\"message\":\"Failed to marshal error\",\"type\":\"internal_error\"}}\n\n"))
		} else {
			c.Writer.Write([]byte(fmt.Sprintf("event: error\ndata: %s\n\n", string(errorJSON))))
		}
		flusher.Flush()
		return
	}

	// Send a final event to indicate completion (similar to OpenAI's [DONE])
	finishEvent := map[string]interface{}{
		"type": "message_stop",
	}
	finishJSON, _ := json.Marshal(finishEvent)
	c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(finishJSON))))
	flusher.Flush()
}
