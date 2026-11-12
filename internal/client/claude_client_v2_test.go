package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Wire-level coverage for the Claude Code client V2 variant
// (NewClaudeClientV2): the 2026-10-21 wire on the Claude Code OAuth path —
// SDK-default anthropic-version, Bearer auth for every credential shape,
// sampling extras stripped — while ?beta=true and the Claude Code header set
// stay exactly as on the classic client.

type claudeV2Capture struct {
	header http.Header
	query  string
	body   []byte
}

func newClaudeV2CapturingServer(t *testing.T, capture *claudeV2Capture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.header = r.Header.Clone()
		capture.query = r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		capture.body = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_01","type":"message","role":"assistant","model":"claude-sonnet-4-6",` +
			`"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn",` +
			`"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
}

func newClaudeV2OAuthProvider(apiBase string) *typ.Provider {
	return &typ.Provider{
		Name:     "test-claude-v2",
		APIBase:  apiBase,
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "sk-ant-oat01-testtoken",
		},
	}
}

func newClaudeV2MessageRequest() *anthropic.MessageNewParams {
	req := &anthropic.MessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 16,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(`{"device_id":"d","account_uuid":"a","session_id":"sess-1"}`)
	// Sampling params travel through extras since the decode-preservation
	// patch (see internal/protocol/request/sampling_extra.go).
	req.SetExtraFields(map[string]any{"temperature": 0.7, "top_p": 0.9, "top_k": 40, "custom": "keep"})
	return req
}

func TestClaudeClientV2_OAuthWire(t *testing.T) {
	var capture claudeV2Capture
	srv := newClaudeV2CapturingServer(t, &capture)
	defer srv.Close()

	c, err := NewClaudeClientV2(context.Background(), newClaudeV2OAuthProvider(srv.URL), "claude-sonnet-4-6", typ.SessionID{Value: "s"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	_, err = c.MessagesNew(context.Background(), newClaudeV2MessageRequest())
	require.NoError(t, err)

	// OAuth auth stays Bearer.
	assert.Equal(t, "Bearer sk-ant-oat01-testtoken", capture.header.Get("Authorization"))
	assert.Empty(t, capture.header.Get("X-Api-Key"))
	// No pinned anthropic-version: the SDK default (2026-10-21) stands.
	assert.Equal(t, "2026-10-21", capture.header.Get("Anthropic-Version"))
	// The Claude-endpoint ?beta=true is kept on the V2 wire (see the
	// newClaudeClientWithWire comment — it is part of the OAuth wire shape,
	// not a classic-only artifact).
	assert.Contains(t, capture.query, "beta=true")
	// The Claude Code header set is intact.
	assert.Contains(t, capture.header.Get("Anthropic-Beta"), anthropicOAuthBeta)
	assert.NotEmpty(t, capture.header.Get("X-Claude-Code-Session-Id"))
	assert.Equal(t, claudeCLIUserAgent, capture.header.Get("User-Agent"))

	// Sampling extras rejected by the v2 wire are stripped; other extras stay.
	var sent map[string]any
	require.NoError(t, json.Unmarshal(capture.body, &sent))
	assert.NotContains(t, sent, "temperature")
	assert.NotContains(t, sent, "top_p")
	assert.NotContains(t, sent, "top_k")
	assert.Equal(t, "keep", sent["custom"])
}

func TestClaudeClientV2_APIKeyBearer(t *testing.T) {
	var capture claudeV2Capture
	srv := newClaudeV2CapturingServer(t, &capture)
	defer srv.Close()

	// A non-OAuth credential on the Claude V2 wire: v2 auth semantics send
	// Authorization: Bearer instead of the classic x-api-key.
	provider := &typ.Provider{
		Name:     "test-claude-v2-key",
		APIBase:  srv.URL,
		AuthType: ai.AuthTypeAPIKey,
		Token:    "sk-ant-api-testkey",
	}
	c, err := NewClaudeClientV2(context.Background(), provider, "claude-sonnet-4-6", typ.SessionID{Value: "s"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	req := newClaudeV2MessageRequest()
	req.SetExtraFields(nil)
	_, err = c.MessagesNew(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, "Bearer sk-ant-api-testkey", capture.header.Get("Authorization"))
	assert.Empty(t, capture.header.Get("X-Api-Key"))
	assert.Equal(t, "2026-10-21", capture.header.Get("Anthropic-Version"))
}

func TestClaudeClientV2_BetaWire(t *testing.T) {
	var capture claudeV2Capture
	srv := newClaudeV2CapturingServer(t, &capture)
	defer srv.Close()

	c, err := NewClaudeClientV2(context.Background(), newClaudeV2OAuthProvider(srv.URL), "claude-sonnet-4-6", typ.SessionID{Value: "s"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	req := &anthropic.BetaMessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 16,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(`{"device_id":"d","account_uuid":"a","session_id":"sess-1"}`)
	req.SetExtraFields(map[string]any{"temperature": 0.7})

	_, err = c.BetaMessagesNew(context.Background(), req)
	require.NoError(t, err)

	// ?beta=true survives on the beta endpoint too (single value), auth is
	// Bearer, version is the SDK default.
	assert.Equal(t, "beta=true", capture.query)
	assert.Equal(t, "Bearer sk-ant-oat01-testtoken", capture.header.Get("Authorization"))
	assert.Equal(t, "2026-10-21", capture.header.Get("Anthropic-Version"))

	var sent map[string]any
	require.NoError(t, json.Unmarshal(capture.body, &sent))
	assert.NotContains(t, sent, "temperature")
}

func TestClaudeClient_ClassicWire_Unchanged(t *testing.T) {
	var capture claudeV2Capture
	srv := newClaudeV2CapturingServer(t, &capture)
	defer srv.Close()

	c, err := NewClaudeClient(context.Background(), newClaudeV2OAuthProvider(srv.URL), "claude-sonnet-4-6", typ.SessionID{Value: "s"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	_, err = c.MessagesNew(context.Background(), newClaudeV2MessageRequest())
	require.NoError(t, err)

	// Classic wire: pinned 2023-06-01, Bearer for OAuth, beta query, sampling
	// extras carried through.
	assert.Equal(t, "Bearer sk-ant-oat01-testtoken", capture.header.Get("Authorization"))
	assert.Equal(t, "2023-06-01", capture.header.Get("Anthropic-Version"))
	assert.Contains(t, capture.query, "beta=true")

	var sent map[string]any
	require.NoError(t, json.Unmarshal(capture.body, &sent))
	assert.Equal(t, 0.7, sent["temperature"])
	assert.Equal(t, 0.9, sent["top_p"])
	assert.Equal(t, 40.0, sent["top_k"])
}
