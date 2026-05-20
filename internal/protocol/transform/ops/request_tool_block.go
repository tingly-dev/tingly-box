package ops

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// The ApplyToolBlock* primitives drop every tool whose resolved name is present
// in the `blocked` set from a request's tool list. They are pure functions over
// a single concrete request type — no rule, chain, or ctx awareness. Tools that
// carry no addressable name (vendor built-ins like web_search, computer use,
// code execution) are left untouched: the block list addresses model-callable
// function/custom tools by name.
//
// When the filtered list becomes empty the field is reset to nil so the SDK's
// omitzero JSON tag drops it from the wire body instead of emitting an empty
// `"tools": []`.

// ApplyToolBlockOpenAIChat filters tools on an OpenAI Chat request.
func ApplyToolBlockOpenAIChat(req *openai.ChatCompletionNewParams, blocked map[string]bool) {
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

// ApplyToolBlockResponses filters tools on an OpenAI Responses request.
func ApplyToolBlockResponses(req *responses.ResponseNewParams, blocked map[string]bool) {
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

// ApplyToolBlockAnthropic filters tools on an Anthropic Messages request.
func ApplyToolBlockAnthropic(req *anthropic.MessageNewParams, blocked map[string]bool) {
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

// ApplyToolBlockAnthropicBeta filters tools on an Anthropic Beta Messages request.
func ApplyToolBlockAnthropicBeta(req *anthropic.BetaMessageNewParams, blocked map[string]bool) {
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

// ApplyToolBlockGoogle filters function declarations on a Google request.
func ApplyToolBlockGoogle(req *protocol.GoogleRequest, blocked map[string]bool) {
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
