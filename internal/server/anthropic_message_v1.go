package server

import (
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicMessagesV1 implements standard v1 messages API.
//
// It runs the provider-independent prologue once, then drives the failover loop
// whose per-attempt callback re-runs the whole provider-dependent pipeline
// (pre-chain → guardrails → target resolution → transform → dispatch) against
// the candidate selected for that attempt. Because the transform is re-run per
// attempt, failover can rotate across heterogeneous API styles.
func (s *Server) AnthropicMessagesV1(c *gin.Context, req *protocol.AnthropicMessagesRequest, requestModel string, responseModel string, rule *typ.Rule, provider *typ.Provider) {
	// ── One-time prologue (provider-independent) ──

	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex.
	// (Mutates the shared rule only, so it must run before any attempt.)
	detectAndApplyContext1MFromIncomingRequest(c, rule)

	scenarioType := rule.GetScenario()
	isStreaming := req.Stream
	scenarioConfig := s.config.GetScenarioConfig(scenarioType)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, req.MessageNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata. Provider/model are refreshed per
	// attempt by the failover loop (UpdateTrackingForFailover).
	SetTrackingContext(c, rule, provider, requestModel, responseModel, isStreaming)

	// Get or create the recorder for dual-stage recording. The body is the
	// pristine request as received (post-vision-proxy, pre-pre-chain); the
	// winning attempt's provider/model is re-bound per attempt via SetActiveService.
	var recorder *ProtocolRecorder
	if scenarioConfig.IsRecording() {
		bs, err := req.MarshalJSON()
		if err != nil {
			bs = []byte("{}")
		}
		recorder = s.EnsureProtocolRecorder(c, string(scenarioType), provider, requestModel, s.GetScenarioRecordMode(scenarioType), bs)
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
	s.dispatchWithPriorityFailover(c, rule, provider, requestModel,
		func(p *typ.Provider, retryModel string) {
			areq := req
			if multi {
				cloned, err := cloneAnthropicV1Request(template)
				if err != nil {
					s.failAttemptSetup(c, err)
					return
				}
				areq = cloned
			}
			s.runAnthropicV1Attempt(c, areq, responseModel, p, retryModel, rule, isStreaming, scenarioType, scenarioConfig, recorder)
		})
}

// runAnthropicV1Attempt executes the provider-dependent half of an Anthropic v1
// request for one failover attempt: resolve the dual endpoint, run the
// pre-transform chain and guardrails, resolve the target API for this provider's
// style, transform, and dispatch. Setup failures route through failAttemptSetup
// so the orchestrator can advance to the next candidate.
func (s *Server) runAnthropicV1Attempt(c *gin.Context, req *protocol.AnthropicMessagesRequest, responseModel string, provider *typ.Provider, requestModel string, rule *typ.Rule, isStreaming bool, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig, recorder *ProtocolRecorder) {
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
	if err := executeAnthropicV1PreChain(
		req.MessageNewParams, scenarioConfig,
		s.config.GetDefaultMaxTokens(), maxAllowed, isStreaming,
	); err != nil {
		s.failAttemptSetup(c, err)
		return
	}

	scenario := GetTrackingContextScenario(c)
	if s.guardrailsEnabledForScenario(scenario) {
		s.applyGuardrailsToAnthropicV1Request(c, req.MessageNewParams, requestModel, provider)
	}

	// Determine target API type for protocol transformation detection
	target := protocol.TypeAnthropicV1
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicV1
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
	ruleFlags := resolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeAnthropicV1, target, provider)

	reqCtx, err := s.transformAnthropicV1(c, req, target, provider, isStreaming, recorder, scenarioType, rulePreBaseTransforms(ruleFlags), rulePreVendorTransforms(ruleFlags))
	if err != nil {
		s.failAttemptSetup(c, err)
		return
	}
	defer reqCtx.Release()

	reqCtx.RequestModel = requestModel
	reqCtx.ResponseModel = responseModel

	s.dispatchChainResult(c, reqCtx, rule, provider, isStreaming, recorder)
}
