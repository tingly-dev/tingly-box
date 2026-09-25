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

func newOAuthProvider() *typ.Provider {
	return &typ.Provider{
		Name:     "test-claude",
		APIBase:  "https://api.anthropic.com/v1",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "sk-ant-oat01-testtoken",
		},
	}
}

func newAPIKeyProvider() *typ.Provider {
	return &typ.Provider{
		Name:     "test-claude-key",
		APIBase:  "https://api.anthropic.com/v1",
		AuthType: ai.AuthTypeAPIKey,
		Token:    "sk-ant-api-testkey",
	}
}

// ===================================================================
// IsClaudeOAuthToken
// ===================================================================

func TestIsClaudeOAuthToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{"oauth token", "sk-ant-oat01-abc123", true},
		{"oauth token with sk-ant-oat prefix", "sk-ant-oat-xyz", true},
		{"api key token", "sk-ant-api-testkey", false},
		{"empty string", "", false},
		{"random string", "not-a-token", false},
		{"contains oat mid string", "prefix-sk-ant-oat-suffix", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsClaudeOAuthToken(tt.token))
		})
	}
}

// ===================================================================
// NewClaudeClient / applyClaudeCodeHeaders
// ===================================================================

func TestNewClaudeClient_OAuthToken(t *testing.T) {
	provider := newOAuthProvider()
	sessionID := typ.SessionID{Value: "test-session-123"}

	c, err := NewClaudeClient(context.Background(), provider, "claude-sonnet-4-6", sessionID)
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.NotNil(t, c.AnthropicClient)
}

func TestNewClaudeClient_APIKey(t *testing.T) {
	provider := newAPIKeyProvider()
	sessionID := typ.SessionID{Value: "test-session-456"}

	c, err := NewClaudeClient(context.Background(), provider, "claude-opus-4-6", sessionID)
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestNewClaudeClient_StripV1FromBase(t *testing.T) {
	// Provider base with /v1 suffix should be stripped
	provider := &typ.Provider{
		Name:     "test",
		APIBase:  "https://api.anthropic.com/v1",
		AuthType: ai.AuthTypeAPIKey,
		Token:    "sk-ant-api-test",
	}
	sessionID := typ.SessionID{Value: "sess"}
	c, err := NewClaudeClient(context.Background(), provider, "", sessionID)
	require.NoError(t, err)
	require.NotNil(t, c)
}

// ===================================================================
// ListModels
// ===================================================================

func TestClaudeClient_ListModels_UsesUpstream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected model-list path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data":[{"id":"claude-test","type":"model","display_name":"Claude Test","created_at":"2025-01-01T00:00:00Z"}],
			"has_more":false,
			"first_id":"claude-test",
			"last_id":"claude-test"
		}`))
	}))
	t.Cleanup(server.Close)

	provider := newOAuthProvider()
	provider.APIBase = server.URL
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	result, err := c.ListModels(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"claude-test"}, result.Models)
	assert.NotNil(t, result.Raw)
}

// ===================================================================
// restoreBetaToolNamesInMessage
// ===================================================================

func TestRestoreBetaToolNamesInMessage(t *testing.T) {
	t.Run("restores tool_use name", func(t *testing.T) {
		msg := &anthropic.BetaMessage{
			Content: []anthropic.BetaContentBlockUnion{
				{Type: "tool_use", Name: "Bash"},
			},
		}
		restoreBetaToolNamesInMessage(msg, map[string]string{"Bash": "bash"})
		assert.Equal(t, "bash", msg.Content[0].Name)
	})

	t.Run("inert for nil message, empty map, and non-tool blocks", func(t *testing.T) {
		restoreBetaToolNamesInMessage(nil, map[string]string{"Bash": "bash"})

		msg := &anthropic.BetaMessage{
			Content: []anthropic.BetaContentBlockUnion{
				{Type: "tool_use", Name: "Read"},
				{Type: "text", Name: ""},
			},
		}
		restoreBetaToolNamesInMessage(msg, nil)
		restoreBetaToolNamesInMessage(msg, map[string]string{"Bash": "bash"})
		assert.Equal(t, "Read", msg.Content[0].Name)
		assert.Equal(t, "", msg.Content[1].Name)
	})
}

// ===================================================================
// Guard — thinking field injection
// ===================================================================

func TestGuard_DisablesThinkingWhenUnset(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	// Build a valid metadata user_id (JSON format)
	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(userID)

	// Thinking is zero-value (all nil) — Guard should set OfDisabled
	base, _ := c.Guard(context.Background(), req)
	assert.NotNil(t, req.Thinking.OfDisabled, "Guard should set OfDisabled when thinking is unset")
	assert.NotNil(t, base)
}

func TestGuard_PreservesExistingThinking(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(userID)
	// Explicitly set thinking to enabled
	budget := int64(1000)
	req.Thinking = anthropic.ThinkingConfigParamUnion{
		OfEnabled: &anthropic.ThinkingConfigEnabledParam{BudgetTokens: budget},
	}

	base, _ := c.Guard(context.Background(), req)
	assert.NotNil(t, req.Thinking.OfEnabled, "Guard should not overwrite existing thinking config")
	assert.Nil(t, req.Thinking.OfDisabled)
	assert.NotNil(t, base)
}

// ===================================================================
// GuardBeta — thinking field injection
// ===================================================================

func TestGuardBeta_DisablesThinkingWhenUnset(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(userID)

	base, _ := c.GuardBeta(context.Background(), req)
	assert.NotNil(t, req.Thinking.OfDisabled, "GuardBeta should set OfDisabled when thinking is unset")
	assert.NotNil(t, base)
}

func TestGuardBeta_PreservesExistingThinking(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(userID)
	budget := int64(500)
	req.Thinking = anthropic.BetaThinkingConfigParamUnion{
		OfEnabled: &anthropic.BetaThinkingConfigEnabledParam{BudgetTokens: budget},
	}

	base, _ := c.GuardBeta(context.Background(), req)
	assert.NotNil(t, req.Thinking.OfEnabled, "GuardBeta should not overwrite existing thinking config")
	assert.Nil(t, req.Thinking.OfDisabled)
	assert.NotNil(t, base)
}

// ===================================================================
// Guard / GuardBeta — models that reject thinking.type.disabled
// ===================================================================

func TestGuard_UsesAdaptiveThinkingForModelsRejectingDisabled(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	for _, model := range []string{"claude-opus-5-5", "claude-fable-5", "claude-fable-5-1"} {
		for name, thinking := range map[string]anthropic.ThinkingConfigParamUnion{
			"unset":    {},
			"disabled": {OfDisabled: &anthropic.ThinkingConfigDisabledParam{}},
		} {
			req := &anthropic.MessageNewParams{
				Model:     anthropic.Model(model),
				MaxTokens: 512,
				Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
				Thinking:  thinking,
			}
			req.Metadata.UserID = param.NewOpt(userID)

			c.Guard(context.Background(), req)
			assert.NotNil(t, req.Thinking.OfAdaptive, "%s/%s: Guard should send adaptive thinking", model, name)
			assert.Nil(t, req.Thinking.OfDisabled, "%s/%s: Guard must not send disabled thinking", model, name)
		}
	}
}

func TestGuard_KeepsClientAdaptiveConfigForModelsRejectingDisabled(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-opus-5-5"),
		MaxTokens: 512,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{Display: anthropic.ThinkingConfigAdaptiveDisplayOmitted},
		},
	}
	req.Metadata.UserID = param.NewOpt(userID)

	c.Guard(context.Background(), req)
	require.NotNil(t, req.Thinking.OfAdaptive)
	assert.Equal(t, anthropic.ThinkingConfigAdaptiveDisplayOmitted, req.Thinking.OfAdaptive.Display)
}

func TestGuardBeta_UsesAdaptiveThinkingForModelsRejectingDisabled(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	for _, model := range []string{"claude-opus-5-5", "claude-fable-5", "claude-fable-5-1"} {
		req := &anthropic.BetaMessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: 512,
			Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
		}
		req.Metadata.UserID = param.NewOpt(userID)

		c.GuardBeta(context.Background(), req)
		assert.NotNil(t, req.Thinking.OfAdaptive, "%s: GuardBeta should send adaptive thinking", model)
		assert.Nil(t, req.Thinking.OfDisabled, "%s: GuardBeta must not send disabled thinking", model)
	}
}

func TestGuardBeta_KeepsClientAdaptiveConfigForModelsRejectingDisabled(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-fable-5"),
		MaxTokens: 512,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
		Thinking: anthropic.BetaThinkingConfigParamUnion{
			OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{Display: anthropic.BetaThinkingConfigAdaptiveDisplayOmitted},
		},
	}
	req.Metadata.UserID = param.NewOpt(userID)

	c.GuardBeta(context.Background(), req)
	require.NotNil(t, req.Thinking.OfAdaptive)
	assert.Equal(t, anthropic.BetaThinkingConfigAdaptiveDisplayOmitted, req.Thinking.OfAdaptive.Display)
}

func TestGuardBeta_StripsClearThinkingEditWhenThinkingDisabled(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(userID)
	// Thinking unset → GuardBeta forces it disabled. The clear_thinking edit must be dropped
	// to avoid "clear_thinking_20251015 requires `thinking` to be enabled or adaptive".
	req.ContextManagement.Edits = []anthropic.BetaContextManagementConfigEditUnionParam{
		{OfClearToolUses20250919: &anthropic.BetaClearToolUses20250919EditParam{}},
		{OfClearThinking20251015: &anthropic.BetaClearThinking20251015EditParam{}},
	}

	base, _ := c.GuardBeta(context.Background(), req)
	assert.NotNil(t, req.Thinking.OfDisabled)
	require.Len(t, req.ContextManagement.Edits, 1, "clear_thinking edit should be removed")
	assert.NotNil(t, req.ContextManagement.Edits[0].OfClearToolUses20250919, "other edits must be preserved")
	assert.NotNil(t, base)
}

func TestGuardBeta_KeepsClearThinkingEditWhenThinkingEnabled(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(userID)
	req.Thinking = anthropic.BetaThinkingConfigParamUnion{
		OfEnabled: &anthropic.BetaThinkingConfigEnabledParam{BudgetTokens: 500},
	}
	req.ContextManagement.Edits = []anthropic.BetaContextManagementConfigEditUnionParam{
		{OfClearThinking20251015: &anthropic.BetaClearThinking20251015EditParam{}},
	}

	base, _ := c.GuardBeta(context.Background(), req)
	assert.NotNil(t, req.Thinking.OfEnabled)
	require.Len(t, req.ContextManagement.Edits, 1, "clear_thinking edit must be kept when thinking is enabled")
	assert.NotNil(t, req.ContextManagement.Edits[0].OfClearThinking20251015)
	assert.NotNil(t, base)
}

func TestGuardBeta_KeepsClearThinkingEditWhenEffortSet(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-opus-4-7"),
		MaxTokens: 512,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(userID)
	// Effort-based adaptive thinking, no thinking union (newer models). GuardBeta must
	// not force-disable thinking nor strip the clear_thinking edit.
	req.OutputConfig.Effort = anthropic.BetaOutputConfigEffortMedium
	req.ContextManagement.Edits = []anthropic.BetaContextManagementConfigEditUnionParam{
		{OfClearThinking20251015: &anthropic.BetaClearThinking20251015EditParam{}},
	}

	base, _ := c.GuardBeta(context.Background(), req)
	assert.Nil(t, req.Thinking.OfDisabled, "effort-based thinking must not be force-disabled")
	require.Len(t, req.ContextManagement.Edits, 1, "clear_thinking edit must be kept when effort is set")
	assert.NotNil(t, req.ContextManagement.Edits[0].OfClearThinking20251015)
	assert.NotNil(t, base)
}

func TestGuardBeta_StripsClearThinkingWhenDisabledOverridesEffort(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-opus-4-7"),
		MaxTokens: 512,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(userID)
	// Contradictory input: explicit disabled wins, so the edit must still be dropped.
	req.OutputConfig.Effort = anthropic.BetaOutputConfigEffortMedium
	req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
	req.ContextManagement.Edits = []anthropic.BetaContextManagementConfigEditUnionParam{
		{OfClearThinking20251015: &anthropic.BetaClearThinking20251015EditParam{}},
	}

	base, _ := c.GuardBeta(context.Background(), req)
	require.Empty(t, req.ContextManagement.Edits, "explicit disabled must drop clear_thinking even when effort is set")
	assert.NotNil(t, base)
}

func TestGuard_DoesNotDisableThinkingWhenEffortSet(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-opus-4-7"),
		MaxTokens: 512,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
	}
	req.Metadata.UserID = param.NewOpt(userID)
	req.OutputConfig.Effort = anthropic.OutputConfigEffortMedium

	base, _ := c.Guard(context.Background(), req)
	assert.Nil(t, req.Thinking.OfDisabled, "effort-based thinking must not be force-disabled")
	assert.NotNil(t, base)
}

// ===================================================================
// Guard — tool remapping
// ===================================================================

func TestGuard_RemapsToolNames(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
		Tools: []anthropic.ToolUnionParam{
			{OfTool: &anthropic.ToolParam{Name: "bash"}},
		},
	}
	req.Metadata.UserID = param.NewOpt(userID)

	_, reverseMap := c.Guard(context.Background(), req)
	assert.Equal(t, "Bash", req.Tools[0].OfTool.Name)
	assert.Equal(t, map[string]string{"Bash": "bash"}, reverseMap)
}

func TestGuardBeta_RemapsToolNames(t *testing.T) {
	provider := newOAuthProvider()
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)

	userID := `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
		Tools: []anthropic.BetaToolUnionParam{
			{OfTool: &anthropic.BetaToolParam{Name: "read"}},
		},
	}
	req.Metadata.UserID = param.NewOpt(userID)

	_, reverseMap := c.GuardBeta(context.Background(), req)
	assert.Equal(t, "Read", req.Tools[0].OfTool.Name)
	assert.Equal(t, map[string]string{"Read": "read"}, reverseMap)
}
