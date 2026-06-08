package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
)

// ClaudeCodeCompatTransform normalizes "system" roles in the inbound Anthropic
// messages array so the request is valid for third-party Anthropic-compatible
// providers. Claude Code sends mid-conversation system-role entries (a
// non-standard extension over the bare user/assistant message contract); this
// transform rewrites them before forwarding so providers that reject the role
// do not error.
//
// Runs as a pre-Base stage; non-Anthropic inbound request types are left
// untouched. The transform is a thin shell — ops.ApplyClaudeCodeCompatRoleRewrite
// is the position-aware operation primitive (merge into the preceding user turn,
// or re-role to user) and this stage only decides when to invoke it based on the
// inbound request type.
type ClaudeCodeCompatTransform struct{}

// NewClaudeCodeCompatTransform returns a new ClaudeCodeCompatTransform.
func NewClaudeCodeCompatTransform() *ClaudeCodeCompatTransform {
	return &ClaudeCodeCompatTransform{}
}

func (t *ClaudeCodeCompatTransform) Name() string {
	return "claude_code_compat"
}

// Apply normalizes system-role messages for Anthropic request types.
func (t *ClaudeCodeCompatTransform) Apply(ctx *TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		ops.ApplyClaudeCodeCompatRoleRewrite(req)
	case *anthropic.BetaMessageNewParams:
		ops.ApplyClaudeCodeBetaCompatRoleRewrite(req)
	}
	return nil
}
