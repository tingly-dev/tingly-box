package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestClaudeClient_ClaudeOrgID drives the full ClaudeClient path
// (NewClaudeClient → Guard → SDK → HTTP) against a capturing server and pins
// the claude_org_id semantics: unset attaches no organization header (classic
// behavior — the login-time capture is opt-in), "auto" attributes to the
// organization captured at OAuth login, and any other value attributes to
// that organization. The flag is resolved from the request context at
// construction (the pool builds a client per request); Guard-built clients
// inherit it via the base client's Options.
func TestClaudeClient_ClaudeOrgID(t *testing.T) {
	const loginOrg = "11111111-2222-3333-4444-555555555555"
	const customOrg = "99999999-8888-7777-6666-555555555555"

	var gotOrg string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrg = r.Header.Get("anthropic-organization-id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6",` +
			`"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn",` +
			`"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	newProvider := func() *typ.Provider {
		return &typ.Provider{
			Name:     "test-claude-org",
			APIBase:  srv.URL,
			AuthType: ai.AuthTypeOAuth,
			OAuthDetail: &ai.OAuthDetail{
				AccessToken: "sk-ant-oat01-testtoken",
				ExtraFields: map[string]interface{}{"organization_id": loginOrg},
			},
		}
	}

	newReq := func() *anthropic.MessageNewParams {
		return &anthropic.MessageNewParams{
			Model:     "claude-sonnet-4-6",
			MaxTokens: 16,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
			},
			Metadata: anthropic.MetadataParam{
				UserID: param.NewOpt(`{"device_id":"d","account_uuid":"a","session_id":"sess-1"}`),
			},
		}
	}

	send := func(t *testing.T, ctx context.Context, provider *typ.Provider) {
		t.Helper()
		c, err := NewClaudeClient(ctx, provider, "claude-sonnet-4-6", typ.SessionID{Value: "s"})
		require.NoError(t, err)
		_, err = c.MessagesNew(ctx, newReq())
		require.NoError(t, err)
	}

	t.Run("default sends no org header even when login org is known", func(t *testing.T) {
		send(t, context.Background(), newProvider())
		assert.Empty(t, gotOrg)
	})

	t.Run("auto sends the login-time org", func(t *testing.T) {
		ctx := typ.WithClaudeOrgID(context.Background(), typ.ClaudeOrgIDAuto)
		send(t, ctx, newProvider())
		assert.Equal(t, loginOrg, gotOrg)
	})

	t.Run("auto with no login-time org sends no header", func(t *testing.T) {
		ctx := typ.WithClaudeOrgID(context.Background(), typ.ClaudeOrgIDAuto)
		p := newProvider()
		p.OAuthDetail.ExtraFields = nil
		send(t, ctx, p)
		assert.Empty(t, gotOrg)
	})

	t.Run("custom value replaces the login-time org", func(t *testing.T) {
		ctx := typ.WithClaudeOrgID(context.Background(), customOrg)
		send(t, ctx, newProvider())
		assert.Equal(t, customOrg, gotOrg)
	})
}
