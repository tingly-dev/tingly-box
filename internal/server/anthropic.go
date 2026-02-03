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
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type (
	// AnthropicModel Model types - based on Anthropic's official models API format
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
	scenarioType := typ.RuleScenario(scenario)

	// Start scenario-level recording (client -> tingly-box traffic) only if enabled
	var recorder *ScenarioRecorder
	if s.ApplyRecording(scenarioType) {
		recorder = s.RecordScenarioRequest(c, scenario)
		if recorder != nil {
			// Store recorder in context for use in handlers
			c.Set("scenario_recorder", recorder)
			// Note: RecordResponse will be called by handler after stream completes
		}
	}

	// Check if beta parameter is set to true
	beta := c.Query("beta") == "true"
	logrus.Debugf("scenario: %s beta: %v", scenario, beta)

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		logrus.Debugf("Failed to read request body: %v", err)
		// Record error if recording is enabled
		if recorder != nil {
			recorder.RecordError(err)
		}
	} else {
		// Store the body back for parsing
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	var betaMessages protocol.AnthropicBetaMessagesRequest
	var messages protocol.AnthropicMessagesRequest
	var model string
	var reqParams interface{} // For smart routing context extraction
	if beta {
		if err := json.Unmarshal(bodyBytes, &betaMessages); err != nil {
			logrus.Debugf("Failed to unmarshal request body: %v", err)
			// Record error if recording is enabled
			if recorder != nil {
				recorder.RecordError(err)
			}
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: "Message error",
					Type:    "invalid_request_error",
				},
			})
			return
		}
		model = string(betaMessages.Model)
		reqParams = &betaMessages.BetaMessageNewParams
	} else {
		if err := json.Unmarshal(bodyBytes, &messages); err != nil {
			logrus.Debugf("Failed to unmarshal request body: %v", err)
			// Record error if recording is enabled
			if recorder != nil {
				recorder.RecordError(err)
			}
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: "Message error",
					Type:    "invalid_request_error",
				},
			})
			return
		}
		model = string(messages.Model)
		reqParams = &messages.MessageNewParams
	}

	if model == "" {
		// Record error if recording is enabled
		if recorder != nil {
			recorder.RecordError(nil)
		}
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

	// Validate scenario
	if !isValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}
	provider, selectedService, rule, err = s.DetermineProviderAndModelWithScenario(scenarioType, model, reqParams)
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
		// Apply compact transformation only if the compact feature is enabled for this scenario
		if s.ApplySmartCompact(scenarioType) {
			tf := smart_compact.NewCompactTransformer(2)
			tf.HandleV1Beta(&betaMessages.BetaMessageNewParams)
			logrus.Infoln("smart compact triggered")
		}
		s.anthropicMessagesV1Beta(c, betaMessages, model, provider, selectedService, rule)
	} else {
		// Apply compact transformation only if the compact feature is enabled for this scenario
		if s.ApplySmartCompact(scenarioType) {
			tf := smart_compact.NewCompactTransformer(2)
			tf.HandleV1(&messages.MessageNewParams)
			logrus.Infoln("smart compact triggered")
		}
		s.anthropicMessagesV1(c, messages, model, provider, selectedService, rule)
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
				svc := services[i]
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
