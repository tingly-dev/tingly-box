package config

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// codexWindowProviders are upstreams the embedded provider catalog knows (or
// deliberately doesn't), for the window resolver tests.
type codexWindowProviders struct {
	deepseek, deepseekAnthropic, anthropic, claudeCode, chatgpt, openai, house, disabled, dual *typ.Provider
}

func newCodexWindowConfig(t *testing.T, withTemplateManager bool) (*Config, codexWindowProviders) {
	t.Helper()
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	if withTemplateManager {
		tm := catalog.NewProviderCatalogManager(catalog.EmbeddedOnly())
		require.NoError(t, tm.Initialize(context.Background()))
		cfg.SetTemplateManager(tm)
	}
	key := func(name, base string, style protocol.APIStyle) *typ.Provider {
		return &typ.Provider{Name: name, APIBase: base, APIStyle: style, AuthType: typ.AuthTypeAPIKey, Token: "k", Enabled: true}
	}
	p := codexWindowProviders{
		deepseek:          key("DeepSeek", "https://api.deepseek.com/v1", protocol.APIStyleOpenAI),
		deepseekAnthropic: key("DeepSeek (Anthropic)", "https://api.deepseek.com/anthropic", protocol.APIStyleAnthropic),
		anthropic:         key("Anthropic", "https://api.anthropic.com", protocol.APIStyleAnthropic),
		openai:            key("OpenAI", "https://api.openai.com/v1", protocol.APIStyleOpenAI),
		house:             key("House", "https://llm.example.internal/v1", protocol.APIStyleOpenAI),
		disabled:          key("Old DeepSeek", "https://api.deepseek.com/v1", protocol.APIStyleOpenAI),
		dual:              key("Anthropic dual", "https://api.anthropic.com", protocol.APIStyleAnthropic),
		chatgpt: &typ.Provider{Name: "ChatGPT", APIBase: "https://chatgpt.com/backend-api", APIStyle: protocol.APIStyleOpenAI,
			AuthType: typ.AuthTypeOAuth, OAuthDetail: &typ.OAuthDetail{Issuer: "codex", AccessToken: "t"}, Enabled: true},
		claudeCode: &typ.Provider{Name: "Claude Code", APIBase: "https://api.anthropic.com", APIStyle: protocol.APIStyleAnthropic,
			AuthType: typ.AuthTypeOAuth, OAuthDetail: &typ.OAuthDetail{Issuer: "claude_code", AccessToken: "t"}, Enabled: true},
	}
	p.dual.APIBaseOpenAI = "https://api.anthropic.com/v1/openai"
	p.dual.APIBaseAnthropic = "https://api.anthropic.com"
	for _, prov := range []*typ.Provider{p.deepseek, p.deepseekAnthropic, p.anthropic, p.claudeCode, p.chatgpt, p.openai, p.house, p.disabled, p.dual} {
		require.NoError(t, cfg.AddProvider(prov))
	}
	// The store creates providers enabled; switch this one off afterwards.
	p.disabled.Enabled = false
	require.NoError(t, cfg.UpdateProvider(p.disabled.UUID, p.disabled))
	got, err := cfg.GetProviderByUUID(p.disabled.UUID)
	require.NoError(t, err)
	require.False(t, got.Enabled)
	return cfg, p
}

func codexSvc(provider *typ.Provider, model string) *loadbalance.Service {
	return &loadbalance.Service{Provider: provider.UUID, Model: model, Active: true}
}

func codexRule(model string, flags typ.RuleFlags, services ...*loadbalance.Service) typ.Rule {
	return typ.Rule{UUID: "r-" + model, Scenario: typ.ScenarioCodex, RequestModel: model, Active: true, Flags: flags, Services: services}
}

// Each kind of upstream contributes the input window it really answers with,
// and the 1M flag only matters where it opens a larger one (a catalog model's
// max_context, or a model nothing is known about).
func TestCodexAutoContextWindow_PerUpstream(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, true)
	on := typ.RuleFlags{Context1M: true}
	off := typ.RuleFlags{}

	tests := []struct {
		name string
		rule typ.Rule
		want CodexContextWindow
	}{
		{"catalog model", codexRule("ds", off, codexSvc(p.deepseek, "deepseek-chat")),
			CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceCatalog}},
		{"1M flag doesn't widen a known non-Claude model", codexRule("ds", on, codexSvc(p.deepseek, "deepseek-chat")),
			CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceCatalog}},
		{"non-Claude model behind an Anthropic-style endpoint keeps its window", codexRule("ds", on, codexSvc(p.deepseekAnthropic, "deepseek-chat")),
			CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceCatalog}},
		{"1M model on DeepSeek", codexRule("flash", off, codexSvc(p.deepseek, "deepseek-v4-flash")),
			CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceCatalog}},
		{"Claude 4.6+ on an API key is 1M without any header", codexRule("sonnet", off, codexSvc(p.anthropic, "claude-sonnet-4-6")),
			CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceCatalog}},
		{"… so the 1M flag changes nothing", codexRule("sonnet", on, codexSvc(p.anthropic, "claude-sonnet-4-6")),
			CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceCatalog}},
		{"… over a dual provider's OpenAI URL too", codexRule("sonnet", off, codexSvc(p.dual, "claude-sonnet-4-6")),
			CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceCatalog}},
		{"Claude whose catalog window is 200K", codexRule("sonnet", on, codexSvc(p.claudeCode, "claude-sonnet-4-5")),
			CodexContextWindow{Window: 200000, Max: 200000, Source: CodexWindowSourceCatalog}},
		{"Claude 4.6 on a subscription answers with 200K without the context-1m header", codexRule("opus", off, codexSvc(p.claudeCode, "claude-opus-4-6")),
			CodexContextWindow{Window: 200000, Max: 200000, Source: CodexWindowSourceCatalog}},
		{"… and the 1M flag adds it", codexRule("opus", on, codexSvc(p.claudeCode, "claude-opus-4-6")),
			CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceExtended}},
		{"Claude 4.7+ on a subscription is 1M without it", codexRule("opus", off, codexSvc(p.claudeCode, "claude-opus-4-7")),
			CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceCatalog}},
		{"ChatGPT-backed model: 272K, up to 872K", codexRule("sol", off, codexSvc(p.chatgpt, "gpt-6-sol")),
			CodexContextWindow{Window: 272000, Max: 872000, Source: CodexWindowSourceCatalog}},
		{"the 1M flag opens a ChatGPT-backed model's 872K, not 1M (#2131)", codexRule("sol", on, codexSvc(p.chatgpt, "gpt-6-sol")),
			CodexContextWindow{Window: 872000, Max: 872000, Source: CodexWindowSourceExtended}},
		{"OpenAI's published window counts output: Codex gets the input limit", codexRule("mini", off, codexSvc(p.openai, "gpt-5.4-mini")),
			CodexContextWindow{Window: 272000, Max: 272000, Source: CodexWindowSourceCatalog}},
		{"unknown model", codexRule("house", off, codexSvc(p.house, "house-model")),
			CodexContextWindow{Window: 200000, Max: 200000, Source: CodexWindowSourceDefault}},
		{"unknown model keeps the old 1M meaning, still a guess", codexRule("house", on, codexSvc(p.house, "house-model")),
			CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceDefault}},
		{"service without a model is looked up by the request model", codexRule("deepseek-chat", off, codexSvc(p.deepseek, "")),
			CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceCatalog}},
		{"request model is looked up when the service model is unknown", codexRule("deepseek-reasoner", off, codexSvc(p.deepseek, "ds-custom")),
			CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceCatalog}},
		{"no services", codexRule("empty", off),
			CodexContextWindow{Window: 200000, Max: 200000, Source: CodexWindowSourceDefault}},
		{"no services with 1M", codexRule("empty", on),
			CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceDefault}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cfg.CodexAutoContextWindow(tt.rule))
		})
	}
}

// A rule that can reach several upstreams gets the smallest window and the
// smallest ceiling; upstreams that never answer don't count.
func TestCodexAutoContextWindow_SmallestReachable(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, true)

	// Load-balanced: the 128K model decides the window, the ChatGPT model's
	// 872K ceiling doesn't survive the 128K one.
	rule := codexRule("mix", typ.RuleFlags{}, codexSvc(p.chatgpt, "gpt-6-sol"), codexSvc(p.deepseek, "deepseek-chat"), codexSvc(p.deepseek, "deepseek-v4-flash"))
	assert.Equal(t, CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceCatalog},
		cfg.CodexAutoContextWindow(rule))

	// The smallest window and the smallest ceiling can come from different
	// upstreams: 272K/872K and 1M/1M give 272K/872K.
	rule = codexRule("mix", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-v4-flash"), codexSvc(p.chatgpt, "gpt-6-sol"))
	assert.Equal(t, CodexContextWindow{Window: 272000, Max: 872000, Source: CodexWindowSourceCatalog},
		cfg.CodexAutoContextWindow(rule))

	// Inactive services, disabled providers and deleted providers never answer.
	inactive := codexSvc(p.deepseek, "deepseek-chat")
	inactive.Active = false
	rule = codexRule("flash", typ.RuleFlags{},
		codexSvc(p.deepseek, "deepseek-v4-flash"), inactive,
		codexSvc(p.disabled, "deepseek-chat"),
		&loadbalance.Service{Provider: "deleted-provider", Model: "deepseek-chat", Active: true}, nil)
	assert.Equal(t, CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceCatalog},
		cfg.CodexAutoContextWindow(rule))

	// On a tie, the known window wins over the guess, in either order.
	house, claude := codexSvc(p.house, "house-model"), codexSvc(p.claudeCode, "claude-sonnet-4-5")
	for _, svcs := range [][]*loadbalance.Service{{house, claude}, {claude, house}} {
		assert.Equal(t, CodexContextWindow{Window: 200000, Max: 200000, Source: CodexWindowSourceCatalog},
			cfg.CodexAutoContextWindow(codexRule("tie", typ.RuleFlags{}, svcs...)))
	}

	// Only unreachable upstreams: judged like an unknown model.
	rule = codexRule("gone", typ.RuleFlags{}, codexSvc(p.disabled, "deepseek-chat"))
	assert.Equal(t, CodexContextWindow{Window: 200000, Max: 200000, Source: CodexWindowSourceDefault}, cfg.CodexAutoContextWindow(rule))
}

// With smart routing on, every route's services count too; off, they don't.
func TestCodexAutoContextWindow_SmartRoutes(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, true)
	rule := codexRule("smart", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-v4-flash"))
	rule.SmartRouting = []smartrouting.SmartRouting{{Description: "short", Services: []*loadbalance.Service{codexSvc(p.deepseek, "deepseek-chat")}}}

	assert.Equal(t, 1000000, cfg.CodexAutoContextWindow(rule).Window, "routes are ignored while smart routing is off")
	rule.SmartEnabled = true
	assert.Equal(t, CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceCatalog},
		cfg.CodexAutoContextWindow(rule))
}

// Without a provider catalog nothing is known: every upstream is judged like an
// unknown model, and RuleModelInfo has nothing to say.
func TestCodexAutoContextWindow_NoCatalog(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, false)
	rule := codexRule("sol", typ.RuleFlags{}, codexSvc(p.chatgpt, "gpt-6-sol"))
	assert.Equal(t, CodexContextWindow{Window: 200000, Max: 200000, Source: CodexWindowSourceDefault}, cfg.CodexAutoContextWindow(rule))
	_, ok := cfg.RuleModelInfo(rule, ai.APIStyleOpenAI)
	assert.False(t, ok)
}

// The rule's own window wins and is also its ceiling.
func TestCodexContextWindowFor_OwnWindow(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, true)
	sol := codexSvc(p.chatgpt, "gpt-6-sol")

	assert.Equal(t, CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceRule},
		cfg.CodexContextWindowFor(codexRule("sol", typ.RuleFlags{CodexContextWindow: 128000}, sol)))
	// Also on a model the catalog doesn't know: the default ceiling is a guess.
	house := codexSvc(p.house, "house-model")
	assert.Equal(t, CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceRule},
		cfg.CodexContextWindowFor(codexRule("house", typ.RuleFlags{CodexContextWindow: 128000, Context1M: true}, house)))
	assert.Equal(t, CodexContextWindow{Window: 1000000, Max: 1000000, Source: CodexWindowSourceRule},
		cfg.CodexContextWindowFor(codexRule("sol", typ.RuleFlags{CodexContextWindow: 1000000}, sol)))
	// Out of range is ignored.
	assert.Equal(t, CodexContextWindow{Window: 272000, Max: 872000, Source: CodexWindowSourceCatalog},
		cfg.CodexContextWindowFor(codexRule("sol", typ.RuleFlags{CodexContextWindow: 100}, sol)))
}

// The maps the catalog renderer and the status check read: default windows are
// left out, ceilings only appear above the window, inactive and non-Codex
// rules are skipped, and two rules sharing a request model keep the smaller.
func TestBuildContextWindowsFromRules_Maps(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, true)
	inactive := codexRule("inactive", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-chat"))
	inactive.Active = false
	claude := codexRule("cc-model", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-chat"))
	claude.Scenario = typ.ScenarioClaudeCode
	twin := codexRule("shared", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-chat"))
	twin.UUID = "r-shared-2"
	cfg.Rules = []typ.Rule{
		codexRule("sol", typ.RuleFlags{}, codexSvc(p.chatgpt, "gpt-6-sol")),
		codexRule("ds", typ.RuleFlags{Context1M: true}, codexSvc(p.deepseek, "deepseek-chat")),
		codexRule("house", typ.RuleFlags{}, codexSvc(p.house, "house-model")),
		codexRule(" padded ", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-v4-flash")),
		codexRule("shared", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-v4-flash")),
		twin, inactive, claude,
	}

	assert.Equal(t, map[string]int{"sol": 272000, "ds": 128000, "padded": 1000000, "shared": 128000}, BuildContextWindowsFromRules(cfg))
	assert.Equal(t, map[string]int{"sol": 872000}, BuildMaxContextWindowsFromRules(cfg))
	assert.Equal(t, CodexContextWindow{Window: 200000, Max: 200000, Source: CodexWindowSourceDefault},
		ResolveCodexContextWindows(cfg)["house"])
	assert.Empty(t, ResolveCodexContextWindows(nil))

	// An unknown model at 1M is a guess, but still not the default window.
	cfg.Rules = []typ.Rule{codexRule("house", typ.RuleFlags{Context1M: true}, codexSvc(p.house, "house-model"))}
	assert.Equal(t, map[string]int{"house": 1000000}, BuildContextWindowsFromRules(cfg))
}

// Two rules sharing a request model share one catalog entry: the smaller
// window and the smaller ceiling, in either order.
func TestResolveCodexContextWindows_SharedModel(t *testing.T) {
	cfg, p := newCodexWindowConfig(t, true)
	sol := codexRule("dup", typ.RuleFlags{}, codexSvc(p.chatgpt, "gpt-6-sol"))
	pinned := codexRule("dup", typ.RuleFlags{CodexContextWindow: 272000}, codexSvc(p.chatgpt, "gpt-6-sol"))
	pinned.UUID = "r-dup-2"
	for _, rules := range [][]typ.Rule{{sol, pinned}, {pinned, sol}} {
		cfg.Rules = rules
		got := ResolveCodexContextWindows(cfg)["dup"]
		assert.Equal(t, [2]int{272000, 272000}, [2]int{got.Window, got.Max})
	}
	ds := codexRule("dup", typ.RuleFlags{}, codexSvc(p.deepseek, "deepseek-chat"))
	ds.UUID = "r-dup-3"
	for _, rules := range [][]typ.Rule{{sol, ds}, {ds, sol}} {
		cfg.Rules = rules
		assert.Equal(t, CodexContextWindow{Window: 128000, Max: 128000, Source: CodexWindowSourceCatalog},
			ResolveCodexContextWindows(cfg)["dup"])
	}
}

// The rendered catalog carries the window and the ceiling separately.
func TestRenderCodexModelCatalog_MaxContextWindow(t *testing.T) {
	prefs := DefaultCodexPrefs()
	prefs.ModelMaxContextWindow = map[string]int{"sol": 872000, "low": 100000}
	raw, err := RenderCodexModelCatalogWithPrefs([]string{"sol", "ds", "low", "plain"},
		map[string]int{"sol": 272000, "ds": 128000, "low": 128000}, prefs)
	require.NoError(t, err)
	got := map[string][3]int{}
	for _, m := range decodeCodexCatalog(t, raw) {
		got[m.Slug] = [3]int{m.ContextWindow, m.MaxContextWindow, m.AutoCompactTokenLimit}
	}
	assert.Equal(t, map[string][3]int{
		"sol":   {272000, 872000, 204000},
		"ds":    {128000, 128000, 96000},
		"low":   {128000, 128000, 96000}, // a ceiling below the window is ignored
		"plain": {200000, 200000, 150000},
	}, got)
}

type codexCatalogEntry struct {
	Slug                  string `json:"slug"`
	ContextWindow         int    `json:"context_window"`
	MaxContextWindow      int    `json:"max_context_window"`
	AutoCompactTokenLimit int    `json:"auto_compact_token_limit"`
}

func decodeCodexCatalog(t *testing.T, raw []byte) []codexCatalogEntry {
	t.Helper()
	var c struct {
		Models []codexCatalogEntry `json:"models"`
	}
	require.NoError(t, json.Unmarshal(raw, &c))
	return c.Models
}
