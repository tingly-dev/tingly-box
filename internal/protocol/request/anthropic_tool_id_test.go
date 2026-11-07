package request

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

func TestAnthropicToolIDsRemainPaired(t *testing.T) {
	ids := []string{"short", strings.Repeat("a", 40), strings.Repeat("x", 40) + "first", strings.Repeat("x", 40) + "second", strings.Repeat("界", 14)}
	input := struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []any  `json:"messages"`
	}{Model: "m", MaxTokens: 100}
	var calls, results []any
	for _, id := range ids {
		calls = append(calls, map[string]any{"type": "tool_use", "id": id, "name": "test", "input": map[string]any{}})
	}
	for i := len(ids) - 1; i >= 0; i-- {
		results = append(results, map[string]any{"type": "tool_result", "tool_use_id": ids[i], "content": "ok"})
	}
	input.Messages = []any{map[string]any{"role": "assistant", "content": calls}, map[string]any{"role": "user", "content": results}}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"v1", "beta"} {
		t.Run(variant, func(t *testing.T) {
			var out *openai.ChatCompletionNewParams
			if variant == "v1" {
				var req anthropic.MessageNewParams
				if err := json.Unmarshal(raw, &req); err != nil {
					t.Fatal(err)
				}
				out, _ = ConvertAnthropicToOpenAIRequest(&req, false, false, false)
			} else {
				var req anthropic.BetaMessageNewParams
				if err := json.Unmarshal(raw, &req); err != nil {
					t.Fatal(err)
				}
				out, _ = ConvertAnthropicBetaToOpenAIRequest(&req, false, false, false)
			}
			if len(out.Messages) != len(ids)+1 || out.Messages[0].OfAssistant == nil {
				t.Fatalf("unexpected messages: %+v", out.Messages)
			}
			seen := map[string]bool{}
			for i, call := range out.Messages[0].OfAssistant.ToolCalls {
				id := call.OfFunction.ID
				if len(id) > 40 || seen[id] {
					t.Errorf("invalid or duplicate call ID %q", id)
				}
				seen[id] = true
				if i < 2 && id != ids[i] {
					t.Errorf("short ID changed: %q", id)
				}
				if out.Messages[len(ids)-i].OfTool == nil || out.Messages[len(ids)-i].OfTool.ToolCallID != id {
					t.Errorf("call %q has no matching result", id)
				}
			}
			if len(out.Messages[0].OfAssistant.ToolCalls) != len(ids) {
				t.Fatal("missing calls")
			}
		})
	}
}

func TestAnthropicToolIDMappingReservesShortIDs(t *testing.T) {
	long := strings.Repeat("long", 12)
	first := []anthropic.MessageParam{anthropic.NewAssistantMessage(anthropic.NewToolUseBlock(long, map[string]any{}, "test"))}
	short := mapAnthropicToolIDs(first)[long]
	messages := []anthropic.MessageParam{
		anthropic.NewAssistantMessage(anthropic.NewToolUseBlock(long, map[string]any{}, "test"), anthropic.NewToolUseBlock(short, map[string]any{}, "test")),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock(long, "ok", false), anthropic.NewToolResultBlock(short, "ok", false)),
	}
	before, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := ConvertAnthropicToOpenAIRequest(&anthropic.MessageNewParams{Model: "m", MaxTokens: 100, Messages: messages}, false, false, false)
	calls := out.Messages[0].OfAssistant.ToolCalls
	if calls[0].OfFunction.ID == short || calls[1].OfFunction.ID != short {
		t.Fatalf("short ID was not reserved: %+v", calls)
	}
	for i, call := range calls {
		if out.Messages[i+1].OfTool.ToolCallID != call.OfFunction.ID {
			t.Fatal("call and result differ")
		}
	}
	after, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("conversion modified the input messages")
	}
}
