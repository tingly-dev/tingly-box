package server

import (
	"github.com/anthropics/anthropic-sdk-go"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

func hasOnlyMCPToolUsesV1(content []anthropic.ContentBlockUnion) ([]anthropic.ToolUseBlock, bool) {
	toolUses := make([]anthropic.ToolUseBlock, 0, len(content))
	for _, block := range content {
		switch v := block.AsAny().(type) {
		case anthropic.ToolUseBlock:
			if !mcpruntime.IsMCPToolName(v.Name) {
				return nil, false
			}
			toolUses = append(toolUses, v)
		}
	}
	if len(toolUses) == 0 {
		return nil, false
	}
	return toolUses, true
}

func hasOnlyMCPToolUsesBeta(content []anthropic.BetaContentBlockUnion) ([]anthropic.BetaToolUseBlock, bool) {
	toolUses := make([]anthropic.BetaToolUseBlock, 0, len(content))
	for _, block := range content {
		switch v := block.AsAny().(type) {
		case anthropic.BetaToolUseBlock:
			if !mcpruntime.IsMCPToolName(v.Name) {
				return nil, false
			}
			toolUses = append(toolUses, v)
		}
	}
	if len(toolUses) == 0 {
		return nil, false
	}
	return toolUses, true
}

// HasDeclaredMCPAnthropicV1Tools reports whether req declares any MCP-named
// tool in its Anthropic v1 tool list. Moved here from root's
// protocol_dispatch.go (which still hosts its call sites, Step 8) because it
// is a pure function with zero *Server dependency, same class as its Beta
// sibling below.
func HasDeclaredMCPAnthropicV1Tools(req *anthropic.MessageNewParams) bool {
	if req == nil || len(req.Tools) == 0 {
		return false
	}
	for _, t := range req.Tools {
		if t.OfTool != nil && mcpruntime.IsMCPToolName(t.OfTool.Name) {
			return true
		}
	}
	return false
}

// HasDeclaredMCPAnthropicBetaTools reports whether req declares any MCP-named
// tool in its Anthropic beta tool list.
func HasDeclaredMCPAnthropicBetaTools(req *anthropic.BetaMessageNewParams) bool {
	// FIXME: we can not use such a simple logic to check
	if req == nil || len(req.Tools) == 0 {
		return false
	}

	for _, t := range req.Tools {
		if t.OfTool != nil && mcpruntime.IsMCPToolName(t.OfTool.Name) {
			return true
		}
	}

	return false
}
