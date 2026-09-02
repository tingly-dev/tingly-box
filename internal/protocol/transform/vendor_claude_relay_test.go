package transform

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// A Claude Code OAuth provider whose APIBase is a relay (not api.anthropic.com)
// still goes to Anthropic's Claude Code backend, so the identity rewrite
// (billing header + metadata) must run on it too — otherwise the OAuth chain's
// Guard sees a request without metadata.user_id and panics.
func TestVendorTransform_AnthropicBeta_ClaudeCodeIssuerOnRelayHost(t *testing.T) {
	vt := NewVendorTransform()
	req := &anthropic.BetaMessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 64,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("say hi"))},
	}
	ctx := &TransformContext{
		Provider: &typ.Provider{
			APIBase:  "https://relay.example.com/v1",
			AuthType: ai.AuthTypeOAuth,
			OAuthDetail: &ai.OAuthDetail{
				Issuer:      ai.IssuerClaudeCode,
				AccessToken: "sk-ant-oat01-x",
			},
		},
		Request: req,
		Extra:   map[string]interface{}{"device": "dev", "user_id": "acct"},
	}
	require.NoError(t, vt.Apply(ctx))

	out := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.NotEmpty(t, out.System)
	assert.Equal(t, "x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000;", out.System[0].Text)
	assert.Contains(t, out.Metadata.UserID.String(), `"device_id":"dev"`)
}

// A plain (non Claude Code) Anthropic-compatible provider on a foreign host
// keeps the classic behavior: no identity rewrite.
func TestVendorTransform_AnthropicBeta_ForeignHostUntouched(t *testing.T) {
	vt := NewVendorTransform()
	req := &anthropic.BetaMessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 64,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("say hi"))},
	}
	ctx := &TransformContext{
		Provider: &typ.Provider{APIBase: "https://relay.example.com/v1", AuthType: ai.AuthTypeAPIKey, Token: "k"},
		Request:  req,
		Extra:    map[string]interface{}{"device": "dev", "user_id": "acct"},
	}
	require.NoError(t, vt.Apply(ctx))

	out := ctx.Request.(*anthropic.BetaMessageNewParams)
	assert.Empty(t, out.System)
	assert.False(t, out.Metadata.UserID.Valid())
}
