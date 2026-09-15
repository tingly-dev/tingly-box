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

	applyClaudeCodeClientHints(c)

	got := typ.GetClaudeCodeClientHints(c.Request.Context())
	assert.Equal(t, []string{"claude-code-20250219", "oauth-2025-04-20", "per-turn-control-2026-07-01", "fast-mode-2026-02-01"}, got.Betas)
	assert.Equal(t, "agent-7", got.AgentID)
	assert.Equal(t, "agent-main", got.ParentAgentID)
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
