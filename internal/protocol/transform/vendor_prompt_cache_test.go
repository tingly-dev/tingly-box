package transform

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// cachedBetaRequest carries cache_control everywhere Claude Code puts it, plus
// the top-level (automatic) form and a block nested in a tool result.
func cachedBetaRequest(t *testing.T) *anthropic.BetaMessageNewParams {
	t.Helper()
	var req anthropic.BetaMessageNewParams
	require.NoError(t, json.Unmarshal([]byte(`{
		"model": "claude-sonnet-4-6", "max_tokens": 64,
		"cache_control": {"type": "ephemeral"},
		"system": [{"type": "text", "text": "sys", "cache_control": {"type": "ephemeral", "ttl": "1h"}}],
		"tools": [{"name": "read", "input_schema": {"type": "object"}, "cache_control": {"type": "ephemeral"}}],
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hi", "cache_control": {"type": "ephemeral"}}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "t1", "name": "read", "input": {}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "t1", "cache_control": {"type": "ephemeral"},
				"content": [{"type": "text", "text": "out", "cache_control": {"type": "ephemeral"}}]}]}
		]}`), &req))
	return &req
}

func cacheControlCount(t *testing.T, req *anthropic.BetaMessageNewParams) int {
	t.Helper()
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	return strings.Count(string(raw), `"cache_control"`)
}

func TestVendorTransform_AnthropicCacheControl(t *testing.T) {
	cases := []struct {
		name     string
		provider *typ.Provider
		keep     bool
	}{
		{"anthropic_official", &typ.Provider{APIBase: "https://api.anthropic.com"}, true},
		{"third_party_compatible", &typ.Provider{APIBase: "https://open.bigmodel.cn/api/anthropic"}, false},
		{"deepseek", &typ.Provider{APIBase: "https://api.deepseek.com/anthropic"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := cachedBetaRequest(t)
	require.Equal(t, 6, cacheControlCount(t, req))
			ctx := &TransformContext{Provider: tc.provider, Request: req, Extra: map[string]interface{}{"device": "dev", "user_id": "acct"}}
			require.NoError(t, NewVendorTransform().Apply(ctx))

			got := cacheControlCount(t, ctx.Request.(*anthropic.BetaMessageNewParams))
			if tc.keep {
				assert.Equal(t, 6, got, "Anthropic implements cache_control: every breakpoint stays")
			} else {
				assert.Zero(t, got, "third-party providers get no cache_control")
			}
		})
	}
}
