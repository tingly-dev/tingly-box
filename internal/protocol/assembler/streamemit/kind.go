package streamemit

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// BlockKind is the assembler-level classification of a content block.
// We collapse the SDK's many tool-result variants into a single ToolUse
// kind because the routing decision is the same for all of them: hold
// until complete, or pass through.
type BlockKind uint8

const (
	BlockKindUnknown BlockKind = iota
	BlockKindText
	BlockKindThinking
	BlockKindToolUse
)

// decideBlockKindV1 inspects a v1 content_block_start payload and returns
// the BlockKind plus the tool_use id (empty for non-tool blocks).
func decideBlockKindV1(b anthropic.ContentBlockStartEventContentBlockUnion) (BlockKind, string) {
	switch b.Type {
	case "text":
		return BlockKindText, ""
	case "thinking", "redacted_thinking":
		return BlockKindThinking, ""
	case "tool_use",
		"server_tool_use",
		"web_search_tool_result",
		"web_fetch_tool_result",
		"code_execution_tool_result",
		"bash_code_execution_tool_result",
		"text_editor_code_execution_tool_result",
		"tool_search_tool_result",
		"container_upload":
		return BlockKindToolUse, b.ID
	default:
		return BlockKindUnknown, b.ID
	}
}

// decideBlockKindV1Beta is the v1beta variant of decideBlockKindV1.
func decideBlockKindV1Beta(b anthropic.BetaRawContentBlockStartEventContentBlockUnion) (BlockKind, string) {
	switch b.Type {
	case "text":
		return BlockKindText, ""
	case "thinking", "redacted_thinking":
		return BlockKindThinking, ""
	case "tool_use",
		"server_tool_use",
		"web_search_tool_result",
		"web_fetch_tool_result",
		"advisor_tool_result",
		"code_execution_tool_result",
		"bash_code_execution_tool_result",
		"text_editor_code_execution_tool_result",
		"tool_search_tool_result",
		"mcp_tool_use",
		"mcp_tool_result",
		"container_upload",
		"compaction":
		return BlockKindToolUse, b.ID
	default:
		return BlockKindUnknown, b.ID
	}
}
