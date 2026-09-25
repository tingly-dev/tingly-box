package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Expectations are live captures of the official binary (interactive
// persona; -p drops redact-thinking). See .design/claude-code.md Part B.

func TestComposeClaudeCodeBetas_Sonnet46OAuthCapture(t *testing.T) {
	got := composeClaudeCodeBetas(claudeBetaSignals{
		Model:      "claude-sonnet-4-6",
		OAuth:      true,
		EffortSet:  true,
		CacheTTL1h: true,
	})
	assert.Equal(t, "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24,extended-cache-ttl-2025-04-11", joinBetas(got))
}

func TestComposeClaudeCodeBetas_APIKeyHasNoOAuth(t *testing.T) {
	got := composeClaudeCodeBetas(claudeBetaSignals{Model: "claude-sonnet-4-6", EffortSet: true})
	assert.Equal(t, "claude-code-20250219,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24", joinBetas(got))
}

func TestComposeClaudeCodeBetas_ModelFamilies(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		// haiku: no claude-code flag, haiku-4-5 has no interleaved thinking
		{"claude-haiku-4-5-20251001", "oauth-2025-04-20,context-management-2025-06-27,prompt-caching-scope-2026-01-05"},
		// claude-3 family: nothing model-gated survives
		{"claude-3-5-haiku-20241022", "oauth-2025-04-20,prompt-caching-scope-2026-01-05"},
		// opus 4.6: same shape as sonnet 4.6
		{"claude-opus-4-6", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05"},
		// 5-series: mid-conversation-system joins the baseline
		{"claude-sonnet-5", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,mid-conversation-system-2026-04-07"},
		{"claude-fable-5-1", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,mid-conversation-system-2026-04-07"},
		// [1m] marker and snapshot dates are ignored for capability checks
		{"claude-opus-4-6[1m]", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := composeClaudeCodeBetas(claudeBetaSignals{Model: tt.model, OAuth: true})
			assert.Equal(t, tt.want, joinBetas(got))
		})
	}
}

func TestComposeClaudeCodeBetas_Context1MSitsAfterOAuth(t *testing.T) {
	got := composeClaudeCodeBetas(claudeBetaSignals{Model: "claude-sonnet-4-6", OAuth: true, Context1M: true})
	assert.Equal(t, []string{betaClaudeCode, betaOAuth, betaContext1M, betaInterleavedThinking}, got[:4])
}

// Direct first-party capture: thinking adaptive, ToolSearch + deferred tools,
// diagnostics in the body.
func TestComposeClaudeCodeBetas_DirectCapture(t *testing.T) {
	got := composeClaudeCodeBetas(claudeBetaSignals{
		Model:          "claude-sonnet-4-6",
		OAuth:          true,
		EffortSet:      true,
		CacheTTL1h:     true,
		ToolSearch:     true,
		ThinkingActive: true,
		Diagnostics:    true,
	})
	assert.Equal(t, "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advanced-tool-use-2025-11-20,effort-2025-11-24,thinking-binding-controls-2026-08-01,extended-cache-ttl-2025-04-11,cache-diagnosis-2026-04-07", joinBetas(got))
}

// Replayed client flags land in the CLI's emission order.
func TestComposeClaudeCodeBetas_ReplayedFlagsInEmissionOrder(t *testing.T) {
	client := []string{"timing-2026-09-09", "inline-tools-2026-09-15", "mid-conversation-system-clear-at-2026-08-21", "dangerous-tool-use-2026-09-03", "thinking-binding-controls-2026-08-01", "thinking-resumption-2026-07-17", "message-threads-2026-08-12", "per-turn-control-2026-07-01"}
	sig := claudeBetaSignals{Model: "claude-sonnet-4-6", OAuth: true, ThinkingActive: true, Diagnostics: true, ClientBetas: client}
	assert.Equal(t, "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,per-turn-control-2026-07-01,timing-2026-09-09,inline-tools-2026-09-15,mid-conversation-system-clear-at-2026-08-21,dangerous-tool-use-2026-09-03,thinking-binding-controls-2026-08-01,thinking-resumption-2026-07-17,cache-diagnosis-2026-04-07,message-threads-2026-08-12", joinBetas(composeClaudeCodeBetas(sig)))
}

func TestComposeClaudeCodeBetas_BodyDerivedFlagsInEmissionOrder(t *testing.T) {
	got := composeClaudeCodeBetas(claudeBetaSignals{
		Model:                  "claude-sonnet-4-6",
		OAuth:                  true,
		EffortSet:              true,
		FormatSet:              true,
		TaskBudgetSet:          true,
		FastMode:               true,
		ThinkingDisplayUpdates: true,
		CacheTTL1h:             true,
		ToolSearch:             true,
	})
	tail := got[7:]
	assert.Equal(t, []string{
		betaAdvancedToolUse, betaEffort, betaTaskBudgets, betaStructuredOutputs, betaThinkingDisplayUpdates,
		betaFastMode, betaExtendedCacheTTL,
	}, tail)
}

func TestComposeClaudeCodeBetas_ClientReplayIsAllowlisted(t *testing.T) {
	got := composeClaudeCodeBetas(claudeBetaSignals{
		Model: "claude-sonnet-4-6",
		OAuth: true,
		ClientBetas: []string{
			"per-turn-control-2026-07-01", // replayable
			"afk-mode-2026-01-31",         // replayable
			"message-batches-2024-09-24",  // SDK flag no CLI sends: dropped
			"claude-code-20250219",        // baseline flag: deduped, not repeated
			"totally-made-up-2026-01-01",  // unknown: dropped
			" fast-mode-2026-02-01 ",      // whitespace tolerated
		},
	})
	assert.Equal(t, "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,per-turn-control-2026-07-01,fast-mode-2026-02-01,afk-mode-2026-01-31", joinBetas(got))
}

func TestFilterClaudeCodeCountTokensBetas(t *testing.T) {
	all := composeClaudeCodeBetas(claudeBetaSignals{Model: "claude-sonnet-4-6", OAuth: true, EffortSet: true, Context1M: true})
	got := filterClaudeCodeCountTokensBetas(all)
	assert.Equal(t, "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27", joinBetas(got))
}

func TestNormalizeClaudeModel(t *testing.T) {
	assert.Equal(t, "claude-haiku-4-5", normalizeClaudeModel("claude-haiku-4-5-20251001"))
	assert.Equal(t, "claude-opus-4-6", normalizeClaudeModel("Claude-Opus-4-6[1m]"))
	assert.Equal(t, "claude-sonnet-4-6", normalizeClaudeModel(" claude-sonnet-4-6 "))
	assert.Equal(t, "claude-3-5-haiku", normalizeClaudeModel("claude-3-5-haiku-20241022"))
}

func TestBetaClaudeBetaSignals_BodyFields(t *testing.T) {
	var req anthropic.BetaMessageNewParams
	body := `{"model":"claude-sonnet-4-6","max_tokens":1,"messages":[],"thinking":{"type":"adaptive","display":"omitted"},"diagnostics":{"previous_message_id":null},"tools":[{"name":"ToolSearch","input_schema":{"type":"object"}},{"name":"Read","input_schema":{"type":"object"},"defer_loading":true}]}`
	require.NoError(t, json.Unmarshal([]byte(body), &req))
	ctx := context.Background()
	sig := betaClaudeBetaSignals(ctx, &req, true)
	assert.True(t, sig.ThinkingActive)
	assert.True(t, sig.Diagnostics)
	assert.True(t, sig.ToolSearch)

	var plain anthropic.BetaMessageNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"claude-sonnet-4-6","max_tokens":1,"messages":[],"thinking":{"type":"disabled"}}`), &plain))
	sig = betaClaudeBetaSignals(ctx, &plain, true)
	assert.False(t, sig.ThinkingActive)
	assert.False(t, sig.Diagnostics)
	assert.False(t, sig.ToolSearch)
}

func TestBetaClaudeBetaSignals_ReadsBody(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model: "claude-sonnet-4-6",
		System: []anthropic.BetaTextBlockParam{
			{Text: "sys", CacheControl: anthropic.BetaCacheControlEphemeralParam{TTL: anthropic.BetaCacheControlEphemeralTTLTTL1h}},
		},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortHigh},
		Speed:        anthropic.BetaMessageNewParamsSpeed("fast"),
		Thinking: anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{
			Display: anthropic.BetaThinkingConfigAdaptiveDisplay("updates"),
		}},
		Tools: []anthropic.BetaToolUnionParam{
			anthropic.BetaToolUnionParamOfToolSearchToolRegex20251119(""),
		},
	}
	ctx := typ.WithRuleFlags(context.Background(), typ.RuleFlags{Context1M: true})
	ctx = typ.WithClaudeCodeClientHints(ctx, typ.ClaudeCodeClientHints{Betas: []string{"afk-mode-2026-01-31"}})

	sig := betaClaudeBetaSignals(ctx, req, true)
	assert.Equal(t, "claude-sonnet-4-6", sig.Model)
	assert.True(t, sig.OAuth)
	assert.True(t, sig.Context1M)
	assert.True(t, sig.EffortSet)
	assert.False(t, sig.FormatSet)
	assert.False(t, sig.TaskBudgetSet)
	assert.True(t, sig.FastMode)
	assert.True(t, sig.ThinkingDisplayUpdates)
	assert.True(t, sig.CacheTTL1h)
	assert.True(t, sig.ToolSearch)
	assert.Equal(t, []string{"afk-mode-2026-01-31"}, sig.ClientBetas)
}

func TestV1ClaudeBetaSignals_CacheTTLOnMessageBlock(t *testing.T) {
	block := anthropic.NewTextBlock("hello")
	block.OfText.CacheControl = anthropic.CacheControlEphemeralParam{TTL: anthropic.CacheControlEphemeralTTLTTL1h}
	req := &anthropic.MessageNewParams{
		Model:    "claude-sonnet-4-6",
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(block)},
	}
	sig := v1ClaudeBetaSignals(context.Background(), req, false)
	assert.True(t, sig.CacheTTL1h)
	assert.False(t, sig.OAuth)
	assert.False(t, sig.EffortSet)
}

func TestSanitizeClaudeHeaderValue(t *testing.T) {
	assert.Equal(t, "agent-1@abc", sanitizeClaudeHeaderValue("agent-1@abc"))
	assert.Equal(t, "a%25b", sanitizeClaudeHeaderValue("a%b"))
	assert.Equal(t, "x%0Ay", sanitizeClaudeHeaderValue("x\ny"))
	assert.Equal(t, "%C3%A9", sanitizeClaudeHeaderValue("é"))
}

// ---------------------------------------------------------------------------
// Wire-level: what actually leaves the Claude OAuth chain
// ---------------------------------------------------------------------------

func newCapturingAnthropicServer(t *testing.T, capture *http.Header) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*capture = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content":     []map[string]any{{"type": "text", "text": "hi"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
}

// newTestClaudeClient builds a Claude OAuth client on the native profile,
// keeping the other rule flags on ctx.
func newTestClaudeClient(t *testing.T, ctx context.Context, apiBase string) *ClaudeClient {
	t.Helper()
	flags := typ.GetRuleFlags(ctx)
	flags.ClaudeCodeVersion = typ.ClaudeCodeVersionLatest
	ctx = typ.WithRuleFlags(ctx, flags)
	provider := &typ.Provider{
		Name:     "test-claude",
		APIBase:  apiBase,
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "sk-ant-oat01-testtoken",
		},
	}
	c, err := NewClaudeClient(ctx, provider, "claude-sonnet-4-6", typ.SessionID{Value: "sess"})
	require.NoError(t, err)
	return c
}

func betaRequestWithMetadata() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 16,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello"))},
		Metadata: anthropic.BetaMetadataParam{
			UserID: param.NewOpt(`{"device_id":"d","account_uuid":"a","session_id":"11111111-2222-3333-4444-555555555555"}`),
		},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortHigh},
	}
}

func TestClaudeClient_WireHeaders(t *testing.T) {
	var captured http.Header
	srv := newCapturingAnthropicServer(t, &captured)
	defer srv.Close()

	ctx := typ.WithRuleFlags(context.Background(), typ.RuleFlags{Context1M: true})
	ctx = typ.WithClaudeCodeClientHints(ctx, typ.ClaudeCodeClientHints{
		Betas:         []string{"per-turn-control-2026-07-01", "message-batches-2024-09-24"},
		AgentID:       "agent-7",
		ParentAgentID: "agent-main",
	})
	c := newTestClaudeClient(t, ctx, srv.URL)

	req := betaRequestWithMetadata()
	req.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaMessageBatches2024_09_24} // must not leak
	_, err := c.BetaMessagesNew(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, captured)

	// One anthropic-beta header value, composed, context-1m folded in once,
	// client per-turn-control replayed, client message-batches dropped.
	betas := captured.Values("Anthropic-Beta")
	require.Len(t, betas, 1, "anthropic-beta must be a single header value, got %v", betas)
	assert.Equal(t, "claude-code-20250219,oauth-2025-04-20,context-1m-2025-08-07,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,per-turn-control-2026-07-01,effort-2025-11-24", betas[0])
	assert.Equal(t, 1, strings.Count(betas[0], "context-1m-2025-08-07"))

	assert.Equal(t, "claude-cli/2.1.280 (external, cli)", captured.Get("User-Agent"))
	assert.Equal(t, "cli", captured.Get("X-App"))
	assert.Equal(t, "main", captured.Get("X-Claude-Code-Request-Class"))
	assert.Equal(t, "11111111-2222-3333-4444-555555555555", captured.Get("X-Claude-Code-Session-Id"))
	assert.Equal(t, "Bearer sk-ant-oat01-testtoken", captured.Get("Authorization"))
	assert.Equal(t, "true", captured.Get("Anthropic-Dangerous-Direct-Browser-Access"))
	assert.Equal(t, "2023-06-01", captured.Get("Anthropic-Version"))
	assert.Equal(t, "0.112.1", captured.Get("X-Stainless-Package-Version"))
	assert.Equal(t, "v26.3.0", captured.Get("X-Stainless-Runtime-Version"))
	assert.Equal(t, "node", captured.Get("X-Stainless-Runtime"))
	assert.Equal(t, "js", captured.Get("X-Stainless-Lang"))
	assert.Equal(t, "0", captured.Get("X-Stainless-Retry-Count"))
	assert.Equal(t, "600", captured.Get("X-Stainless-Timeout"))
	assert.Equal(t, stainlessOSName(runtime.GOOS), captured.Get("X-Stainless-Os"))
	assert.Equal(t, stainlessArchName(runtime.GOARCH), captured.Get("X-Stainless-Arch"))
	assert.Empty(t, captured.Get("X-Stainless-Helper-Method"), "the CLI does not use the .stream() helper")
	assert.Equal(t, "agent-7", captured.Get("X-Claude-Code-Agent-Id"))
	assert.Equal(t, "agent-main", captured.Get("X-Claude-Code-Parent-Agent-Id"))
	assert.Empty(t, captured.Get("Anthropic-Organization-Id"))
}

func TestClaudeClient_WireHeaders_NoHintsNoAgentHeaders(t *testing.T) {
	var captured http.Header
	srv := newCapturingAnthropicServer(t, &captured)
	defer srv.Close()

	ctx := context.Background()
	c := newTestClaudeClient(t, ctx, srv.URL)
	_, err := c.BetaMessagesNew(ctx, betaRequestWithMetadata())
	require.NoError(t, err)

	assert.Empty(t, captured.Get("X-Claude-Code-Agent-Id"))
	assert.Empty(t, captured.Get("X-Claude-Code-Parent-Agent-Id"))
	assert.Equal(t, "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24", captured.Get("Anthropic-Beta"))
}

func TestClaudeClient_CountTokensBetaSubset(t *testing.T) {
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens": 3}`))
	}))
	defer srv.Close()

	ctx := typ.WithRuleFlags(context.Background(), typ.RuleFlags{Context1M: true})
	c := newTestClaudeClient(t, ctx, srv.URL)
	_, err := c.BetaMessagesCountTokens(ctx, &anthropic.BetaMessageCountTokensParams{
		Model:    "claude-sonnet-4-6",
		Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello"))},
		Betas:    []anthropic.AnthropicBeta{anthropic.AnthropicBetaMessageBatches2024_09_24},
	})
	require.NoError(t, err)
	betas := captured.Values("Anthropic-Beta")
	require.Len(t, betas, 1)
	assert.Equal(t, "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27", betas[0])
}

// With claude_code_version unset the chain is the legacy 2.1.86 emulation,
// byte-for-byte.
func TestClaudeClient_LegacyProfileUnchanged(t *testing.T) {
	var captured http.Header
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": []map[string]any{{"type": "text", "text": "hi"}}, "stop_reason": "end_turn",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	ctx := typ.WithClaudeCodeClientHints(context.Background(), typ.ClaudeCodeClientHints{AgentID: "agent-7", BackgroundSession: true})
	provider := &typ.Provider{
		Name: "legacy", APIBase: srv.URL, AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{AccessToken: "sk-ant-oat01-testtoken"},
	}
	c, err := NewClaudeClient(ctx, provider, "claude-sonnet-4-6", typ.SessionID{Value: "sess"})
	require.NoError(t, err)
	require.False(t, c.native)

	req := betaRequestWithMetadata()
	req.System = []anthropic.BetaTextBlockParam{
		{Text: "x-anthropic-billing-header: cc_version=2.1.86.abc; cc_entrypoint=cli; cch=00000;"},
		{Text: "<system-reminder>x</system-reminder>"},
	}
	_, err = c.BetaMessagesNew(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, claudeCLIUserAgent, captured.Get("User-Agent"))
	assert.Equal(t, "claude-cli/2.1.86 (external, cli)", captured.Get("User-Agent"))
	assert.Equal(t, stainlessHelperMethod, captured.Get("X-Stainless-Helper-Method"))
	assert.Equal(t, stainlessPackageVersion, captured.Get("X-Stainless-Package-Version"))
	assert.Equal(t, stainlessRuntimeVersion, captured.Get("X-Stainless-Runtime-Version"))
	assert.Equal(t, anthropicBeta, captured.Get("Anthropic-Beta"))
	assert.Empty(t, captured.Get("X-Claude-Code-Agent-Id"))
	assert.Equal(t, "cli", captured.Get("X-App"), "legacy chain ignores the background-session hint")
	assert.Contains(t, string(body), "cch=00000;", "legacy chain does not hash cch")
	assert.Contains(t, string(body), `\u003csystem-reminder\u003e`, "legacy chain keeps Go's JSON escaping")
}

func TestClaudeClient_WireHintHeaders(t *testing.T) {
	var captured http.Header
	srv := newCapturingAnthropicServer(t, &captured)
	defer srv.Close()

	ctx := typ.WithClaudeCodeClientHints(context.Background(), typ.ClaudeCodeClientHints{
		Betas:        []string{"thinking-resumption-2026-07-17"},
		RequestClass: "subagent",
		AgentType:    "explore",
	})
	c := newTestClaudeClient(t, ctx, srv.URL)

	req := betaRequestWithMetadata()
	req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}
	_, err := c.BetaMessagesNew(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, "claude-cli/2.1.280 (external, cli)", captured.Get("User-Agent"))
	assert.Equal(t, "0.112.1", captured.Get("X-Stainless-Package-Version"))
	assert.Equal(t, "v26.3.0", captured.Get("X-Stainless-Runtime-Version"))
	assert.Equal(t, "subagent", captured.Get("X-Claude-Code-Request-Class"))
	assert.Equal(t, "explore", captured.Get("X-Claude-Code-Agent-Type"))
	betas := captured.Values("Anthropic-Beta")
	require.Len(t, betas, 1)
	assert.Equal(t, "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24,thinking-binding-controls-2026-08-01,thinking-resumption-2026-07-17", betas[0])

	// Invalid class falls back to the main thread; no agent type.
	ctx2 := typ.WithClaudeCodeClientHints(context.Background(), typ.ClaudeCodeClientHints{RequestClass: "Bad Class"})
	c2 := newTestClaudeClient(t, ctx2, srv.URL)
	_, err = c2.BetaMessagesNew(ctx2, betaRequestWithMetadata())
	require.NoError(t, err)
	assert.Equal(t, "main", captured.Get("X-Claude-Code-Request-Class"))
	assert.Empty(t, captured.Get("X-Claude-Code-Agent-Type"))
}

// A background session (x-app: cli-bg) keeps its kind upstream on the native
// profile; the interactive default sends "cli".
func TestClaudeClient_XAppBackgroundSession(t *testing.T) {
	var captured http.Header
	srv := newCapturingAnthropicServer(t, &captured)
	defer srv.Close()

	ctx := typ.WithClaudeCodeClientHints(context.Background(), typ.ClaudeCodeClientHints{BackgroundSession: true})
	c := newTestClaudeClient(t, ctx, srv.URL)
	_, err := c.BetaMessagesNew(ctx, betaRequestWithMetadata())
	require.NoError(t, err)
	assert.Equal(t, "cli-bg", captured.Get("X-App"))

	ctx2 := context.Background()
	c2 := newTestClaudeClient(t, ctx2, srv.URL)
	_, err = c2.BetaMessagesNew(ctx2, betaRequestWithMetadata())
	require.NoError(t, err)
	assert.Equal(t, "cli", captured.Get("X-App"))
}

// The v1 Messages path derives the tool-search beta like the beta path.
func TestV1ClaudeBetaSignals_ToolSearch(t *testing.T) {
	cases := map[string]string{
		"defer_loading":   `[{"name":"Read","input_schema":{"type":"object"},"defer_loading":true}]`,
		"ToolSearch tool": `[{"name":"ToolSearch","input_schema":{"type":"object"}}]`,
		"regex search":    `[{"type":"tool_search_tool_regex_20251119","name":"tool_search_tool_regex"}]`,
	}
	for name, tools := range cases {
		t.Run(name, func(t *testing.T) {
			var req anthropic.MessageNewParams
			require.NoError(t, json.Unmarshal([]byte(`{"model":"claude-sonnet-4-6","max_tokens":1,"messages":[],"tools":`+tools+`}`), &req))
			assert.True(t, v1ClaudeBetaSignals(context.Background(), &req, true).ToolSearch)
		})
	}
	var plain anthropic.MessageNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"claude-sonnet-4-6","max_tokens":1,"messages":[],"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`), &plain))
	assert.False(t, v1ClaudeBetaSignals(context.Background(), &plain, true).ToolSearch)
}
