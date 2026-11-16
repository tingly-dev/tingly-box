package transform

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// claudeCodeOAuthProvider is a Claude Code OAuth provider at apiBase: the only kind
// of provider the identity rewrite (billing header + metadata) runs for.
func claudeCodeOAuthProvider(apiBase string) *typ.Provider {
	return &typ.Provider{
		APIBase:  apiBase,
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			Issuer:      ai.IssuerClaudeCode,
			AccessToken: "sk-ant-oat01-x",
		},
	}
}

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
	assert.Regexp(t, `^x-anthropic-billing-header: cc_version=2\.1\.86\.[0-9a-f]{3}; cc_entrypoint=cli; cch=[0-9a-f]{5};$`, out.System[0].Text)
	assert.Contains(t, out.Metadata.UserID.String(), `"device_id":"dev"`)
}

// The claude_code_version flag reaches the vendor transform through the
// chain's Extra map (stamped by ClaudeCodeVersionTransform) and selects the
// native identity rewrite.
func TestVendorTransform_AnthropicBeta_ClaudeCodeVersionSelectsNativeIdentity(t *testing.T) {
	vt := NewVendorTransform()
	req := &anthropic.BetaMessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 64,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("say hi"))},
	}
	ctx := &TransformContext{
		Provider: claudeCodeOAuthProvider("https://api.anthropic.com"),
		Request:  req,
		Extra:    map[string]interface{}{"device": "dev", "user_id": "acct"},
	}
	require.NoError(t, NewClaudeCodeVersionTransform(typ.ClaudeCodeVersion2_1_280).Apply(ctx))
	require.NoError(t, vt.Apply(ctx))

	out := ctx.Request.(*anthropic.BetaMessageNewParams)
	require.NotEmpty(t, out.System)
	assert.Equal(t, "x-anthropic-billing-header: cc_version=2.1.280.31f; cc_entrypoint=cli; cch=00000;", out.System[0].Text)
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

// An API-key provider on Anthropic's own host is plain API usage. The
// identity rewrite would have Anthropic bill it as Claude Code, which the API
// credits of Max/Team plans do not cover ("Your credit balance is too low").
// Anthropic's wire rules still apply there: Haiku loses adaptive thinking.
func TestVendorTransform_AnthropicV1_APIKeyOnAnthropicHostCarriesNoIdentity(t *testing.T) {
	vt := NewVendorTransform()
	req := &anthropic.MessageNewParams{
		Model:     "claude-3-5-haiku-20241022",
		MaxTokens: 64,
		System:    []anthropic.TextBlockParam{{Text: "You are a helpful assistant."}},
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("say hi"))},
	}
	ctx := &TransformContext{
		Provider: &typ.Provider{APIBase: "https://api.anthropic.com", AuthType: ai.AuthTypeAPIKey, Token: "sk-ant-api03-x"},
		Request:  req,
		Extra:    map[string]interface{}{"device": "dev", "user_id": "acct"},
	}
	require.NoError(t, vt.Apply(ctx))

	out := ctx.Request.(*anthropic.MessageNewParams)
	require.Len(t, out.System, 1)
	assert.Equal(t, "You are a helpful assistant.", out.System[0].Text)
	assert.False(t, out.Metadata.UserID.Valid())
	assert.Nil(t, out.Thinking.OfAdaptive, "Anthropic's wire rules still apply on its host")
}

// Same for the Beta shape, with the native profile selected: claude_code_version
// picks how the identity is rendered, not whether an API key gets one.
func TestVendorTransform_AnthropicBeta_APIKeyOnAnthropicHostCarriesNoIdentity(t *testing.T) {
	vt := NewVendorTransform()
	req := &anthropic.BetaMessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 64,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("say hi"))},
	}
	ctx := &TransformContext{
		Provider: &typ.Provider{APIBase: "https://api.anthropic.com", AuthType: ai.AuthTypeAPIKey, Token: "sk-ant-api03-x"},
		Request:  req,
		Extra:    map[string]interface{}{"device": "dev", "user_id": "acct"},
	}
	require.NoError(t, NewClaudeCodeVersionTransform(typ.ClaudeCodeVersion2_1_280).Apply(ctx))
	require.NoError(t, vt.Apply(ctx))

	out := ctx.Request.(*anthropic.BetaMessageNewParams)
	assert.Empty(t, out.System)
	assert.False(t, out.Metadata.UserID.Valid())
}
