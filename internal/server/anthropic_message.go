package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleAnthropicMessages handles Anthropic v1 messages API requests
// This is the entry point that delegates to the appropriate implementation (v1 or beta)
func (ph *ProtocolHandler) HandleAnthropicMessages(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	// Check if beta parameter is set to true
	beta := c.Query("beta") == "true"
	logrus.Debugf("scenario: %s beta: %v", scenario, beta)

	// Validate scenario
	if !IsValidRuleScenario(scenarioType) {
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	//if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportAnthropic) {
	//	c.JSON(http.StatusBadRequest, ErrorResponse{
	//		Error: ErrorDetail{
	//			Message: fmt.Sprintf("scenario %s does not support Anthropic messages", scenario),
	//			Type:    "invalid_request_error",
	//		},
	//	})
	//	return
	//}

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
			},
		})
		return
	}

	// Determine provider & requestModel
	var (
		provider        *typ.Provider
		selectedService *loadbalance.Service
		rule            *typ.Rule
	)
	var requestModel string
	var reqParams interface{} // For smart routing context extraction

	var betaMessages = &protocol.AnthropicBetaMessagesRequest{}
	var messages = &protocol.AnthropicMessagesRequest{}
	if beta {
		if err := json.Unmarshal(bodyBytes, betaMessages); err != nil {
			logrus.WithError(err).Errorf("Anthropic beta decode error")
			c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Message decode error: %s", err.Error()),
					Type:    "invalid_request_error",
				},
			})
			return
		}
		requestModel = string(betaMessages.Model)
		reqParams = betaMessages.BetaMessageNewParams

	} else {
		if err := json.Unmarshal(bodyBytes, messages); err != nil {
			logrus.WithError(err).Errorf("Anthropic decode error")
			c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Message decode error: %s", err.Error()),
					Type:    "invalid_request_error",
				},
			})
			return
		}

		requestModel = string(messages.Model)
		reqParams = messages.MessageNewParams
	}

	// Check if this is the request requestModel name first
	rule, err = ph.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}
	if rule == nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "no such rule",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	ph.applyVisionProxy(c, scenarioType, rule, reqParams)

	// Select service using routing pipeline
	provider, selectedService, err = ph.deps.RoutingSelector.SelectService(c, scenarioType, rule, reqParams)
	if err != nil {
		logrus.WithError(err).Errorf("Select service error")
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	actualModel := selectedService.Model

	// Delegate to the appropriate implementation based on beta parameter
	if beta {
		ph.AnthropicMessagesV1Beta(c, betaMessages, actualModel, requestModel, rule, provider)

	} else {
		ph.AnthropicMessagesV1(c, messages, actualModel, requestModel, rule, provider)
	}
}

// AnthropicMessagesV1 implements standard v1 messages API.
//
// It runs the provider-independent prologue once, then drives the failover loop
// whose per-attempt callback re-runs the whole provider-dependent pipeline
// (pre-chain → guardrails → target resolution → transform → dispatch) against
// the candidate selected for that attempt. Because the transform is re-run per
// attempt, failover can rotate across heterogeneous API styles.
func (ph *ProtocolHandler) AnthropicMessagesV1(c *gin.Context, req *protocol.AnthropicMessagesRequest, requestModel string, responseModel string, rule *typ.Rule, provider *typ.Provider) {
	// ── One-time prologue (provider-independent) ──

	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex.
	// (Mutates the shared rule only, so it must run before any attempt.)
	applyContextOneM(c, rule)

	scenarioType := rule.GetScenario()
	isStreaming := req.Stream
	scenarioConfig := ph.deps.Config.GetScenarioConfig(scenarioType)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, req.MessageNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata. Provider/model are refreshed per
	// attempt by the failover loop (UpdateTrackingForFailover).
	SetTrackingContext(c, rule, provider, requestModel, responseModel, isStreaming)

	// Get or create the recorder for dual-stage recording. The body is the
	// pristine request as received (post-vision-proxy, pre-pre-chain); the
	// winning attempt's provider/model is re-bound per attempt via SetActiveService.
	var recorder *recording.ProtocolRecorder
	if scenarioConfig.IsRecordingEnable() {
		bs, err := req.MarshalJSON()
		if err != nil {
			bs = []byte("{}")
		}
		recorder = ph.EnsureProtocolRecorder(c, string(scenarioType), provider, requestModel, ph.getScenarioRecordMode(scenarioType), bs)
	}

	// Snapshot a pristine template only when failover is possible; the single
	// service case reuses the original request with no clone overhead.
	multi := len(rule.GetActiveServices()) > 1
	var template []byte
	if multi {
		bs, err := req.MarshalJSON()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		template = bs
	}

	// ── Per-attempt pipeline (provider-dependent) ──
	ph.DispatchWithPriorityFailover(c, rule, provider, requestModel,
		func(p *typ.Provider, retryModel string) {
			areq := req
			if multi {
				cloned, err := CloneAnthropicV1Request(template)
				if err != nil {
					ph.FailAttemptSetup(c, err)
					return
				}
				areq = cloned
			}
			ph.runAnthropicV1Attempt(c, areq, responseModel, p, retryModel, rule, isStreaming, scenarioType, scenarioConfig, recorder)
		})
}

// runAnthropicV1Attempt executes the provider-dependent half of an Anthropic v1
// request for one failover attempt: resolve the dual endpoint, run the
// pre-transform chain and guardrails, resolve the target API for this provider's
// style, transform, and dispatch. Setup failures route through failAttemptSetup
// so the orchestrator can advance to the next candidate.
func (ph *ProtocolHandler) runAnthropicV1Attempt(c *gin.Context, req *protocol.AnthropicMessagesRequest, responseModel string, provider *typ.Provider, requestModel string, rule *typ.Rule, isStreaming bool, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig, recorder *recording.ProtocolRecorder) {
	// Resolve dual endpoint: when the provider has an Anthropic-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = provider.ResolveStyle(protocol.APIStyleAnthropic)
	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	req.Model = anthropic.Model(requestModel)

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", requestModel)

	// Build and run server-side pre-transform chain (scenario-driven flags)
	maxAllowed := ph.deps.TemplateManager.GetMaxTokensForModelByProvider(provider, requestModel)
	if err := ExecuteAnthropicPreChain(
		req.MessageNewParams, scenarioConfig,
		ph.deps.Config.GetDefaultMaxTokens(), maxAllowed, isStreaming,
	); err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}

	scenario := GetTrackingContextScenario(c)
	if ph.guardrailsEnabledForScenario(scenario) {
		ApplyGuardrailsToAnthropicV1Request(c, ph.currentGuardrailsRuntime(), req.MessageNewParams, requestModel, provider)
	}

	// Determine target API type for protocol transformation detection
	target := protocol.TypeAnthropicV1
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicV1
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, ResolveRuleFlags(c, rule), IncomingAPIResponses)
		if routeErr != nil {
			ph.FailAttemptSetup(c, routeErr)
			return
		}
		target = resolvedTarget
	}

	// Resolve flags with scenario injection and auto-apply for CleanHeader.
	// (This also applies the custom User-Agent to the request context.)
	ruleFlags := ResolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeAnthropicV1, target, provider)

	reqCtx, err := ph.TransformAnthropicV1(c, req, target, provider, isStreaming, recorder, scenarioType, RulePreBaseTransforms(ruleFlags), RulePreVendorTransforms(ruleFlags))
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	defer reqCtx.Release()

	reqCtx.RequestModel = requestModel
	reqCtx.ResponseModel = responseModel

	ph.DispatchChainResult(c, reqCtx, rule, provider, isStreaming, recorder)
}

// AnthropicMessagesV1Beta implements beta messages API.
//
// Like AnthropicMessagesV1, the provider-independent prologue runs once and the
// per-attempt callback re-runs the provider-dependent pipeline so failover can
// rotate across heterogeneous API styles.
func (ph *ProtocolHandler) AnthropicMessagesV1Beta(c *gin.Context, req *protocol.AnthropicBetaMessagesRequest, requestModel string, responseModel string, rule *typ.Rule, provider *typ.Provider) {
	// ── One-time prologue (provider-independent) ──

	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex.
	// (Mutates the shared rule only, so it must run before any attempt.)
	applyContextOneM(c, rule)

	scenarioType := rule.GetScenario()
	isStreaming := req.Stream
	scenarioConfig := ph.deps.Config.GetScenarioConfig(scenarioType)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, req.BetaMessageNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata. Provider/model are refreshed per
	// attempt by the failover loop (UpdateTrackingForFailover).
	SetTrackingContext(c, rule, provider, requestModel, responseModel, isStreaming)

	// Get or create the recorder for dual-stage recording (pristine request body).
	var recorder *recording.ProtocolRecorder
	if scenarioConfig.IsRecordingEnable() {
		bs, err := req.MarshalJSON()
		if err != nil {
			bs = []byte("{}")
		}
		recorder = ph.EnsureProtocolRecorder(c, string(scenarioType), provider, requestModel, ph.getScenarioRecordMode(scenarioType), bs)
	}

	// Snapshot a pristine template only when failover is possible.
	multi := len(rule.GetActiveServices()) > 1
	var template []byte
	if multi {
		bs, err := req.MarshalJSON()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		template = bs
	}

	// ── Per-attempt pipeline (provider-dependent) ──
	ph.DispatchWithPriorityFailover(c, rule, provider, requestModel,
		func(p *typ.Provider, retryModel string) {
			areq := req
			if multi {
				cloned, err := CloneAnthropicBetaRequest(template)
				if err != nil {
					ph.FailAttemptSetup(c, err)
					return
				}
				areq = cloned
			}
			ph.runAnthropicBetaAttempt(c, areq, responseModel, p, retryModel, rule, isStreaming, scenarioType, scenarioConfig, recorder)
		})
}

// runAnthropicBetaAttempt executes the provider-dependent half of an Anthropic
// beta request for one failover attempt. See runAnthropicV1Attempt.
func (ph *ProtocolHandler) runAnthropicBetaAttempt(c *gin.Context, req *protocol.AnthropicBetaMessagesRequest, responseModel string, provider *typ.Provider, requestModel string, rule *typ.Rule, isStreaming bool, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig, recorder *recording.ProtocolRecorder) {
	// Resolve dual endpoint: when the provider has an Anthropic-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = provider.ResolveStyle(protocol.APIStyleAnthropic)
	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	req.Model = anthropic.Model(requestModel)

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", requestModel)

	// Build and run server-side pre-transform chain (scenario-driven flags)
	maxAllowed := ph.deps.TemplateManager.GetMaxTokensForModelByProvider(provider, requestModel)
	if err := ExecuteAnthropicPreChain(
		req.BetaMessageNewParams, scenarioConfig,
		ph.deps.Config.GetDefaultMaxTokens(), maxAllowed, isStreaming,
	); err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}

	// request guardrails
	scenario := GetTrackingContextScenario(c)
	if ph.guardrailsEnabledForScenario(scenario) {
		ApplyGuardrailsToAnthropicV1BetaRequest(c, ph.currentGuardrailsRuntime(), req.BetaMessageNewParams, requestModel, provider)
	}

	// Determine target API type for protocol transformation detection
	target := protocol.TypeAnthropicBeta
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicBeta
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, ResolveRuleFlags(c, rule), IncomingAPIResponses)
		if routeErr != nil {
			ph.FailAttemptSetup(c, routeErr)
			return
		}
		target = resolvedTarget
	}

	// Resolve flags with scenario injection and auto-apply for CleanHeader.
	// (This also applies the custom User-Agent to the request context.)
	ruleFlags := ResolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeAnthropicBeta, target, provider)

	reqCtx, err := ph.TransformAnthropicBeta(c, req, target, provider, isStreaming, recorder, scenarioType, RulePreBaseTransforms(ruleFlags), RulePreVendorTransforms(ruleFlags))
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	defer reqCtx.Release()

	reqCtx.RequestModel = requestModel
	reqCtx.ResponseModel = responseModel

	ph.DispatchChainResult(c, reqCtx, rule, provider, isStreaming, recorder)
}
