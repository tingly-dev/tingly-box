package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ToolBlockTransform strips a configured set of tools from the request's tool
// list. It runs as a pre-Base stage so it sees the request in its inbound shape
// — the names it matches are the ones the client actually sent, before any
// protocol conversion or vendor-specific tool renaming.
//
// When no names are configured it is never added to the chain (see rulePreBaseTransforms).
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
		applyToolBlockOpenAIChat(req, t.blocked)
	case *responses.ResponseNewParams:
		applyToolBlockResponses(req, t.blocked)
	case *anthropic.MessageNewParams:
		applyToolBlockAnthropic(req, t.blocked)
	case *anthropic.BetaMessageNewParams:
		applyToolBlockAnthropicBeta(req, t.blocked)
	case *protocol.GoogleRequest:
		applyToolBlockGoogle(req, t.blocked)
	}
	return nil
}

// The applyToolBlock* functions drop every tool whose resolved name is present
// in the blocked set. Tools that carry no addressable name (vendor built-ins
// like web_search, computer use, code execution) are left untouched.
// When the filtered list becomes empty the field is reset to nil so the SDK's
// omitzero JSON tag drops it from the wire body instead of emitting "tools": [].

func applyToolBlockOpenAIChat(req *openai.ChatCompletionNewParams, blocked map[string]bool) {
	if req == nil || len(blocked) == 0 || len(req.Tools) == 0 {
		return
	}
	kept := make([]openai.ChatCompletionToolUnionParam, 0, len(req.Tools))
	for _, tool := range req.Tools {
		if name := openAIChatToolName(tool); name != "" && blocked[name] {
			continue
		}
		kept = append(kept, tool)
	}
	if len(kept) == 0 {
		req.Tools = nil
		return
	}
	req.Tools = kept
}

func openAIChatToolName(tool openai.ChatCompletionToolUnionParam) string {
	if tool.OfFunction != nil {
		return tool.OfFunction.Function.Name
	}
	if tool.OfCustom != nil {
		return tool.OfCustom.Custom.Name
	}
	return ""
}

func applyToolBlockResponses(req *responses.ResponseNewParams, blocked map[string]bool) {
	if req == nil || len(blocked) == 0 || len(req.Tools) == 0 {
		return
	}
	kept := make([]responses.ToolUnionParam, 0, len(req.Tools))
	for _, tool := range req.Tools {
		if tool.OfFunction != nil && blocked[tool.OfFunction.Name] {
			continue
		}
		kept = append(kept, tool)
	}
	if len(kept) == 0 {
		req.Tools = nil
		return
	}
	req.Tools = kept
}

func applyToolBlockAnthropic(req *anthropic.MessageNewParams, blocked map[string]bool) {
	if req == nil || len(blocked) == 0 || len(req.Tools) == 0 {
		return
	}
	kept := make([]anthropic.ToolUnionParam, 0, len(req.Tools))
	for _, tool := range req.Tools {
		if tool.OfTool != nil && blocked[tool.OfTool.Name] {
			continue
		}
		kept = append(kept, tool)
	}
	if len(kept) == 0 {
		req.Tools = nil
		return
	}
	req.Tools = kept
}

func applyToolBlockAnthropicBeta(req *anthropic.BetaMessageNewParams, blocked map[string]bool) {
	if req == nil || len(blocked) == 0 || len(req.Tools) == 0 {
		return
	}
	kept := make([]anthropic.BetaToolUnionParam, 0, len(req.Tools))
	for _, tool := range req.Tools {
		if tool.OfTool != nil && blocked[tool.OfTool.Name] {
			continue
		}
		kept = append(kept, tool)
	}
	if len(kept) == 0 {
		req.Tools = nil
		return
	}
	req.Tools = kept
}

func applyToolBlockGoogle(req *protocol.GoogleRequest, blocked map[string]bool) {
	if req == nil || req.Config == nil || len(blocked) == 0 || len(req.Config.Tools) == 0 {
		return
	}
	for _, tool := range req.Config.Tools {
		if tool == nil || len(tool.FunctionDeclarations) == 0 {
			continue
		}
		kept := tool.FunctionDeclarations[:0]
		for _, fd := range tool.FunctionDeclarations {
			if fd != nil && blocked[fd.Name] {
				continue
			}
			kept = append(kept, fd)
		}
		if len(kept) == 0 {
			tool.FunctionDeclarations = nil
			continue
		}
		tool.FunctionDeclarations = kept
	}
}
