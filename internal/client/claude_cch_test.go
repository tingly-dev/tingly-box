package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reference vectors computed with an independent Python implementation of
// xxHash64 (Bun/Zig PRIME64_4) whose output matched three live cch values
// captured from the official 2.1.258 binary; its standard-prime mode
// reproduces the canonical xxh64("") = ef46db3751d8e999.
func TestXXHash64Zig_Vectors(t *testing.T) {
	tests := []struct {
		in   string
		want uint64
	}{
		{"", 0xb8b30e7de65b46c5},
		{"a", 0xde28ae28b0e07bb2},
		{"abc", 0xdfc4f4d6913699b6},
		{"0123456789abcdef0123", 0x9add911504d2928b},             // 20 bytes: 8+8+4 tail
		{"0123456789abcdef0123456789abcdef", 0x7f82b050ec994b30}, // exactly one 32-byte stripe
		{"The quick brown fox jumps over the lazy dog", 0xa5705e22f779a6d9},
		{strings.Repeat("x", 100), 0x6070011b77a4bb8e},
	}
	for _, tt := range tests {
		t.Run(tt.in[:min(len(tt.in), 12)], func(t *testing.T) {
			assert.Equal(t, tt.want, xxhash64Zig([]byte(tt.in), claudeCodeCCHSeed))
		})
	}
	assert.Equal(t, "b46c5", formatClaudeCodeCCH(0xb8b30e7de65b46c5))
}

func TestClaudeCodeCCHPreimage_TopLevelEdits(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			name: "model first, max_tokens in the middle",
			in:   `{"model":"claude-sonnet-4-6","max_tokens":32000,"messages":[{"role":"user","content":"hi"}],"stream":true}`,
			want: `{"model":"","messages":[{"role":"user","content":"hi"}],"stream":true}`,
		},
		{
			name: "max_tokens last (preceding comma dropped)",
			in:   `{"messages":[],"model":"claude-x","stream":true,"max_tokens":5}`,
			want: `{"messages":[],"model":"","stream":true}`,
		},
		{
			name: "max_tokens first",
			in:   `{"max_tokens":5,"model":"claude-x"}`,
			want: `{"model":""}`,
		},
		{
			name: "only max_tokens",
			in:   `{"max_tokens":5}`,
			want: `{}`,
		},
		{
			name: "nested model/max_tokens keys are not top-level and stay",
			in:   `{"model":"m","tools":[{"input_schema":{"properties":{"model":{"type":"string"},"max_tokens":{"type":"integer"}}}}],"max_tokens":1}`,
			want: `{"model":"","tools":[{"input_schema":{"properties":{"model":{"type":"string"},"max_tokens":{"type":"integer"}}}}]}`,
		},
		{
			name: "keys inside strings with escaped quotes do not confuse the scanner",
			in:   `{"model":"a\"b","messages":[{"content":"say \"max_tokens\": 3, \"model\": x"}],"max_tokens":9}`,
			want: `{"model":"","messages":[{"content":"say \"max_tokens\": 3, \"model\": x"}]}`,
		},
		{
			name: "whitespace tolerated",
			in:   "{ \"model\" : \"m\" ,\n \"max_tokens\" : 3 , \"stream\" : true }",
			want: "{ \"model\" : \"\" ,\n  \"stream\" : true }",
		},
		{
			name: "not an object: unchanged",
			in:   `[1,2,3]`,
			want: `[1,2,3]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(claudeCodeCCHPreimage([]byte(tt.in))))
		})
	}
}

func TestCanonicalizeJSONEscapes(t *testing.T) {
	in := `{"t":"a \u003cb\u003e \u0026 c \u2028 d \u2029 e \\u003c literal \u00e9 \n"}`
	want := "{\"t\":\"a <b> & c \u2028 d \u2029 e \\\\u003c literal \\u00e9 \\n\"}"
	got := string(canonicalizeJSONEscapes([]byte(in)))
	assert.Equal(t, want, got)
	// Semantics preserved.
	var a, b map[string]string
	require.NoError(t, json.Unmarshal([]byte(in), &a))
	require.NoError(t, json.Unmarshal([]byte(got), &b))
	assert.Equal(t, a, b)
	// No escapes: same bytes back.
	plain := []byte(`{"t":"plain"}`)
	assert.Equal(t, plain, canonicalizeJSONEscapes(plain))
}

func TestRewriteClaudeCodeCCH_SyntheticBodies(t *testing.T) {
	// Expected values from the Python reference implementation.
	t.Run("already-edited body hashes as-is", func(t *testing.T) {
		body := `{"model":"","messages":[{"role":"user","content":"say hi"}],"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000;"}],"stream":true}`
		out, cch, ok := rewriteClaudeCodeCCH([]byte(body))
		require.True(t, ok)
		assert.Equal(t, "a92ae", cch)
		assert.Equal(t, strings.Replace(body, "cch=00000;", "cch=a92ae;", 1), string(out))
		assert.Len(t, out, len(body), "in-place patch keeps the length")
	})
	t.Run("Go-escaped body with model and max_tokens", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-6","max_tokens":32000,"messages":[{"role":"user","content":"say \u003chi\u003e \u0026 bye \u2028 ok"}],"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000;"}],"stream":true}`
		out, cch, ok := rewriteClaudeCodeCCH([]byte(body))
		require.True(t, ok)
		assert.Equal(t, "a6a96", cch)
		assert.Contains(t, string(out), "say <hi> & bye \u2028 ok", "wire body is JS-canonical")
		assert.Contains(t, string(out), `"model":"claude-sonnet-4-6","max_tokens":32000`, "wire body keeps model and max_tokens")
		assert.Contains(t, string(out), "cch=a6a96;")
	})
	t.Run("max_tokens last", func(t *testing.T) {
		body := `{"messages":[],"model":"claude-x","stream":true,"max_tokens":5}`
		assert.Equal(t, "cfc88", formatClaudeCodeCCH(xxhash64Zig(claudeCodeCCHPreimage([]byte(body)), claudeCodeCCHSeed)))
	})
	t.Run("no placeholder: canonicalized only", func(t *testing.T) {
		out, cch, ok := rewriteClaudeCodeCCH([]byte(`{"a":"\u003c"}`))
		assert.False(t, ok)
		assert.Empty(t, cch)
		assert.Equal(t, `{"a":"<"}`, string(out))
	})
}

// TestRewriteClaudeCodeCCH_LiveCaptures replays raw bodies captured from the
// official binary when TINGLY_CC_CAPTURE_DIR points at a directory of
// req*.raw files (each already carrying the real cch). Skipped otherwise.
func TestRewriteClaudeCodeCCH_LiveCaptures(t *testing.T) {
	dir := os.Getenv("TINGLY_CC_CAPTURE_DIR")
	if dir == "" {
		t.Skip("TINGLY_CC_CAPTURE_DIR not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "req*.raw"))
	require.NotEmpty(t, files)
	re := regexp.MustCompile(`cch=([0-9a-f]{5});`)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		m := re.FindSubmatch(raw)
		require.NotNil(t, m, "%s has no cch", f)
		placeholder := bytes.Replace(raw, m[0], []byte(claudeCodeCCHPlaceholder), 1)
		out, cch, ok := rewriteClaudeCodeCCH(placeholder)
		require.True(t, ok)
		assert.Equal(t, string(m[1]), cch, "%s", f)
		assert.Equal(t, raw, out, "%s: rewritten body must equal the captured wire body", f)
	}
}

func TestClaudeClient_CCHOnTheWire(t *testing.T) {
	var captured []byte
	var contentLength int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		contentLength = r.ContentLength
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": []map[string]any{{"type": "text", "text": "hi"}}, "stop_reason": "end_turn",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	c := newTestClaudeClient(t, ctx, srv.URL)
	req := betaRequestWithMetadata()
	req.System = []anthropic.BetaTextBlockParam{
		{Text: "x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000;"},
		{Text: "<system-reminder>You are Claude Code & friends</system-reminder>"},
	}
	_, err := c.BetaMessagesNew(ctx, req)
	require.NoError(t, err)

	body := string(captured)
	m := regexp.MustCompile(`cch=([0-9a-f]{5});`).FindStringSubmatch(body)
	require.NotNil(t, m, "cch not patched: %s", body)
	assert.NotEqual(t, "00000", m[1])
	assert.Equal(t, int64(len(captured)), contentLength)
	assert.Contains(t, body, "<system-reminder>You are Claude Code & friends</system-reminder>", "no HTML-safe escaping on the wire")
	assert.NotContains(t, body, `\u003c`)

	// The value is reproducible from the wire body with the documented rule.
	placeholder := strings.Replace(body, m[0], claudeCodeCCHPlaceholder, 1)
	assert.Equal(t, m[1], formatClaudeCodeCCH(xxhash64Zig(claudeCodeCCHPreimage([]byte(placeholder)), claudeCodeCCHSeed)))
}
