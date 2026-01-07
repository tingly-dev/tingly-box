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
	"tingly-box/pkg/adaptor"
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
func (s *Server) AnthropicMessages(c *gin.Context) {
	scenario := c.Param("scenario")

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

	// Parse into MessageNewParams using SDK's JSON unmarshaling
	var req anthropic.MessageNewParams
	if err := c.ShouldBindJSON(&req); err != nil {
		// Log the invalid request for debugging
		logrus.Debugf("Invalid JSON request received: %v\nBody: %s", err, string(bodyBytes))
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Get model from request
	proxyModel := string(req.Model)
	if proxyModel == "" {
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
		provider, selectedService, rule, err = s.DetermineProviderAndModel(proxyModel)
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
		provider, selectedService, rule, err = s.DetermineProviderAndModelWithScenario(scenarioType, proxyModel)
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

	// Set the rule and provider in context so middleware can use the same rule
	if rule != nil {
		c.Set("rule", rule)
	}
	// Use the selected service's model
	actualModel := selectedService.Model
	req.Model = anthropic.Model(actualModel)

	// Ensure max_tokens is set (Anthropic API requires this)
	// and cap it at the model's maximum allowed value
	if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil {

	} else {
		if req.MaxTokens == 0 {
			req.MaxTokens = int64(s.config.GetDefaultMaxTokens())
		}
		// Cap max_tokens at the model's maximum to prevent API errors
		maxAllowed := s.templateManager.GetMaxTokensForModel(provider.Name, actualModel)
		if req.MaxTokens > int64(maxAllowed) {
			req.MaxTokens = int64(maxAllowed)
		}
	}

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
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
			s.handleAnthropicStreamResponse(c, stream, proxyModel)
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
			// FIXME: now we use req model as resp model
			anthropicResp.Model = anthropic.Model(proxyModel)
			c.JSON(http.StatusOK, anthropicResp)
		}
		return
	} else {
		// Check if adaptor is enabled
		if !s.enableAdaptor {
			c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Request format adaptation is disabled. Cannot send Anthropic request to OpenAI-style provider '%s'. Use --adapter flag to enable format conversion.", provider.Name),
					Type:    "adapter_disabled",
				},
			})
			return
		}

		// Use OpenAI conversion path (default behavior)
		if isStreaming {
			// Convert Anthropic request to OpenAI format for streaming
			openaiReq := adaptor.ConvertAnthropicToOpenAIRequest(&req, true)

			da, _ := json.MarshalIndent(req, "", "    ")
			fmt.Printf("requst to openai: %s\n", da)

			do, _ := json.MarshalIndent(openaiReq, "", "    ")
			fmt.Printf("requst to openai: %s\n", do)

			// Create streaming request
			stream, err := s.forwardOpenAIStreamRequest(provider, openaiReq)
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
			err = adaptor.HandleOpenAIToAnthropicStreamResponse(c, stream, proxyModel)
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: err.Error(),
						Type:    "api_error",
						Code:    "streaming_unsupported",
					},
				})
			}

		} else {
			// Handle non-streaming request
			openaiReq := adaptor.ConvertAnthropicToOpenAIRequest(&req, true)
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
			anthropicResp := adaptor.ConvertOpenAIToAnthropicResponse(response, proxyModel)
			c.JSON(http.StatusOK, anthropicResp)
		}
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

	// Parse into MessageNewParams using SDK's JSON unmarshaling
	var req anthropic.MessageCountTokensParams
	if err := c.ShouldBindJSON(&req); err != nil {
		// Log the invalid request for debugging
		logrus.Debugf("Invalid JSON request received: %v\nBody: %s", err, string(bodyBytes))
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

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", selectedService.Model)

	// Check provider's API style to decide which path to take
	apiStyle := string(provider.APIStyle)
	if apiStyle == "" {
		apiStyle = "openai" // default to openai
	}

	// If the provider uses Anthropic API style, use the actual count_tokens endpoint
	if apiStyle == "anthropic" {
		// Get or create Anthropic client wrapper from pool
		wrapper := s.clientPool.GetAnthropicClient(provider, actualModel)

		// Get the underlying SDK client
		client := wrapper.Client()

		// Make the request using Anthropic SDK with timeout (provider.Timeout is in seconds)
		timeout := time.Duration(provider.Timeout) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
		count, err := countTokensWithTiktoken(string(req.Model), req.Messages, req.System.OfTextBlockArray)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: "Invalid request body: " + err.Error(),
					Type:    "invalid_request_error",
				},
			})
			return
		}
		c.JSON(http.StatusOK, anthropic.MessageTokensCount{
			InputTokens: int64(count),
		})
	}
}

// forwardAnthropicRequestRaw forwards request from raw map using Anthropic SDK
func (s *Server) forwardAnthropicRequestRaw(provider *typ.Provider, rawReq map[string]interface{}, model string) (*anthropic.Message, error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, model)
	logrus.Debugf("Anthropic API Token Length: %d", len(provider.Token))

	// Get the underlying SDK client
	client := wrapper.Client()

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
	message, err := client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// forwardAnthropicRequest forwards request using Anthropic SDK with proper types
func (s *Server) forwardAnthropicRequest(provider *typ.Provider, req anthropic.MessageNewParams) (*anthropic.Message, error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	// Get the underlying SDK client
	client := wrapper.Client()

	// Make the request using Anthropic SDK with timeout (provider.Timeout is in seconds)
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	message, err := client.Messages.New(ctx, req)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// forwardAnthropicStreamRequest forwards streaming request using Anthropic SDK
func (s *Server) forwardAnthropicStreamRequest(provider *typ.Provider, req anthropic.MessageNewParams) (*anthropicstream.Stream[anthropic.MessageStreamEventUnion], error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	logrus.Debugln("Creating Anthropic streaming request")

	// Get the underlying SDK client
	client := wrapper.Client()

	// Use background context for streaming
	// The stream will manage its own lifecycle and timeout
	// We don't use a timeout here because streaming responses can take longer
	ctx := context.Background()
	stream := client.Messages.NewStreaming(ctx, req)

	return stream, nil
}

// handleAnthropicStreamResponse processes the Anthropic streaming response and sends it to the client
func (s *Server) handleAnthropicStreamResponse(c *gin.Context, stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion], model string) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Debugf("Panic in Anthropic streaming handler: %v", r)
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
				logrus.Debugf("Error closing Anthropic stream: %v", err)
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

		event.Message.Model = anthropic.Model(model)

		// Convert the event to JSON
		eventJSON, err := json.Marshal(event)
		if err != nil {
			logrus.Debugf("Failed to marshal Anthropic stream event: %v", err)
			continue
		}

		// Send the event as SSE
		// Anthropic streaming uses server-sent events format
		// MENTION: keep the format
		// event: xxx
		// data: xxx
		// (extra \n here)
		c.Writer.Write(
			[]byte(
				fmt.Sprintf(
					"event: %s\ndata: %s\n\n",
					event.Type, string(eventJSON)),
			),
		)
		flusher.Flush()
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Debugf("Anthropic stream error: %v", err)

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
			logrus.Debugf("Failed to marshal Anthropic error event: %v", marshalErr)
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
