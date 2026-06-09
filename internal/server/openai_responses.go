package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
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

	if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("scenario %s does not support OpenAI responses", scenario),
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

	s.applyVisionProxy(c, scenarioType, rule, req)

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

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, &req.ResponseNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

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

	// Virtual-model providers are served by the in-process vmodel handler.
	// The vmodel handler speaks OpenAI Chat format, so the Responses request is
	// converted before forwarding. req.ResponseNewParams already carries actualModel.
	//
	// NOTE: this path intentionally skips outbound dispatch helpers (pre-chain,
	// guardrails, post-recording). Usage/quota tracking for vmodel is tracked separately.
	if provider.IsVirtual() && s.virtualModelService != nil {
		chatParams := request.ConvertOpenAIResponsesToChat(&req.ResponseNewParams, int64(maxAllowed))
		chatReq := protocol.OpenAIChatCompletionRequest{
			ChatCompletionNewParams: *chatParams,
			Stream:                  req.Stream,
		}
		rewritten, err := json.Marshal(chatReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to prepare virtual-model request: " + err.Error(),
					Type:    "internal_error",
				},
			})
			return
		}
		c.Request.Body = io.NopCloser(strings.NewReader(string(rewritten)))
		c.Request.ContentLength = int64(len(rewritten))
		s.virtualModelService.GetHandler().ChatCompletions(c)
		return
	}

	s.ResponsesCreate(c, scenarioType, provider, rule, req, rule.RequestModel, maxAllowed)
}

func (s *Server) ResponsesCreate(c *gin.Context, scenarioType typ.RuleScenario, provider *typ.Provider, rule *typ.Rule, req protocol.ResponseCreateRequest, responseModel string, maxAllowed int) {
	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex
	detectAndApplyContext1MFromIncomingRequest(c, rule)

	// Resolve fusion endpoint: when the provider has an OpenAI-compatible
	// fusion URL configured, route there natively to avoid a transform.
	provider = s.resolveProviderForClient(provider, protocol.APIStyleOpenAI)

	isStreaming := req.Stream

	// Determine target API type based on provider API style
	target := protocol.TypeOpenAIResponses
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicBeta
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
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, resolveRuleFlags(c, rule), IncomingAPIResponses)
		if routeErr != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: routeErr.Error(),
					Type:    "invalid_request_error",
					Code:    "unsupported_endpoint",
				},
			})
			return
		}
		target = resolvedTarget
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Unsupported provider API style: %s", provider.APIStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Resolve flags with scenario injection, consistent with the chat/v1/beta
	// handlers (this also applies the custom User-Agent to the request context).
	scenarioConfig := s.config.GetScenarioConfig(scenarioType)
	ruleFlags := resolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeOpenAIResponses, target, provider)
	reqCtx, err := s.transformOpenAIResponses(c, req, target, provider, isStreaming, nil, scenarioType, maxAllowed, rulePreBaseTransforms(ruleFlags), ruleExtraTransforms(ruleFlags)...)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Transform failed: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}
	// Carry the response-shaping hints for downstream dispatch, matching the
	// chat handler (consumed by shouldStripUsage on the conversion sub-paths).
	reqCtx.Extra["cursor_compat"] = ruleFlags.CursorCompat
	reqCtx.Extra["skip_usage"] = ruleFlags.SkipUsage

	// Use unified dispatch with mid-request failover (non-streaming only).
	s.dispatchWithPriorityFailover(c, rule, provider, string(req.Model),
		func(p *typ.Provider, _ string) {
			s.dispatchChainResult(c, reqCtx, rule, p, isStreaming, nil)
		})
}

// convertToResponsesParams converts raw JSON to OpenAI SDK params format
// This handles the model override and forwards the rest as-is
func (s *Server) convertToResponsesParams(bodyBytes []byte, actualModel string) (responses.ResponseNewParams, error) {
	// Preprocess to add type fields to input items (needed for union deserialization)
	// and flatten output_text content blocks
	processedData, err := protocol.PreprocessInputData(bodyBytes)
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
