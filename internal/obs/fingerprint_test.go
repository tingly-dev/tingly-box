package obs

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDPattern(t *testing.T) {
	cases := map[string]string{
		"chatcmpl-AbC123": "chatcmpl-*",
		"msg_abc":         "msg_*",
		"resp_123":        "resp_*",
		"gen-1730000-x":   "gen-*",
		"":                "",
		"nodashes":        "*",
	}
	for in, want := range cases {
		assert.Equal(t, want, IDPattern(in), "IDPattern(%q)", in)
	}
}

func TestFingerprintHeaders_AllowlistAndRedaction(t *testing.T) {
	headers := map[string]string{
		"Authorization":            "Bearer secret",
		"Set-Cookie":               "sid=abc",
		"Content-Type":             "application/json",
		"X-Request-Id":             "req_0123456789abcdef",
		"X-Ratelimit-Limit-Tokens": "90000",
		"X-Unknown-Token":          "should-drop",
	}
	got := FingerprintHeaders(headers)
	require.NotNil(t, got)
	assert.Equal(t, "application/json", got["content-type"])
	assert.Equal(t, "req_*", got["x-request-id"], "id-bearing headers are patternized")
	assert.Equal(t, "90000", got["x-ratelimit-limit-tokens"])
	for _, k := range []string{"authorization", "set-cookie", "x-unknown-token"} {
		_, ok := got[k]
		assert.False(t, ok, "%s must not survive fingerprint redaction", k)
	}
}

func TestFingerprintUpstreamHeaders_AllowlistAndRedaction(t *testing.T) {
	headers := map[string]string{
		"Content-Type":                       "application/json",
		"X-Request-Id":                       "req_0123456789abcdef",
		"Cf-Ray":                             "8c1e-SJC",
		"X-Ms-Request-Id":                    "abc-def",
		"Anthropic-Ratelimit-Requests-Limit": "1000",
		"Openai-Organization":                "org-account-id",
		"X-Api-Key":                          "sk-secret",
		"Set-Cookie":                         "sid=abc",
		"X-Some-Internal-Debug-Header":       "nope",
	}
	got := FingerprintUpstreamHeaders(headers)
	require.NotNil(t, got)
	assert.Equal(t, "application/json", got["content-type"])
	assert.Equal(t, "req_*", got["x-request-id"], "id-bearing headers are patternized")
	assert.Equal(t, "8c1e-*", got["cf-ray"])
	assert.Equal(t, "abc-*", got["x-ms-request-id"])
	assert.Equal(t, "1000", got["anthropic-ratelimit-requests-limit"], "vendor rate-limit headers are kept")
	for _, k := range []string{"openai-organization", "x-api-key", "set-cookie", "x-some-internal-debug-header"} {
		_, ok := got[k]
		assert.False(t, ok, "%s must not survive upstream redaction", k)
	}
}

func TestFingerprintUpstreamHeaders_TruncatesValues(t *testing.T) {
	got := FingerprintUpstreamHeaders(map[string]string{"Server": strings.Repeat("a", 400)})
	require.NotNil(t, got)
	assert.Len(t, got["server"], maxHeaderValueLen)
}

func TestBuildUpstreamResponse(t *testing.T) {
	up := BuildUpstreamResponse(429, map[string]string{"Server": "envoy", "Set-Cookie": "x"})
	require.NotNil(t, up)
	assert.Equal(t, 429, up.StatusCode)
	assert.Equal(t, "envoy", up.Headers["server"])
	assert.NotContains(t, up.Headers, "set-cookie")
}

func TestRedactFullHeaders(t *testing.T) {
	long := ""
	for range 400 {
		long += "a"
	}
	headers := map[string]string{
		"Authorization":  "Bearer secret",
		"Set-Cookie":     "sid=abc",
		"Content-Type":   "application/json",
		"X-Custom-Trace": long,
	}
	got := RedactFullHeaders(headers)
	require.NotNil(t, got)
	assert.Equal(t, "application/json", got["Content-Type"], "non-sensitive headers are kept for debugging")
	assert.Len(t, got["X-Custom-Trace"], maxHeaderValueLen, "values are truncated")
	for _, k := range []string{"Authorization", "Set-Cookie"} {
		_, ok := got[k]
		assert.False(t, ok, "%s must never be persisted", k)
	}
}

func TestBuildResponseFingerprint_OpenAIChat(t *testing.T) {
	body := map[string]interface{}{
		"id":                 "chatcmpl-AbC123",
		"model":              "gpt-4o-2024-08-06",
		"system_fingerprint": "fp_44709d6fcb",
		"service_tier":       "default",
		"choices": []interface{}{
			map[string]interface{}{"finish_reason": "stop"},
			map[string]interface{}{"finish_reason": "stop"},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     float64(11),
			"completion_tokens": float64(4),
			"total_tokens":      float64(15),
			"prompt_tokens_details": map[string]interface{}{
				"cached_tokens": float64(3),
			},
			"completion_tokens_details": map[string]interface{}{
				"reasoning_tokens": float64(2),
			},
		},
	}
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer secret",
	}
	fp := BuildResponseFingerprint("openai", 200, headers, body, 1500*time.Millisecond)
	require.NotNil(t, fp)
	assert.Equal(t, "through_tb", fp.Channel)
	assert.Equal(t, "openai", fp.Protocol)
	assert.Equal(t, 200, fp.StatusCode)
	assert.Equal(t, int64(1500), fp.ElapsedMs)
	assert.Equal(t, "gpt-4o-2024-08-06", fp.Model)
	assert.Equal(t, "chatcmpl-*", fp.IDPattern)
	assert.Equal(t, "fp_44709d6fcb", fp.SystemFingerprint)
	assert.Equal(t, "default", fp.ServiceTier)
	assert.Equal(t, []string{"stop"}, fp.FinishReasons)
	assert.Equal(t, "application/json", fp.Headers["content-type"])
	assert.NotContains(t, fp.Headers, "authorization")

	require.NotNil(t, fp.Usage)
	assert.Equal(t, 11, fp.Usage.InputTokens)
	assert.Equal(t, 4, fp.Usage.OutputTokens)
	assert.Equal(t, 3, fp.Usage.CacheReadTokens)
	assert.Equal(t, 2, fp.Usage.ReasoningTokens)
	assert.Equal(t, 15, fp.Usage.TotalTokens)
	assert.Contains(t, fp.Usage.Keys, "prompt_tokens_details.cached_tokens")
}

func TestBuildResponseFingerprint_Anthropic(t *testing.T) {
	body := map[string]interface{}{
		"id":          "msg_01ABC",
		"model":       "claude-opus-4-5",
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{
			"input_tokens":                float64(20),
			"output_tokens":               float64(7),
			"cache_read_input_tokens":     float64(5),
			"cache_creation_input_tokens": float64(2),
		},
	}
	fp := BuildResponseFingerprint("anthropic", 200, nil, body, 0)
	require.NotNil(t, fp)
	assert.Equal(t, "msg_*", fp.IDPattern)
	assert.Equal(t, []string{"end_turn"}, fp.FinishReasons)
	require.NotNil(t, fp.Usage)
	assert.Equal(t, 20, fp.Usage.InputTokens)
	assert.Equal(t, 7, fp.Usage.OutputTokens)
	assert.Equal(t, 5, fp.Usage.CacheReadTokens)
	assert.Equal(t, 2, fp.Usage.CacheWriteTokens)
}

func TestBuildResponseFingerprint_NilBody(t *testing.T) {
	fp := BuildResponseFingerprint("openai", 200, nil, nil, time.Second)
	require.NotNil(t, fp)
	assert.Nil(t, fp.Usage)
	assert.Empty(t, fp.Model)
	assert.Equal(t, "through_tb", fp.Channel)
}

func TestCapResponseBody(t *testing.T) {
	small := map[string]interface{}{"model": "m", "usage": map[string]interface{}{"input_tokens": float64(1)}}
	assert.Equal(t, small, CapResponseBody(small, 1024), "small bodies pass through untouched")

	big := map[string]interface{}{
		"id":    "chatcmpl-x",
		"model": "m",
		"usage": map[string]interface{}{"input_tokens": float64(1)},
		"content": func() string {
			s := ""
			for range 2048 {
				s += "a"
			}
			return s
		}(),
	}
	capped := CapResponseBody(big, 256)
	require.NotNil(t, capped)
	assert.Equal(t, true, capped["_truncated"])
	assert.Contains(t, capped, "_sha256")
	assert.Contains(t, capped, "_bytes")
	assert.Equal(t, "chatcmpl-x", capped["id"], "fingerprint-relevant scalars survive the cap")
	assert.Contains(t, capped, "usage")
}

func TestCapResponseBody_Disabled(t *testing.T) {
	big := map[string]interface{}{"content": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}
	assert.Equal(t, big, CapResponseBody(big, 0), "maxBytes<=0 disables the cap")
}
