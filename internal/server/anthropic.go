package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"tingly-box/internal/loadbalance"
	"tingly-box/internal/typ"
)

// Use official Anthropic SDK types directly
type (
	// Request types
	AnthropicMessagesRequest = anthropic.MessageNewParams
	AnthropicMessage         = anthropic.MessageParam

	// Response types
	AnthropicMessagesResponse = anthropic.Message
	AnthropicUsage            = anthropic.Usage

	// Model types - based on Anthropic's official models API format
	AnthropicModel struct {
		ID          string `json:"id"`
		CreatedAt   string `json:"created_at"`
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
	}
	AnthropicModelsResponse struct {
		Data    []AnthropicModel `json:"data"`
		FirstID string           `json:"first_id"`
		HasMore bool             `json:"has_more"`
		LastID  string           `json:"last_id"`
	}
)

// AnthropicMessages handles Anthropic v1 messages API requests
// This is the entry point that delegates to the appropriate implementation (v1 or beta)
func (s *Server) AnthropicMessages(c *gin.Context) {
	scenario := c.Param("scenario")

	// Check if beta parameter is set to true
	beta := c.Query("beta") == "true"

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		logrus.Debugf("Failed to read request body: %v", err)
	} else {
		// Store the body back for parsing
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	// Parse the request to check if streaming is requested
	var rawReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawReq); err != nil {
		logrus.Debugf("Invalid JSON in request body: %v", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid JSON: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Get model from request
	model := rawReq["model"].(string)
	if model == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
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
	if scenario == "" {
		provider, selectedService, rule, err = s.DetermineProviderAndModel(model)
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
		provider, selectedService, rule, err = s.DetermineProviderAndModelWithScenario(scenarioType, model)
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

	// Delegate to the appropriate implementation based on beta parameter
	if beta {
		s.anthropicMessagesV1Beta(c, bodyBytes, rawReq, model, provider, selectedService, rule)
	} else {
		s.anthropicMessagesV1(c, bodyBytes, rawReq, model, provider, selectedService, rule)
	}
}

// AnthropicListModels handles Anthropic v1 models endpoint
func (s *Server) AnthropicListModels(c *gin.Context) {
	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Config not available",
				Type:    "internal_error",
			},
		})
		return
	}

	rules := cfg.GetRequestConfigs()

	var models []AnthropicModel
	for _, rule := range rules {
		if !rule.Active {
			continue
		}

		// Build display name with provider info
		displayName := rule.RequestModel
		services := rule.GetServices()
		if len(services) > 0 {
			providerNames := make([]string, 0, len(services))
			for i := range services {
				svc := &services[i]
				if svc.Active {
					provider, err := cfg.GetProviderByUUID(svc.Provider)
					if err == nil {
						providerNames = append(providerNames, provider.Name)
					}
				}
			}
			if len(providerNames) > 0 {
				displayName += fmt.Sprintf(" (via %v)", providerNames)
			}
		}

		models = append(models, AnthropicModel{
			ID:          rule.RequestModel,
			CreatedAt:   "2024-01-01T00:00:00Z",
			DisplayName: displayName,
			Type:        "model",
		})
	}

	firstID := ""
	lastID := ""
	if len(models) > 0 {
		firstID = models[0].ID
		lastID = models[len(models)-1].ID
	}

	c.JSON(http.StatusOK, AnthropicModelsResponse{
		Data:    models,
		FirstID: firstID,
		HasMore: false,
		LastID:  lastID,
	})
}

// AnthropicCountTokens handles Anthropic v1 count_tokens endpoint
// This is the entry point that delegates to the appropriate implementation (v1 or beta)
func (s *Server) AnthropicCountTokens(c *gin.Context) {
	// Check if beta parameter is set to true
	beta := c.Query("beta") == "true"

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		logrus.Debugf("Failed to read request body: %v", err)
	} else {
		// Store the body back for parsing
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	// Parse the request to check if streaming is requested
	var rawReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawReq); err != nil {
		logrus.Debugf("Invalid JSON in request body: %v", err)
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
	logrus.Debugf("Stream requested for AnthropicMessages: %v", isStreaming)

	// Get model from request
	model := rawReq["model"].(string)
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

	// Delegate to the appropriate implementation based on beta parameter
	if beta {
		s.anthropicCountTokensV1Beta(c, bodyBytes, rawReq, model, provider, selectedService)
	} else {
		s.anthropicCountTokensV1(c, bodyBytes, rawReq, model, provider, selectedService)
	}
}

// forwardAnthropicRequestRaw forwards request from raw map using Anthropic SDK
func (s *Server) forwardAnthropicRequestRaw(provider *typ.Provider, rawReq map[string]interface{}, model string) (*anthropic.Message, error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, model)
	logrus.Debugf("Anthropic API Token Length: %d", len(provider.Token))

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
	// and cap it at the model's maximum allowed value
	if maxTokens, ok := rawReq["max_tokens"]; ok {
		if maxTokensFloat, ok := maxTokens.(float64); ok {
			params.MaxTokens = int64(maxTokensFloat)
		}
	} else {
		// Set default max_tokens if not provided (Anthropic API requires this)
		params.MaxTokens = int64(s.config.GetDefaultMaxTokens())
	}
	// Cap max_tokens at the model's maximum to prevent API errors
	maxAllowed := s.templateManager.GetMaxTokensForModel(provider.Name, model)
	if params.MaxTokens > int64(maxAllowed) {
		params.MaxTokens = int64(maxAllowed)
	}

	// Make the request using Anthropic SDK with timeout (provider.Timeout is in seconds)
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	message, err := wrapper.MessagesNew(ctx, params)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// ForwardAnthropicRequest forwards request using Anthropic SDK with proper types
// This is a public utility function used by other handlers (e.g., openai.go)
func (s *Server) ForwardAnthropicRequest(provider *typ.Provider, req anthropic.MessageNewParams) (*anthropic.Message, error) {
	return s.forwardAnthropicRequestV1(provider, req)
}

// ForwardAnthropicStreamRequest forwards streaming request using Anthropic SDK
// This is a public utility function used by other handlers (e.g., openai.go)
func (s *Server) ForwardAnthropicStreamRequest(provider *typ.Provider, req anthropic.MessageNewParams) (*anthropicstream.Stream[anthropic.MessageStreamEventUnion], error) {
	return s.forwardAnthropicStreamRequestV1(provider, req)
}
