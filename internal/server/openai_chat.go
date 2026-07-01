package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// OpenAIChatCompletion runs the provider-independent prologue once, then drives
// the failover loop whose per-attempt callback re-runs the provider-dependent
// pipeline (align → cap → target resolution → transform → dispatch) so failover
// can rotate across heterogeneous API styles.
func (s *Server) OpenAIChatCompletion(c *gin.Context, req *protocol.OpenAIChatCompletionRequest, responseModel string, provider *typ.Provider, scenarioType typ.RuleScenario, rule *typ.Rule) {
	// ── One-time prologue (provider-independent) ──

	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex.
	detectAndApplyContext1MFromIncomingRequest(c, rule)

	isStreaming := req.Stream
	actualModel := req.Model
	scenarioConfig := s.config.GetScenarioConfig(scenarioType)

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
	s.dispatchWithPriorityFailover(c, rule, provider, actualModel,
		func(p *typ.Provider, retryModel string) {
			areq := req
			if multi {
				cloned, err := cloneOpenAIChatRequest(template)
				if err != nil {
					s.failAttemptSetup(c, err)
					return
				}
				areq = cloned
			}
			s.runOpenAIChatAttempt(c, areq, responseModel, p, retryModel, rule, isStreaming, scenarioType, scenarioConfig)
		})
}

// runOpenAIChatAttempt executes the provider-dependent half of an OpenAI chat
// request for one failover attempt. Setup failures route through
// failAttemptSetup so the orchestrator can advance to the next candidate.
func (s *Server) runOpenAIChatAttempt(c *gin.Context, req *protocol.OpenAIChatCompletionRequest, responseModel string, provider *typ.Provider, actualModel string, rule *typ.Rule, isStreaming bool, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig) {
	// Resolve dual endpoint: when the provider has an OpenAI-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = s.resolveProviderForClient(provider, protocol.APIStyleOpenAI)
	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	req.Model = actualModel
	maxAllowed := s.templateManager.GetMaxTokensForModelByProvider(provider, actualModel)

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
		tempFlags := resolveRuleFlags(c, rule)
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, tempFlags, IncomingAPIChat)
		if routeErr != nil {
			s.failAttemptSetup(c, routeErr)
			return
		}
		target = resolvedTarget
	default:
		s.failAttemptSetup(c, fmt.Errorf("Unsupported API style: %s %s", provider.Name, apiStyle))
		return
	}

	// === Resolve flags with scenario injection ===
	// (resolveRuleFlagsWithScenario also applies the custom User-Agent to the
	// request context, so no separate call is needed here.)
	ruleFlags := resolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeOpenAIChat, target, provider)

	// === Transform via pipeline ===
	reqCtx, err := s.transformOpenAIChat(c, req, target, provider, isStreaming, nil, scenarioType, rulePreBaseTransforms(ruleFlags), rulePreVendorTransforms(ruleFlags))
	if err != nil {
		s.failAttemptSetup(c, fmt.Errorf("Transform failed: %w", err))
		return
	}
	defer reqCtx.Release()

	reqCtx.Extra["cursor_compat"] = ruleFlags.CursorCompat
	reqCtx.Extra["skip_usage"] = ruleFlags.SkipUsage

	// === Dispatch via transform chain ===
	reqCtx.RequestModel = actualModel
	reqCtx.ResponseModel = responseModel
	s.dispatchChainResult(c, reqCtx, rule, provider, isStreaming, nil)
}
