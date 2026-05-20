package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
)

// ToolBlockTransform strips a configured set of tools from the request's tool
// list. It runs as a pre-Base stage so it sees the request in its inbound shape
// — the names it matches are the ones the client actually sent, before any
// protocol conversion or vendor-specific tool renaming.
//
// The transform is a thin shell: the ops.ApplyToolBlock* primitives do the
// per-shape filtering; this stage only type-switches the inbound request to the
// matching primitive. When no names are configured it is never added to the
// chain (see rulePreBaseTransforms).
type ToolBlockTransform struct {
	blocked map[string]bool
}

// NewToolBlockTransform builds a transform that blocks the given tool names.
// Names are matched exactly; empty entries are ignored.
func NewToolBlockTransform(names []string) *ToolBlockTransform {
	blocked := make(map[string]bool, len(names))
	for _, n := range names {
		if n != "" {
			blocked[n] = true
		}
	}
	return &ToolBlockTransform{blocked: blocked}
}

func (t *ToolBlockTransform) Name() string {
	return "tool_block"
}

// Apply filters the tool list on whichever inbound request shape is present.
func (t *ToolBlockTransform) Apply(ctx *TransformContext) error {
	if len(t.blocked) == 0 {
		return nil
	}
	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		ops.ApplyToolBlockOpenAIChat(req, t.blocked)
	case *responses.ResponseNewParams:
		ops.ApplyToolBlockResponses(req, t.blocked)
	case *anthropic.MessageNewParams:
		ops.ApplyToolBlockAnthropic(req, t.blocked)
	case *anthropic.BetaMessageNewParams:
		ops.ApplyToolBlockAnthropicBeta(req, t.blocked)
	case *protocol.GoogleRequest:
		ops.ApplyToolBlockGoogle(req, t.blocked)
	}
	return nil
}
