package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleOpenAIChatCompletions handles OpenAI v1 chat completion requests
func (ph *ProtocolHandler) HandleOpenAIChatCompletions(c *gin.Context) {

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

	// Parse OpenAI-style request
	var req = &protocol.OpenAIChatCompletionRequest{}
	if err := json.Unmarshal(bodyBytes, req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate
	responseModel := req.Model
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
	var (
		provider        *typ.Provider
		selectedService *loadbalance.Service
		rule            *typ.Rule
	)

	// Convert string to RuleScenario and validate
	scenarioType := typ.RuleScenario(scenario)
	if !IsValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	//if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) {
	//	c.JSON(http.StatusBadRequest, ErrorResponse{
	//		Error: ErrorDetail{
	//			Message: fmt.Sprintf("scenario %s does not support OpenAI chat completions", scenario),
	//			Type:    "invalid_request_error",
	//		},
	//	})
	//	return
	//}

	// Check if this is the request model name first
	rule, err = ph.determineRuleWithScenario(c, scenarioType, req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	ph.applyVisionProxy(c, scenarioType, rule, &req.ChatCompletionNewParams)

	// Select service using routing pipeline
	provider, selectedService, err = ph.deps.RoutingSelector.SelectService(c, scenarioType, rule, &req.ChatCompletionNewParams)
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
	req.Model = actualModel

	ph.applyVisionProxy(c, scenarioType, rule, req.ChatCompletionNewParams)

	ph.OpenAIChatCompletion(c, req, responseModel, provider, scenarioType, rule)
}

// OpenAIChatCompletion runs the provider-independent prologue once, then drives
// the failover loop whose per-attempt callback re-runs the provider-dependent
// pipeline (align → cap → target resolution → transform → dispatch) so failover
// can rotate across heterogeneous API styles.
func (ph *ProtocolHandler) OpenAIChatCompletion(c *gin.Context, req *protocol.OpenAIChatCompletionRequest, responseModel string, provider *typ.Provider, scenarioType typ.RuleScenario, rule *typ.Rule) {
	// ── One-time prologue (provider-independent) ──

	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex.
	applyContextOneM(c, rule)

	isStreaming := req.Stream
	actualModel := req.Model
	scenarioConfig := ph.deps.Config.GetScenarioConfig(scenarioType)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, &req.ChatCompletionNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata. Provider/model are refreshed per
	// attempt by the failover loop (UpdateTrackingForFailover).
	SetTrackingContext(c, rule, provider, actualModel, responseModel, isStreaming)

	// Snapshot a pristine template only when failover is possible.
	multi := len(rule.GetActiveServices()) > 1
	var template []byte
	if multi {
		bs, err := req.MarshalJSON()
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{Message: err.Error(), Type: "api_error"},
			})
			return
		}
		template = bs
	}

	// ── Per-attempt pipeline (provider-dependent) ──
	ph.DispatchWithPriorityFailover(c, rule, provider, actualModel,
		func(p *typ.Provider, retryModel string) {
			areq := req
			if multi {
				cloned, err := CloneOpenAIChatRequest(template)
				if err != nil {
					ph.FailAttemptSetup(c, err)
					return
				}
				areq = cloned
			}
			ph.runOpenAIChatAttempt(c, areq, responseModel, p, retryModel, rule, isStreaming, scenarioType, scenarioConfig)
		})
}

// runOpenAIChatAttempt executes the provider-dependent half of an OpenAI chat
// request for one failover attempt. Setup failures route through
// failAttemptSetup so the orchestrator can advance to the next candidate.
func (ph *ProtocolHandler) runOpenAIChatAttempt(c *gin.Context, req *protocol.OpenAIChatCompletionRequest, responseModel string, provider *typ.Provider, actualModel string, rule *typ.Rule, isStreaming bool, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig) {
	// Resolve dual endpoint: when the provider has an OpenAI-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = provider.ResolveStyle(protocol.APIStyleOpenAI)
	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	req.Model = actualModel
	maxAllowed := ph.deps.TemplateManager.GetMaxTokensForModelByProvider(provider, actualModel)

	transform.AlignToolMessagesForOpenAI(req.ChatCompletionNewParams)

	// === Cap max_tokens at model's maximum ===
	if req.MaxTokens.Valid() && req.MaxTokens.Value > int64(maxAllowed) {
		req.MaxTokens.Value = int64(maxAllowed)
	}

	// === Determine target API type ===
	apiStyle := provider.APIStyle
	target := protocol.TypeOpenAIChat
	switch apiStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicBeta
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		// Need flags for endpoint resolution, but we'll re-resolve with scenario after target is determined
		tempFlags := ResolveRuleFlags(c, rule)
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, tempFlags, IncomingAPIChat)
		if routeErr != nil {
			ph.FailAttemptSetup(c, routeErr)
			return
		}
		target = resolvedTarget
	default:
		ph.FailAttemptSetup(c, fmt.Errorf("Unsupported API style: %s %s", provider.Name, apiStyle))
		return
	}

	// === Resolve flags with scenario injection ===
	// (resolveRuleFlagsWithScenario also applies the custom User-Agent to the
	// request context, so no separate call is needed here.)
	ruleFlags := ResolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeOpenAIChat, target, provider)

	// === Transform via pipeline ===
	reqCtx, err := ph.TransformOpenAIChat(c, req, target, provider, isStreaming, nil, scenarioType, RulePreBaseTransforms(ruleFlags), RulePreVendorTransforms(ruleFlags))
	if err != nil {
		ph.FailAttemptSetup(c, fmt.Errorf("Transform failed: %w", err))
		return
	}
	defer reqCtx.Release()

	reqCtx.Extra["cursor_compat"] = ruleFlags.CursorCompat
	reqCtx.Extra["skip_usage"] = ruleFlags.SkipUsage

	// === Dispatch via transform chain ===
	reqCtx.RequestModel = actualModel
	reqCtx.ResponseModel = responseModel
	ph.DispatchChainResult(c, reqCtx, rule, provider, isStreaming, nil)
}
