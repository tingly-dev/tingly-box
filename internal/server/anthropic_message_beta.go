package server

import (
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicMessagesV1Beta implements beta messages API.
//
// Like AnthropicMessagesV1, the provider-independent prologue runs once and the
// per-attempt callback re-runs the provider-dependent pipeline so failover can
// rotate across heterogeneous API styles.
func (s *Server) AnthropicMessagesV1Beta(c *gin.Context, req *protocol.AnthropicBetaMessagesRequest, requestModel string, responseModel string, rule *typ.Rule, provider *typ.Provider) {
	// ── One-time prologue (provider-independent) ──

	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex.
	// (Mutates the shared rule only, so it must run before any attempt.)
	detectAndApplyContext1MFromIncomingRequest(c, rule)

	scenarioType := rule.GetScenario()
	isStreaming := req.Stream
	scenarioConfig := s.config.GetScenarioConfig(scenarioType)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, req.BetaMessageNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata. Provider/model are refreshed per
	// attempt by the failover loop (UpdateTrackingForFailover).
	SetTrackingContext(c, rule, provider, requestModel, responseModel, isStreaming)

	// Get or create the recorder for dual-stage recording (pristine request body).
	var recorder *ProtocolRecorder
	if scenarioConfig.IsRecording() {
		bs, err := req.MarshalJSON()
		if err != nil {
			bs = []byte("{}")
		}
		recorder = s.EnsureProtocolRecorder(c, string(scenarioType), provider, requestModel, s.GetScenarioRecordMode(scenarioType), bs)
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
	s.dispatchWithPriorityFailover(c, rule, provider, requestModel,
		func(p *typ.Provider, retryModel string) {
			areq := req
			if multi {
				cloned, err := cloneAnthropicBetaRequest(template)
				if err != nil {
					s.failAttemptSetup(c, err)
					return
				}
				areq = cloned
			}
			s.runAnthropicBetaAttempt(c, areq, responseModel, p, retryModel, rule, isStreaming, scenarioType, scenarioConfig, recorder)
		})
}

// runAnthropicBetaAttempt executes the provider-dependent half of an Anthropic
// beta request for one failover attempt. See runAnthropicV1Attempt.
func (s *Server) runAnthropicBetaAttempt(c *gin.Context, req *protocol.AnthropicBetaMessagesRequest, responseModel string, provider *typ.Provider, requestModel string, rule *typ.Rule, isStreaming bool, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig, recorder *ProtocolRecorder) {
	// Resolve dual endpoint: when the provider has an Anthropic-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = s.resolveProviderForClient(provider, protocol.APIStyleAnthropic)
	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	req.Model = anthropic.Model(requestModel)

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", requestModel)

	// Build and run server-side pre-transform chain (scenario-driven flags)
	maxAllowed := s.templateManager.GetMaxTokensForModelByProvider(provider, requestModel)
	if err := executeAnthropicBetaPreChain(
		req.BetaMessageNewParams, scenarioConfig,
		s.config.GetDefaultMaxTokens(), maxAllowed, isStreaming,
	); err != nil {
		s.failAttemptSetup(c, err)
		return
	}

	// request guardrails
	scenario := GetTrackingContextScenario(c)
	if s.guardrailsEnabledForScenario(scenario) {
		s.applyGuardrailsToAnthropicV1BetaRequest(c, req.BetaMessageNewParams, requestModel, provider)
	}

	// Determine target API type for protocol transformation detection
	target := protocol.TypeAnthropicBeta
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicBeta
	case protocol.APIStyleGoogle:
		target = protocol.TypeGoogle
	case protocol.APIStyleOpenAI:
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, resolveRuleFlags(c, rule), IncomingAPIResponses)
		if routeErr != nil {
			s.failAttemptSetup(c, routeErr)
			return
		}
		target = resolvedTarget
	}

	// Resolve flags with scenario injection and auto-apply for CleanHeader.
	// (This also applies the custom User-Agent to the request context.)
	ruleFlags := resolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeAnthropicBeta, target, provider)

	reqCtx, err := s.transformAnthropicBeta(c, req, target, provider, isStreaming, recorder, scenarioType, rulePreBaseTransforms(ruleFlags), rulePreVendorTransforms(ruleFlags))
	if err != nil {
		s.failAttemptSetup(c, err)
		return
	}
	defer reqCtx.Release()

	reqCtx.RequestModel = requestModel
	reqCtx.ResponseModel = responseModel

	s.dispatchChainResult(c, reqCtx, rule, provider, isStreaming, recorder)
}
