package protocolserver

import (
	"github.com/openai/openai-go/v3"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

func hasOnlyMCPToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) bool {
	if len(toolCalls) == 0 {
		return false
	}
	for _, tc := range toolCalls {
		if !mcpruntime.IsMCPToolName(tc.Function.Name) {
			return false
		}
	}
	return true
}

// HasDeclaredMCPTools reports whether req declares any MCP-named tool in its
// OpenAI Chat tool list.
func HasDeclaredMCPTools(req *openai.ChatCompletionNewParams) bool {
	if req == nil || len(req.Tools) == 0 {
		return false
	}
	for _, t := range req.Tools {
		fn := t.GetFunction()
		if fn == nil {
			continue
		}
		if mcpruntime.IsMCPToolName(fn.Name) {
			return true
		}
	}
	return false
}
