package protocolserver

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestApplyClaudeCodeClientHints_AttachesBetasAndAgentHeaders(t *testing.T) {
	c := newGinContext(t)
	c.Request.Header.Add("anthropic-beta", "claude-code-20250219, oauth-2025-04-20,per-turn-control-2026-07-01")
	c.Request.Header.Add("anthropic-beta", "fast-mode-2026-02-01")
	c.Request.Header.Set("x-claude-code-agent-id", " agent-7 ")
	c.Request.Header.Set("x-claude-code-parent-agent-id", "agent-main")
	c.Request.Header.Set("x-claude-code-request-class", "subagent")
	c.Request.Header.Set("x-claude-code-agent-type", "explore")
	c.Request.Header.Set("x-app", "cli-bg")

	applyClaudeCodeClientHints(c)

	got := typ.GetClaudeCodeClientHints(c.Request.Context())
	assert.Equal(t, []string{"claude-code-20250219", "oauth-2025-04-20", "per-turn-control-2026-07-01", "fast-mode-2026-02-01"}, got.Betas)
	assert.Equal(t, "agent-7", got.AgentID)
	assert.Equal(t, "agent-main", got.ParentAgentID)
	assert.Equal(t, "subagent", got.RequestClass)
	assert.Equal(t, "explore", got.AgentType)
	assert.True(t, got.BackgroundSession)
}

func TestApplyClaudeCodeClientHints_InteractiveXAppIsNotAHint(t *testing.T) {
	c := newGinContext(t)
	c.Request.Header.Set("x-app", "cli")

	applyClaudeCodeClientHints(c)

	assert.True(t, typ.GetClaudeCodeClientHints(c.Request.Context()).IsZero())
}

func TestApplyClaudeCodeClientHints_NoHeadersIsNoOp(t *testing.T) {
	c := newGinContext(t)

	applyClaudeCodeClientHints(c)

	assert.True(t, typ.GetClaudeCodeClientHints(c.Request.Context()).IsZero())
}

func TestApplyClaudeCodeClientHints_NilRequestIsNoOp(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	applyClaudeCodeClientHints(c) // must not panic
}

// The decoder stashes the body members the typed params drop; the hints carry
// them to the Claude OAuth client together with the 2.1.328 request headers.
func TestApplyClaudeCodeClientHints_CarriesBodyExtrasAndFirstPartyHeaders(t *testing.T) {
	c := newGinContext(t)
	c.Request.Header.Set("x-claude-1p-request-class", "background-task")
	c.Request.Header.Set("x-claude-code-prompt-id", "569cda3b-ee9e-4129-a332-48aed6b554a3")
	stashClaudeCodeBodyExtras(c, []byte(`{"model":"m","resumption":{"type":"enabled"},"tags":{"a":"b"},"cache_control":{"type":"ephemeral","evict_on_complete":true}}`))

	applyClaudeCodeClientHints(c)

	got := typ.GetClaudeCodeClientHints(c.Request.Context())
	assert.Equal(t, "background-task", got.FirstPartyRequestClass)
	assert.Equal(t, "569cda3b-ee9e-4129-a332-48aed6b554a3", got.PromptID)
	assert.JSONEq(t, `{"type":"enabled"}`, string(got.BodyExtras["resumption"]))
	assert.Contains(t, string(got.BodyExtras["cache_control"]), "evict_on_complete")
	assert.NotContains(t, got.BodyExtras, "tags", "tags are gateway-only and never replayed")
}

func TestApplyClaudeCodeClientHints_NoExtrasIsNoOp(t *testing.T) {
	c := newGinContext(t)
	stashClaudeCodeBodyExtras(c, []byte(`{"model":"m"}`))
	applyClaudeCodeClientHints(c)
	assert.True(t, typ.GetClaudeCodeClientHints(c.Request.Context()).IsZero())
}
