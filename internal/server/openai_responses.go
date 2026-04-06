package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleResponsesCreate handles POST /v1/responses
func (s *Server) HandleResponsesCreate(c *gin.Context) {
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
	var req protocol.ResponseCreateRequest
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
	inputValue := protocol.GetInputValue(req.Input)
	if inputValue == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Input is required",
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

	// Check if this is the request model name first
	rule, err = s.determineRuleWithScenario(c, scenarioType, req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Select service using routing pipeline
	provider, selectedService, err = s.routingSelector.SelectService(c, scenarioType, rule, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	actualModel := selectedService.Model
	maxAllowed := s.templateManager.GetMaxTokensForModelByProvider(provider, actualModel)

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, req.Model, req.Stream)

	// Convert request to OpenAI SDK format first so fallback conversions can reuse it.
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

	req.ResponseNewParams = params
	// req.Model is replaced with actualModel (resolved backend model) from this point on
	req.Model = actualModel
	s.ResponsesCreate(c, scenarioType, provider, rule, req, rule.RequestModel, maxAllowed)
}

func (s *Server) ResponsesCreate(c *gin.Context, scenarioType typ.RuleScenario, provider *typ.Provider, rule *typ.Rule, req protocol.ResponseCreateRequest, responseModel string, maxAllowed int) {
	actualModel := req.Model
	isStreaming := req.Stream

	// Determine target API type based on provider API style
	target := protocol.TypeOpenAIResponses
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicV1
	case protocol.APIStyleGoogle:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Responses API does not support Google-style providers yet. Provider: %s", provider.Name),
				Type:    "invalid_request_error",
				Code:    "unsupported_provider_style",
			},
		})
		return
	case protocol.APIStyleOpenAI:
		if provider.APIBase != protocol.CodexAPIBase {
			preferredEndpoint := NewAdaptiveProbe(s).GetPreferredEndpoint(provider, actualModel)
			if preferredEndpoint != "responses" {
				target = protocol.TypeOpenAIChat
			}
		}
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Unsupported provider API style: %s", provider.APIStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Execute transform chain
	reqCtx, err := s.transformOpenAIResponses(c, req, target, provider, isStreaming, nil, scenarioType, maxAllowed)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Transform failed: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Use unified dispatch
	s.dispatchChainResult(c, reqCtx, rule, provider, isStreaming, nil)
}

// buildResponsesPayloadFromChat converts a Chat completion response to Responses API format
func buildResponsesPayloadFromChat(resp *openai.ChatCompletion, responseModel, actualModel string) map[string]any {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	messageContent := ""
	if len(resp.Choices) > 0 {
		messageContent = resp.Choices[0].Message.Content
	}

	output := []map[string]any{}
	if messageContent != "" {
		output = append(output, map[string]any{
			"type":         "output_text",
			"text":         messageContent,
			"output_index": 0,
		})
	}

	return map[string]any{
		"id":     resp.ID,
		"object": "response",
		"model":  model,
		"status": "completed",
		"output": output,
		"usage": map[string]any{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
			"total_tokens":  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		},
	}
}

// buildResponsesPayloadFromAnthropic converts an Anthropic message response to Responses API format
func buildResponsesPayloadFromAnthropic(resp *anthropic.Message, responseModel, actualModel string) map[string]any {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	output := []map[string]any{}
	outputIndex := 0
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text == "" {
				continue
			}
			output = append(output, map[string]any{
				"type":         "output_text",
				"text":         block.Text,
				"output_index": outputIndex,
			})
			outputIndex++
		case "tool_use":
			argsJSON := "{}"
			if block.Input != nil {
				if raw, err := json.Marshal(block.Input); err == nil {
					argsJSON = string(raw)
				}
			}
			output = append(output, map[string]any{
				"type":         "function_call",
				"id":           block.ID,
				"name":         block.Name,
				"arguments":    argsJSON,
				"output_index": outputIndex,
			})
			outputIndex++
		}
	}

	return map[string]any{
		"id":     resp.ID,
		"object": "response",
		"model":  model,
		"status": "completed",
		"output": output,
		"usage": map[string]any{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
			"total_tokens":  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

// convertToResponsesParams converts raw JSON to OpenAI SDK params format
// This handles the model override and forwards the rest as-is
func (s *Server) convertToResponsesParams(bodyBytes []byte, actualModel string) (responses.ResponseNewParams, error) {
	// Preprocess to add type fields to input items (needed for union deserialization)
	processedData, err := protocol.AddTypeFieldToInputItems(bodyBytes)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	var raw map[string]any
	if err := json.Unmarshal(processedData, &raw); err != nil {
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
