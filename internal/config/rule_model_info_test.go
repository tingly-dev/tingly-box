package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RuleModelInfo reads every upstream a rule can reach, the same ones the Codex
// window does: smart routes count while smart routing is on, services on a
// disabled or deleted provider never answer.
func TestRuleModelInfo_ReachableUpstreams(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, true)

	rule := codexRule("smart", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-v4-flash"))
	rule.SmartRouting = []smartrouting.SmartRouting{{Description: "short", Services: []*loadbalance.Service{codexSvc(p.deepseek, "deepseek-chat")}}}
	info, ok := cfg.RuleModelInfo(rule, ai.APIStyleOpenAI)
	require.True(t, ok)
	assert.Equal(t, 1000000, info.Context, "routes are ignored while smart routing is off")
	rule.SmartEnabled = true
	info, _ = cfg.RuleModelInfo(rule, ai.APIStyleOpenAI)
	assert.Equal(t, 128000, info.Context)

	rule = codexRule("gone", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-v4-flash"), codexSvc(p.disabled, "deepseek-chat"),
		&loadbalance.Service{Provider: "deleted-provider", Model: "deepseek-chat", Active: true})
	info, _ = cfg.RuleModelInfo(rule, ai.APIStyleOpenAI)
	assert.Equal(t, 1000000, info.Context)

	_, ok = cfg.RuleModelInfo(codexRule("off", typ.RuleFlags{}, codexSvc(p.disabled, "deepseek-chat")), ai.APIStyleOpenAI)
	assert.False(t, ok)
}

// The larger opt-in window is kept for one upstream only: templates count it
// differently (OpenAI's includes output), so a merge would mix units. The
// Codex window merges it per upstream instead.
func TestRuleModelInfo_MaxContextFromOneUpstream(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, true)

	info, ok := cfg.RuleModelInfo(codexRule("sol", typ.RuleFlags{}, codexSvc(p.chatgpt, "gpt-6-sol")), ai.APIStyleOpenAI)
	require.True(t, ok)
	assert.Equal(t, [2]int{272000, 872000}, [2]int{info.Context, info.MaxContext})

	// Either order: the upstream with the larger window may be read first.
	for _, mix := range []typ.Rule{
		codexRule("mix", typ.RuleFlags{}, codexSvc(p.openai, "gpt-5.4-mini"), codexSvc(p.chatgpt, "gpt-6-sol")),
		codexRule("mix", typ.RuleFlags{}, codexSvc(p.chatgpt, "gpt-6-sol"), codexSvc(p.openai, "gpt-5.4-mini")),
	} {
		info, _ = cfg.RuleModelInfo(mix, ai.APIStyleOpenAI)
		assert.Equal(t, [3]int{272000, 272000, 0}, [3]int{info.Context, info.InputLimit(), info.MaxContext})
		assert.Equal(t, CodexContextWindow{Window: 272000, Max: 272000, Source: CodexWindowSourceCatalog}, cfg.CodexAutoContextWindow(mix))
	}
}

// The rule's 1M flag adds the context-1m header only to requests sent to the
// provider's Anthropic endpoint, the one the gateway picks for the client.
func TestSendsContext1M(t *testing.T) {
	_, p := newCodexWindowConfig(t, true)
	on := typ.RuleFlags{Context1M: true}
	forced := typ.RuleFlags{Context1M: true, AnthropicEndpointOverride: "anthropic"}
	tests := []struct {
		name     string
		provider *typ.Provider
		flags    typ.RuleFlags
		client   ai.APIStyle
		want     bool
	}{
		{"Anthropic endpoint with the flag", p.claudeCode, on, ai.APIStyleOpenAI, true},
		{"without the flag", p.claudeCode, typ.RuleFlags{}, ai.APIStyleAnthropic, false},
		{"a dual provider serves an OpenAI client on its OpenAI URL", p.dual, on, ai.APIStyleOpenAI, false},
		{"and an Anthropic client on its Anthropic URL", p.dual, on, ai.APIStyleAnthropic, true},
		{"unless the rule forces the Anthropic endpoint", p.dual, forced, ai.APIStyleOpenAI, true},
		{"OpenAI-only provider, forced or not", p.chatgpt, forced, ai.APIStyleAnthropic, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sendsContext1M(tt.provider, codexRule("m", tt.flags), tt.client))
		})
	}
}

// Where the 1M flag adds the header, the model lists give the window it opens:
// Opus 4.6 on a Claude subscription is 200K, 1M with the header. A larger
// window no header opens (ChatGPT's 872K, which only Codex can ask for) stays
// the ceiling.
func TestRuleModelInfo_Context1MOpensHeaderWindow(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, true)
	window := func(rule typ.Rule, client ai.APIStyle) [3]int {
		t.Helper()
		info, ok := cfg.RuleModelInfo(rule, client)
		require.True(t, ok)
		return [3]int{info.Context, info.InputLimit(), info.MaxContext}
	}
	opus := codexSvc(p.claudeCode, "claude-opus-4-6")
	for _, client := range []ai.APIStyle{ai.APIStyleAnthropic, ai.APIStyleOpenAI} {
		assert.Equal(t, [3]int{200000, 200000, 1000000}, window(codexRule("opus", typ.RuleFlags{}, opus), client), client)
		assert.Equal(t, [3]int{1000000, 1000000, 0}, window(codexRule("opus", typ.RuleFlags{Context1M: true}, opus), client), client)
	}
	sol := codexSvc(p.chatgpt, "gpt-6-sol")
	assert.Equal(t, [3]int{272000, 272000, 872000}, window(codexRule("sol", typ.RuleFlags{Context1M: true}, sol), ai.APIStyleOpenAI))
}
