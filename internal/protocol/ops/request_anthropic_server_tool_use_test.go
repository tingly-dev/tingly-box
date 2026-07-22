package ops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

const betaServerToolUseHistoryJSON = `{
	"model": "claude-sonnet-5",
	"max_tokens": 1024,
	"messages": [
		{"role": "user", "content": [{"type": "text", "text": "search something"}]},
		{"role": "assistant", "content": [
			{"type": "text", "text": "searching"},
			{"type": "thinking", "thinking": "t", "signature": "s"},
			{"type": "server_tool_use", "id": "call_abc-123", "name": "web_search", "input": {"query": "q"}}
		]},
		{"role": "user", "content": [
			{"type": "web_search_tool_result", "tool_use_id": "call_abc-123", "content": []}
		]}
	]
}`

func TestSanitizeAnthropicBetaServerToolUseIDs_RewritesInvalidIDAndRemapsResult(t *testing.T) {
	var req anthropic.BetaMessageNewParams
	if err := json.Unmarshal([]byte(betaServerToolUseHistoryJSON), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	SanitizeAnthropicBetaServerToolUseIDs(&req)

	stu := req.Messages[1].Content[2].OfServerToolUse
	if stu == nil {
		t.Fatalf("server_tool_use block lost after sanitize")
	}
	if !serverToolUseIDPattern.MatchString(stu.ID) {
		t.Fatalf("server_tool_use id %q still invalid", stu.ID)
	}
	if !strings.Contains(stu.ID, "call_abc") {
		t.Fatalf("rewritten id %q lost the original correlation hint", stu.ID)
	}

	result := req.Messages[2].Content[0].OfWebSearchToolResult
	if result == nil {
		t.Fatalf("web_search_tool_result block lost after sanitize")
	}
	if result.ToolUseID != stu.ID {
		t.Fatalf("tool_use_id %q not remapped to %q", result.ToolUseID, stu.ID)
	}

	// The sanitized request must serialize with the new IDs.
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if strings.Contains(string(b), "call_abc-123") {
		t.Fatalf("serialized request still contains the invalid id: %s", b)
	}
}

func TestSanitizeAnthropicBetaServerToolUseIDs_KeepsValidIDs(t *testing.T) {
	req := anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			betaAssistantMessage(
				anthropic.NewBetaServerToolUseBlock("srvtoolu_01AbC", map[string]any{"query": "q"}, "web_search"),
			),
		},
	}

	SanitizeAnthropicBetaServerToolUseIDs(&req)

	if got := req.Messages[0].Content[0].OfServerToolUse.ID; got != "srvtoolu_01AbC" {
		t.Fatalf("valid id was rewritten to %q", got)
	}
}

func TestSanitizeAnthropicBetaServerToolUseIDs_EmptyIDsGetDistinctIDs(t *testing.T) {
	req := anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			betaAssistantMessage(
				anthropic.NewBetaServerToolUseBlock("", nil, "web_search"),
				anthropic.NewBetaServerToolUseBlock("", nil, "web_search"),
			),
		},
	}

	SanitizeAnthropicBetaServerToolUseIDs(&req)

	first := req.Messages[0].Content[0].OfServerToolUse.ID
	second := req.Messages[0].Content[1].OfServerToolUse.ID
	if !serverToolUseIDPattern.MatchString(first) || !serverToolUseIDPattern.MatchString(second) {
		t.Fatalf("empty ids not rewritten: %q, %q", first, second)
	}
	if first == second {
		t.Fatalf("empty ids collided: %q", first)
	}
}

func TestSanitizeAnthropicV1ServerToolUseIDs_RewritesInvalidIDAndRemapsResult(t *testing.T) {
	req := anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{
			anthropic.NewAssistantMessage(
				anthropic.NewServerToolUseBlock("toolu_xyz.9", map[string]any{"query": "q"}, "web_search"),
			),
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfWebSearchToolResult: &anthropic.WebSearchToolResultBlockParam{ToolUseID: "toolu_xyz.9"}},
				},
			},
		},
	}

	SanitizeAnthropicV1ServerToolUseIDs(&req)

	stu := req.Messages[0].Content[0].OfServerToolUse
	if !serverToolUseIDPattern.MatchString(stu.ID) {
		t.Fatalf("server_tool_use id %q still invalid", stu.ID)
	}
	if got := req.Messages[1].Content[0].OfWebSearchToolResult.ToolUseID; got != stu.ID {
		t.Fatalf("tool_use_id %q not remapped to %q", got, stu.ID)
	}
}

func TestSanitizeAnthropicV1ServerToolUseIDs_SameOldIDMapsToSameNewID(t *testing.T) {
	req := anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{
			anthropic.NewAssistantMessage(
				anthropic.NewServerToolUseBlock("call_1", nil, "web_search"),
			),
			anthropic.NewAssistantMessage(
				anthropic.NewServerToolUseBlock("call_1", nil, "web_search"),
			),
		},
	}

	SanitizeAnthropicV1ServerToolUseIDs(&req)

	first := req.Messages[0].Content[0].OfServerToolUse.ID
	second := req.Messages[1].Content[0].OfServerToolUse.ID
	if first != second {
		t.Fatalf("same old id mapped to different new ids: %q vs %q", first, second)
	}
}

func betaAssistantMessage(blocks ...anthropic.BetaContentBlockParamUnion) anthropic.BetaMessageParam {
	return anthropic.BetaMessageParam{
		Role:    anthropic.BetaMessageParamRoleAssistant,
		Content: blocks,
	}
}
