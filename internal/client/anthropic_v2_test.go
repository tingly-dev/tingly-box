package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Wire-level coverage for the Client V2 group (anthropic_v2.go /
// NewAnthropicClientV2): official-host providers speak the new 2026-10-21
// API (Bearer auth, no anthropic-version pin, no ?beta=true, sampling
// extras stripped), while the classic client keeps the 2023-06-01 wire.

type v2Capture struct {
	header http.Header
	query  string
	body   []byte
}

func newV2CapturingServer(t *testing.T, capture *v2Capture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.header = r.Header.Clone()
		capture.query = r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		capture.body = body
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_01", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content":     []map[string]any{{"type": "text", "text": "hi"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
}

func newV2TestProvider(apiBase string) *typ.Provider {
	return &typ.Provider{
		Name:     "p",
		APIBase:  apiBase,
		APIStyle: protocol.APIStyleAnthropic,
		Token:    "sk-test-key",
	}
}

func v2MessageRequest() *anthropic.MessageNewParams {
	req := &anthropic.MessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 16,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
	}
	// Sampling params travel through extras since the decode-preservation
	// patch (see internal/protocol/request/sampling_extra.go).
	req.SetExtraFields(map[string]any{"temperature": 0.7, "top_p": 0.9, "top_k": 40, "custom": "keep"})
	return req
}

func TestAnthropicClientV2_Wire(t *testing.T) {
	var capture v2Capture
	srv := newV2CapturingServer(t, &capture)
	defer srv.Close()

	c, err := NewAnthropicClientV2(newV2TestProvider(srv.URL), "claude-sonnet-4-6", typ.SessionID{})
	require.NoError(t, err)

	_, err = c.MessagesNew(context.Background(), v2MessageRequest())
	require.NoError(t, err)

	// v2 auth: Authorization Bearer, no x-api-key.
	assert.Equal(t, "Bearer sk-test-key", capture.header.Get("Authorization"))
	assert.Empty(t, capture.header.Get("X-Api-Key"))
	// No pinned anthropic-version: the SDK default (2026-10-21) is used.
	assert.Equal(t, "2026-10-21", capture.header.Get("Anthropic-Version"))

	// Sampling extras rejected by the v2 wire are stripped; other extras stay.
	var sent map[string]any
	require.NoError(t, json.Unmarshal(capture.body, &sent))
	assert.NotContains(t, sent, "temperature")
	assert.NotContains(t, sent, "top_p")
	assert.NotContains(t, sent, "top_k")
	assert.Equal(t, "keep", sent["custom"])
}

func TestAnthropicClientV2_BetaWire_NoBetaQuery(t *testing.T) {
	var capture v2Capture
	srv := newV2CapturingServer(t, &capture)
	defer srv.Close()

	c, err := NewAnthropicClientV2(newV2TestProvider(srv.URL), "claude-sonnet-4-6", typ.SessionID{})
	require.NoError(t, err)

	req := &anthropic.BetaMessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 16,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaContext1m2025_08_07},
	}
	req.SetExtraFields(map[string]any{"temperature": 0.7})
	_, err = c.BetaMessagesNew(context.Background(), req)
	require.NoError(t, err)

	// v2 API has no ?beta=true routing.
	assert.Empty(t, capture.query)
	assert.Equal(t, "Bearer sk-test-key", capture.header.Get("Authorization"))
	assert.Equal(t, "2026-10-21", capture.header.Get("Anthropic-Version"))

	var sent map[string]any
	require.NoError(t, json.Unmarshal(capture.body, &sent))
	assert.NotContains(t, sent, "temperature")
}

func TestAnthropicClient_ClassicWire_Unchanged(t *testing.T) {
	var capture v2Capture
	srv := newV2CapturingServer(t, &capture)
	defer srv.Close()

	c, err := NewAnthropicClient(newV2TestProvider(srv.URL), "claude-sonnet-4-6", typ.SessionID{})
	require.NoError(t, err)

	_, err = c.MessagesNew(context.Background(), v2MessageRequest())
	require.NoError(t, err)

	// Classic auth: x-api-key, no Authorization bearer.
	assert.Equal(t, "sk-test-key", capture.header.Get("X-Api-Key"))
	assert.Empty(t, capture.header.Get("Authorization"))
	// Classic pinned API version.
	assert.Equal(t, "2023-06-01", capture.header.Get("Anthropic-Version"))

	// Classic wire keeps sampling extras.
	var sent map[string]any
	require.NoError(t, json.Unmarshal(capture.body, &sent))
	assert.Equal(t, 0.7, sent["temperature"])
	assert.Equal(t, 0.9, sent["top_p"])
	assert.Equal(t, 40.0, sent["top_k"])
}

func TestAnthropicClient_ClassicBetaWire_KeepsBetaQuery(t *testing.T) {
	var capture v2Capture
	srv := newV2CapturingServer(t, &capture)
	defer srv.Close()

	c, err := NewAnthropicClient(newV2TestProvider(srv.URL), "claude-sonnet-4-6", typ.SessionID{})
	require.NoError(t, err)

	req := &anthropic.BetaMessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 16,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaContext1m2025_08_07},
	}
	_, err = c.BetaMessagesNew(context.Background(), req)
	require.NoError(t, err)

	// The classic beta egress keeps ?beta=true (some compatible providers route on it).
	assert.Contains(t, capture.query, "beta=true")
	assert.Equal(t, "sk-test-key", capture.header.Get("X-Api-Key"))
	assert.Equal(t, "2023-06-01", capture.header.Get("Anthropic-Version"))
}

func TestIsOfficialAnthropicHost(t *testing.T) {
	assert.True(t, isOfficialAnthropicHost("https://api.anthropic.com"))
	assert.True(t, isOfficialAnthropicHost("https://api.anthropic.com/v1"))
	assert.True(t, isOfficialAnthropicHost("https://claude.ai"))
	assert.False(t, isOfficialAnthropicHost("https://api.deepseek.com"))
	assert.False(t, isOfficialAnthropicHost("https://example.com/anthropic-proxy"))
}
