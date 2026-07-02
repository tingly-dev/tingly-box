package server

import (
	"strings"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// compactWakeModel is the built-in TransformModel that performs the rapid XML
// compaction. It lives on the shared vmodel-builtin-anthropic provider
// alongside the other virtual models — see vmodel/anthropic/defaults.go.
const compactWakeModel = "claude-code-compact"

// compactWakeMatches decides whether typedRequest should be short-circuited to
// the rapid-compact virtual model: scenario is claude_code, keyword is
// non-empty, and the latest user message (which must actually carry text —
// a trailing tool_result must not match a stale earlier message) contains it,
// case-insensitively. Pure and side-effect-free so the matching logic is
// testable without a provider store.
func compactWakeMatches(scenarioType typ.RuleScenario, rule *typ.Rule, keyword string, typedRequest any) bool {
	if !scenarioType.Is(typ.ScenarioClaudeCode) || rule == nil || typedRequest == nil || keyword == "" {
		return false
	}
	reqCtx := smartrouting.ExtractContext(typedRequest)
	if reqCtx == nil || !reqCtx.LatestUserHasText {
		return false
	}
	return strings.Contains(strings.ToLower(reqCtx.GetLatestUserMessage()), strings.ToLower(keyword))
}

// applyCompactWake is the single entry point for the Claude Code rapid-compact
// plugin. Like applyVisionProxy it runs before service selection (after the
// rule is resolved) — but unlike vision proxy, a match does not mutate the
// request in place and continue: it short-circuits the whole request to the
// local XML-compaction virtual model, so the forced (provider, service) pair
// is returned instead of nil, and the caller skips normal service selection
// entirely. This keeps the "instant response, no upstream token cost"
// property (the virtual model is a terminal TransformModel, see
// vmodel/anthropic/transform_model.go) while triggering upfront instead of
// through smart routing. Returns (nil, nil) when the request should proceed
// to normal service selection unchanged.
func (s *Server) applyCompactWake(scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any) (*typ.Provider, *loadbalance.Service) {
	if rule == nil {
		return nil, nil
	}
	keyword := s.config.GetEffectiveCompactKeyword(rule)
	if !compactWakeMatches(scenarioType, rule, keyword, typedRequest) {
		return nil, nil
	}

	provider, err := s.config.GetProviderByUUID(virtualserver.BuiltinAnthropicUUID)
	if err != nil || provider == nil || !provider.Enabled {
		return nil, nil
	}

	return provider, &loadbalance.Service{
		Provider: virtualserver.BuiltinAnthropicUUID,
		Model:    compactWakeModel,
		Active:   true,
		Weight:   1,
	}
}
