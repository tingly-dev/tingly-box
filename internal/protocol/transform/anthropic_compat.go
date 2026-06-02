package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
)

// AnthropicCompatTransform rewrites "system" roles in the inbound messages
// array to "user". It runs as a pre-Base stage so it sees the request in its
// original Anthropic shape before any protocol conversion.
//
// Non-Anthropic inbound request types are left untouched.
type AnthropicCompatTransform struct{}

// NewAnthropicCompatTransform returns a new AnthropicCompatTransform.
func NewAnthropicCompatTransform() *AnthropicCompatTransform {
	return &AnthropicCompatTransform{}
}

func (t *AnthropicCompatTransform) Name() string {
	return "anthropic_compat"
}

// Apply rewrites system-role messages to user-role for Anthropic request types.
func (t *AnthropicCompatTransform) Apply(ctx *TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		ops.ApplyAnthropicCompatRoleRewrite(req)
	case *anthropic.BetaMessageNewParams:
		ops.ApplyAnthropicBetaCompatRoleRewrite(req)
	}
	return nil
}
