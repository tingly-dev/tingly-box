package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func claudeServerWithHeaders(t *testing.T, headers map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "m", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": []map[string]any{{"type": "text", "text": "hi"}}, "stop_reason": "end_turn",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

type observed struct {
	mu      sync.Mutex
	calls   int
	headers http.Header
	uuid    string
}

func observeClaude(t *testing.T) *observed {
	t.Helper()
	o := &observed{}
	SetClaudeResponseObserver(func(p *typ.Provider, h http.Header) {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.calls++
		o.headers = h
		o.uuid = p.UUID
	})
	t.Cleanup(func() { SetClaudeResponseObserver(nil) })
	return o
}

func TestClaudeClient_ReportsRateLimitHeaders(t *testing.T) {
	o := observeClaude(t)
	srv := claudeServerWithHeaders(t, map[string]string{
		"anthropic-ratelimit-unified-status":         "allowed",
		"anthropic-ratelimit-unified-5h-utilization": "0.42",
		"anthropic-ratelimit-unified-5h-reset":       "1786147200",
	})
	ctx := context.Background()
	c := newTestClaudeClient(t, ctx, srv.URL)
	c.GetProvider().UUID = "claude-1"

	_, err := c.BetaMessagesNew(ctx, betaRequestWithMetadata())
	require.NoError(t, err)

	o.mu.Lock()
	defer o.mu.Unlock()
	assert.Equal(t, 1, o.calls)
	assert.Equal(t, "claude-1", o.uuid)
	assert.Equal(t, "0.42", o.headers.Get("anthropic-ratelimit-unified-5h-utilization"))
}

func TestClaudeClient_NoRateLimitHeadersNoReport(t *testing.T) {
	o := observeClaude(t)
	srv := claudeServerWithHeaders(t, nil)
	ctx := context.Background()
	c := newTestClaudeClient(t, ctx, srv.URL)

	_, err := c.BetaMessagesNew(ctx, betaRequestWithMetadata())
	require.NoError(t, err)

	o.mu.Lock()
	defer o.mu.Unlock()
	assert.Zero(t, o.calls)
}

func TestClaudeClient_RateLimitObserverIsOptional(t *testing.T) {
	SetClaudeResponseObserver(nil)
	srv := claudeServerWithHeaders(t, map[string]string{"anthropic-ratelimit-unified-status": "allowed"})
	ctx := context.Background()
	c := newTestClaudeClient(t, ctx, srv.URL)
	_, err := c.BetaMessagesNew(ctx, betaRequestWithMetadata())
	require.NoError(t, err)
}
