package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"tingly-box/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gin-gonic/gin"
)

const (
	// DefaultMaxTokens is the default max_tokens value for Anthropic API requests
	DefaultMaxTokens = 60000
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
		return
	} else {
		// Use OpenAI conversion path (default behavior)
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
	var apiBase = provider.APIBase
	if strings.HasSuffix(apiBase, "/v1") {
		apiBase = apiBase[:len(apiBase)-3]
	}
	log.Printf("Anthropic API Base: %s, Token Length: %d", apiBase, len(provider.Token))

	// Create Anthropic client
	client := anthropic.NewClient(
		option.WithAPIKey(provider.Token),
		option.WithBaseURL(apiBase),
	)

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
	var apiBase = provider.APIBase
	if strings.HasSuffix(apiBase, "/v1") {
		apiBase = apiBase[:len(apiBase)-3]
	}

	// Create Anthropic client
	client := anthropic.NewClient(
		option.WithAPIKey(provider.Token),
		option.WithBaseURL(apiBase),
	)

	// Make the request using Anthropic SDK with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.RequestTimeout)
	defer cancel()
	message, err := client.Messages.New(ctx, req)
	if err != nil {
		return nil, err
	}

	return message, nil
}
