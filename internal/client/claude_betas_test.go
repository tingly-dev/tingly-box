package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// The expectations below are the anthropic-beta headers captured from the
// official 2.1.258 binary (interactive persona; the -p capture differs only
// by redact-thinking, which the CLI drops in non-interactive mode). See
// .design/claude-code-client-compat.md §3.

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
		betaEffort, betaTaskBudgets, betaStructuredOutputs, betaThinkingDisplayUpdates,
		betaFastMode, betaExtendedCacheTTL, betaAdvancedToolUse,
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

func newTestClaudeClient(t *testing.T, ctx context.Context, apiBase string) *ClaudeClient {
	t.Helper()
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

	assert.Equal(t, "claude-cli/2.1.258 (external, cli)", captured.Get("User-Agent"))
	assert.Equal(t, "cli", captured.Get("X-App"))
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
	assert.Equal(t, stainlessOS(), captured.Get("X-Stainless-Os"))
	assert.Equal(t, stainlessArch(), captured.Get("X-Stainless-Arch"))
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
