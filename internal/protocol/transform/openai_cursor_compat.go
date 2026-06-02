package transform

import (
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
)

// OpenAICursorCompatTransform flattens rich content blocks in OpenAI Chat
// messages for Cursor-style clients that only accept plain string content.
//
// It runs as a pre-Base stage so it sees the inbound request in its original
// shape — Cursor IDE always sends OpenAI Chat, so the type-switch matches and
// flattening happens before BaseTransform converts to the target shape.
// For non-Chat inbound shapes (Anthropic, Responses, Google) it is a no-op:
// Cursor compatibility is only meaningful when the source-of-truth message
// structure is OpenAI Chat.
//
// The transform is a thin shell — ops.ApplyCursorCompatContentNormalization is
// the operation primitive; this stage only decides when to invoke it based on
// the inbound request type.
//
// Only added to the chain when the rule's cursor_compat flag is enabled.
type OpenAICursorCompatTransform struct{}

// NewOpenAICursorCompatTransform creates a new pre-Base cursor compatibility
// transform.
func NewOpenAICursorCompatTransform() *OpenAICursorCompatTransform {
	return &OpenAICursorCompatTransform{}
}

func (t *OpenAICursorCompatTransform) Name() string {
	return "openai_cursor_compat"
}

// Apply flattens rich content on OpenAI Chat requests. For any other shape
// (Anthropic v1/beta, Responses, Google) it is a no-op.
func (t *OpenAICursorCompatTransform) Apply(ctx *TransformContext) error {
	req, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	if !ok {
		return nil
	}
	ops.ApplyCursorCompatContentNormalization(req)
	return nil
}
