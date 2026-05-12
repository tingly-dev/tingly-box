package transform

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// NativeWebSearchStripTransform removes native web_search / web_fetch tool definitions
// from upstream requests when tingly-box's own MCP equivalents are enabled and ready.
//
// This prevents the LLM from selecting Anthropic's built-in web tools (which require a
// Claude Code subscription) when our MCP-backed versions are available via Serper/Jina.
type NativeWebSearchStripTransform struct {
	runtime *runtime.Runtime
}

func NewNativeWebSearchStripTransform(rt *runtime.Runtime) *NativeWebSearchStripTransform {
	return &NativeWebSearchStripTransform{runtime: rt}
}

func (t *NativeWebSearchStripTransform) Name() string { return "native_websearch_strip" }

func (t *NativeWebSearchStripTransform) Apply(ctx *protocoltransform.TransformContext) error {
	if t.runtime == nil {
		return nil
	}

	stripSearch := t.runtime.IsClientToolAvailable(mcptools.BuiltinWebtoolsSourceID, mcptools.BuiltinWebSearchToolName)
	stripFetch := t.runtime.IsClientToolAvailable(mcptools.BuiltinWebtoolsSourceID, mcptools.BuiltinWebFetchToolName)

	if !stripSearch && !stripFetch {
		return nil
	}

	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		req.Tools = stripOpenAIWebTools(req.Tools, stripSearch, stripFetch)
	case *anthropic.MessageNewParams:
		req.Tools = stripAnthropicV1WebTools(req.Tools, stripSearch, stripFetch)
	case *anthropic.BetaMessageNewParams:
		req.Tools = stripAnthropicBetaWebTools(req.Tools, stripSearch, stripFetch)
	}

	return nil
}

// isNativeWebToolName returns true for the bare names used by OpenAI-style native web tools.
// Handles both snake_case ("web_search") and CamelCase ("WebSearch") variants.
func isNativeWebToolName(name string, stripSearch, stripFetch bool) bool {
	lower := strings.ToLower(name)
	if stripSearch && (lower == "web_search" || lower == "websearch") {
		return true
	}
	if stripFetch && (lower == "web_fetch" || lower == "webfetch") {
		return true
	}
	return false
}

func stripOpenAIWebTools(tools []openai.ChatCompletionToolUnionParam, stripSearch, stripFetch bool) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return tools
	}
	out := tools[:0:len(tools)]
	for _, tool := range tools {
		fn := tool.GetFunction()
		if fn != nil && isNativeWebToolName(fn.Name, stripSearch, stripFetch) {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func stripAnthropicV1WebTools(tools []anthropic.ToolUnionParam, stripSearch, stripFetch bool) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return tools
	}
	out := tools[:0:len(tools)]
	for _, tool := range tools {
		if stripSearch && (tool.OfWebSearchTool20250305 != nil || tool.OfWebSearchTool20260209 != nil) {
			continue
		}
		if stripFetch && (tool.OfWebFetchTool20250910 != nil || tool.OfWebFetchTool20260209 != nil || tool.OfWebFetchTool20260309 != nil) {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func stripAnthropicBetaWebTools(tools []anthropic.BetaToolUnionParam, stripSearch, stripFetch bool) []anthropic.BetaToolUnionParam {
	if len(tools) == 0 {
		return tools
	}
	out := tools[:0:len(tools)]
	for _, tool := range tools {
		if stripSearch && (tool.OfWebSearchTool20250305 != nil || tool.OfWebSearchTool20260209 != nil) {
			continue
		}
		if stripFetch && (tool.OfWebFetchTool20250910 != nil || tool.OfWebFetchTool20260209 != nil || tool.OfWebFetchTool20260309 != nil) {
			continue
		}
		out = append(out, tool)
	}
	return out
}
