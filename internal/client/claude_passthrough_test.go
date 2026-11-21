package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Claude Code members the SDK's typed params drop must follow their beta
// upstream, and only with it (.research/claude-code/2.1.328.md §6 G2).

const passthroughBody = `{"model":"claude-opus-4-8","max_tokens":64,"stream":false,
"metadata":{"user_id":"{\"device_id\":\"d\",\"account_uuid\":\"a\",\"session_id\":\"11111111-2222-3333-4444-555555555555\"}"},
"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral","ttl":"1h"}}],
"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],
"cache_control":{"type":"ephemeral","ttl":"1h","evict_on_complete":true},
"thinking":{"type":"adaptive","display":"highlights","block_binding":{"prefix_mismatch_behavior":"error"}},
"output_config":{"effort":"high","task_budget":{"type":"tokens","total":1000}},
"diagnostics":{"previous_message_id":null},
"resumption":{"type":"enabled"},
"program_use_case_id":"puc_1",
"thread":{"id":"thr_1"},
"fallback_credit_token":"fct_123",
"safeguards":[{"type":"auto_mode"}],
"tags":{"team":"a"}}`

// sendPassthrough decodes body like the server, applies the extras the way
// applyClaudeCodeClientHints does, and returns the upstream body and betas.
func sendPassthrough(t *testing.T, body string, inboundBetas []string) (upstream []byte, betas string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream, _ = io.ReadAll(r.Body)
		betas = r.Header.Get("anthropic-beta")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "m", "type": "message", "role": "assistant", "model": "x",
			"content": []map[string]any{{"type": "text", "text": "hi"}}, "stop_reason": "end_turn",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1}})
	}))
	t.Cleanup(srv.Close)

	var req protocol.AnthropicBetaMessagesRequest
	require.NoError(t, json.Unmarshal([]byte(body), &req))
	ctx := typ.WithClaudeCodeClientHints(context.Background(), typ.ClaudeCodeClientHints{
		Betas:      inboundBetas,
		BodyExtras: typ.ExtractClaudeCodeBodyExtras([]byte(body)),
	})
	c := newTestClaudeClient(t, ctx, srv.URL)
	_, err := c.BetaMessagesNew(ctx, req.BetaMessageNewParams)
	require.NoError(t, err)
	return upstream, betas
}

func TestClaudeClient_PassThroughFollowsBeta(t *testing.T) {
	up, betas := sendPassthrough(t, passthroughBody, []string{
		betaResumption, betaProgramUseCaseIDs, betaMessageThreads, betaPromptCachingEvict, betaFallbackCredit,
		"request-tags-2026-08-21", // gateway-only: dropped together with tags
	})

	for _, b := range []string{betaResumption, betaProgramUseCaseIDs, betaMessageThreads, betaPromptCachingEvict, betaFallbackCredit} {
		assert.Contains(t, betas, b)
	}
	assert.NotContains(t, betas, "request-tags", "request tags never reach the upstream")

	assert.JSONEq(t, `{"type":"enabled"}`, gjson.GetBytes(up, "resumption").Raw)
	assert.Equal(t, "puc_1", gjson.GetBytes(up, "program_use_case_id").String())
	assert.Equal(t, "thr_1", gjson.GetBytes(up, "thread.id").String())
	assert.True(t, gjson.GetBytes(up, "cache_control.evict_on_complete").Bool())
	assert.Equal(t, "1h", gjson.GetBytes(up, "cache_control.ttl").String())
	assert.Equal(t, "fct_123", gjson.GetBytes(up, "fallback_credit_token").String(), "typed field, paired with fallback-credit")

	// Already carried by the typed params: must keep surviving.
	assert.Equal(t, "highlights", gjson.GetBytes(up, "thinking.display").String())
	assert.Equal(t, "error", gjson.GetBytes(up, "thinking.block_binding.prefix_mismatch_behavior").String())
	assert.Equal(t, int64(1000), gjson.GetBytes(up, "output_config.task_budget.total").Int())
	assert.True(t, gjson.GetBytes(up, "diagnostics").Exists())

	// Not replayed: tags are gateway-only, safeguards need a server classifier.
	assert.False(t, gjson.GetBytes(up, "tags").Exists())
	assert.False(t, gjson.GetBytes(up, "safeguards").Exists())
}

func TestClaudeClient_PassThroughWithoutBetaDropsField(t *testing.T) {
	// The inbound client sent the members but not their betas (a client that
	// is not really Claude Code): neither half may leak upstream.
	up, betas := sendPassthrough(t, passthroughBody, nil)
	for _, b := range []string{betaResumption, betaProgramUseCaseIDs, betaMessageThreads, betaPromptCachingEvict} {
		assert.NotContains(t, betas, b)
	}
	for _, f := range []string{"resumption", "program_use_case_id", "thread", "cache_control.evict_on_complete"} {
		assert.False(t, gjson.GetBytes(up, f).Exists(), f)
	}
	assert.Equal(t, "1h", gjson.GetBytes(up, "cache_control.ttl").String(), "typed members are untouched")
}

func TestExtractClaudeCodeBodyExtras(t *testing.T) {
	got := typ.ExtractClaudeCodeBodyExtras([]byte(passthroughBody))
	var names []string
	for k := range got {
		names = append(names, k)
	}
	assert.ElementsMatch(t, []string{"resumption", "program_use_case_id", "thread", "cache_control"}, names)
	assert.Nil(t, typ.ExtractClaudeCodeBodyExtras([]byte(`{"model":"m","resumption":null}`)))
	assert.Nil(t, typ.ExtractClaudeCodeBodyExtras([]byte(`not json`)))
	assert.False(t, strings.Contains(string(got["cache_control"]), "tags"))
}
