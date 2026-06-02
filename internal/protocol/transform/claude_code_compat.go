package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
)

// ClaudeCodeCompatTransform rewrites "system" roles in the inbound messages
// array to "user". Claude Code sends system-role entries inside the messages
// list (a non-standard extension); this transform normalizes them before
// forwarding so third-party providers that reject the non-standard role do not error.
//
// Runs as a pre-Base stage; non-Anthropic inbound request types are left untouched.
type ClaudeCodeCompatTransform struct{}

// NewClaudeCodeCompatTransform returns a new ClaudeCodeCompatTransform.
func NewClaudeCodeCompatTransform() *ClaudeCodeCompatTransform {
	return &ClaudeCodeCompatTransform{}
}

func (t *ClaudeCodeCompatTransform) Name() string {
	return "claude_code_compat"
}

// Apply rewrites system-role messages to user-role for Anthropic request types.
func (t *ClaudeCodeCompatTransform) Apply(ctx *TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		ops.ApplyClaudeCodeCompatRoleRewrite(req)
	case *anthropic.BetaMessageNewParams:
		ops.ApplyClaudeCodeBetaCompatRoleRewrite(req)
	}
	return nil
}
