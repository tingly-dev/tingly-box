package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"tingly-box/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gin-gonic/gin"
)

// AnthropicMessages handles Anthropic v1 messages API requests
func (s *Server) AnthropicMessages(c *gin.Context) {
	var req AnthropicMessagesRequest
	// Parse request body
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate required fields
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

	// Determine provider and model based on request
	provider, rule, err := s.DetermineProviderAndModel(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Update request with actual model name
	if rule != nil && rule.DefaultModel != "" {
		req.Model = rule.DefaultModel
	}

	// Check provider's API style to decide which path to take
	apiStyle := string(provider.APIStyle)
	if apiStyle == "" {
		apiStyle = "openai" // default to openai
	}

	if apiStyle == "anthropic" {
		// Use direct Anthropic SDK call
		anthropicResp, err := s.forwardAnthropicRequest(provider, &req)
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
		anthropicResp := s.convertOpenAIToAnthropic(response, req.Model)
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

// Anthropic request/response structures
type AnthropicMessagesRequest struct {
	Model         string             `json:"model"`
	Messages      []AnthropicMessage `json:"messages"`
	MaxTokens     int                `json:"max_tokens"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	TopK          *int               `json:"top_k,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	System        string             `json:"system,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnthropicMessagesResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Content      []AnthropicContent `json:"content"`
	Model        string             `json:"model"`
	StopReason   string             `json:"stop_reason"`
	StopSequence string             `json:"stop_sequence"`
	Usage        AnthropicUsage     `json:"usage"`
}

type AnthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type AnthropicModel struct {
	ID           string   `json:"id"`
	Object       string   `json:"object"`
	Created      int64    `json:"created"`
	DisplayName  string   `json:"display_name"`
	Type         string   `json:"type"`
	MaxTokens    int      `json:"max_tokens"`
	Capabilities []string `json:"capabilities"`
}

type AnthropicModelsResponse struct {
	Data []AnthropicModel `json:"data"`
}

// forwardAnthropicRequest forwards request directly using Anthropic SDK
func (s *Server) forwardAnthropicRequest(provider *config.Provider, req *AnthropicMessagesRequest) (*AnthropicMessagesResponse, error) {
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

	// Convert AnthropicMessagesRequest to Anthropic SDK parameters
	messages := make([]anthropic.MessageParam, 0, len(req.Messages))
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		} else if msg.Role == "assistant" {
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
		}
	}

	// Build request parameters - use simpler approach for now
	params := anthropic.MessageNewParams{
		MaxTokens: int64(req.MaxTokens),
		Messages:  messages,
	}

	// Set model - use Anthropic SDK model type
	params.Model = anthropic.Model(req.Model)

	// Make the request using Anthropic SDK
	message, err := client.Messages.New(context.Background(), params)
	if err != nil {
		return nil, err
	}

	// Convert Anthropic SDK response to our format
	return s.convertAnthropicSDKToResponse(message, req.Model), nil
}

// convertAnthropicSDKToResponse converts Anthropic SDK response to our format
func (s *Server) convertAnthropicSDKToResponse(message *anthropic.Message, originalModel string) *AnthropicMessagesResponse {
	// Convert content
	content := make([]AnthropicContent, 0, len(message.Content))
	for _, block := range message.Content {
		if block.Type == "text" {
			content = append(content, AnthropicContent{
				Type: "text",
				Text: string(block.Text),
			})
		}
	}

	// Determine stop reason
	stopReason := string(message.StopReason)

	response := &AnthropicMessagesResponse{
		ID:           message.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      content,
		Model:        originalModel,
		StopReason:   stopReason,
		StopSequence: "",
		Usage: AnthropicUsage{
			InputTokens:  int(message.Usage.InputTokens),
			OutputTokens: int(message.Usage.OutputTokens),
		},
	}

	return response
}
