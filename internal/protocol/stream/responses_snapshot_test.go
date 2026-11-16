package stream

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestResponsesConversionRetainsDoneArguments(t *testing.T) {
	for _, route := range []string{"chat", "anthropic"} {
		for _, mode := range []string{"arguments_done", "item_done", "completed", "streamed", "partial"} {
			t.Run(route+"/"+mode, func(t *testing.T) {
				args := `{"id":9007199254740993}`
				item := map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "lookup", "arguments": args}
				events := []map[string]any{{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "lookup", "arguments": ""}}}
				if mode == "streamed" || mode == "partial" {
					delta := args
					if mode == "partial" {
						delta = `{"id":`
					}
					events = append(events, map[string]any{"type": "response.function_call_arguments.delta", "item_id": "fc_1", "output_index": 0, "delta": delta})
				}
				if mode == "arguments_done" || mode == "streamed" {
					events = append(events, map[string]any{"type": "response.function_call_arguments.done", "item_id": "fc_1", "output_index": 0, "arguments": args})
				}
				if mode == "item_done" {
					events = append(events, map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
				}
				events = append(events, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{item}}})
				var c StreamConverter
				if route == "chat" {
					c = NewOpenAIResponsesToChatConverter(newResponsesIterFromEvents(events), "model", false)
				} else {
					c = NewOpenAIResponsesToAnthropicConverter(context.Background(), newResponsesIterFromEvents(events), "model")
				}
				var actual strings.Builder
				starts, stops := 0, 0
				for {
					e, done, err := c.Next()
					require.NoError(t, err)
					if done {
						break
					}
					if route == "chat" {
						for _, tc := range e.(wire.ChatStreamChunk).Choices[0].Delta.ToolCalls {
							if tc.ID != "" {
								starts++
							}
							if tc.Function.Arguments != nil {
								actual.WriteString(*tc.Function.Arguments)
							}
						}
					} else {
						event, _ := AsAnthropicEvent(e)
						raw := event.RawJSON()
						if gjson.Get(raw, "content_block.type").String() == "tool_use" {
							starts++
						}
						if event.Type == "content_block_stop" {
							stops++
						}
						actual.WriteString(gjson.Get(raw, "delta.partial_json").String())
					}
				}
				require.Equal(t, args, actual.String(), "snapshots must fill missing arguments exactly once")
				require.Equal(t, 1, starts)
				if route == "anthropic" {
					require.Equal(t, 1, stops)
				}
			})
		}
	}
}

func TestResponsesConversionRetainsMixedFinalOutput(t *testing.T) {
	events := []map[string]any{
		{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "first"},
		{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{
			map[string]any{"type": "message", "id": "m1", "content": []any{map[string]any{"type": "output_text", "text": "first"}}},
			map[string]any{"type": "message", "id": "m2", "content": []any{map[string]any{"type": "output_text", "text": "second"}, map[string]any{"type": "refusal", "refusal": "cannot comply"}}},
			map[string]any{"type": "reasoning", "id": "r1", "summary": []any{map[string]any{"type": "summary_text", "text": "reason"}}},
			map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "lookup", "arguments": "{}"},
		}}},
	}
	for _, route := range []string{"chat", "anthropic"} {
		t.Run(route, func(t *testing.T) {
			var c StreamConverter
			if route == "chat" {
				c = NewOpenAIResponsesToChatConverter(newResponsesIterFromEvents(events), "model", false)
			} else {
				c = NewOpenAIResponsesToAnthropicConverter(context.Background(), newResponsesIterFromEvents(events), "model")
			}
			var text, reason, refusal, args strings.Builder
			for {
				e, done, err := c.Next()
				require.NoError(t, err)
				if done {
					break
				}
				if route == "chat" {
					d := e.(wire.ChatStreamChunk).Choices[0].Delta
					text.WriteString(d.Content)
					reason.WriteString(d.ReasoningContent)
					refusal.WriteString(d.Refusal)
					for _, tc := range d.ToolCalls {
						if tc.Function.Arguments != nil {
							args.WriteString(*tc.Function.Arguments)
						}
					}
				} else {
					v, _ := AsAnthropicEvent(e)
					raw := v.RawJSON()
					text.WriteString(gjson.Get(raw, "delta.text").String())
					reason.WriteString(gjson.Get(raw, "delta.thinking").String())
					args.WriteString(gjson.Get(raw, "delta.partial_json").String())
				}
			}
			if route == "chat" {
				require.Equal(t, "firstsecond", text.String())
				require.Equal(t, "cannot comply", refusal.String())
			} else {
				require.Equal(t, "firstsecondcannot comply", text.String())
			}
			require.Equal(t, "reason", reason.String())
			require.Equal(t, "{}", args.String())
		})
	}
}

func TestResponsesChatIncompleteToolCallKeepsTruncation(t *testing.T) {
	events := []map[string]any{{"type": "response.incomplete", "response": map[string]any{"status": "incomplete", "incomplete_details": map[string]any{"reason": "max_output_tokens"}, "output": []any{map[string]any{"type": "function_call", "id": "fc1", "call_id": "call1", "name": "lookup", "arguments": `{"id":`}}}}}
	c := NewOpenAIResponsesToChatConverter(newResponsesIterFromEvents(events), "model", false)
	var finish string
	for {
		e, done, err := c.Next()
		require.NoError(t, err)
		if done {
			break
		}
		if reason := e.(wire.ChatStreamChunk).Choices[0].FinishReason; reason != nil {
			finish = *reason
		}
	}
	require.Equal(t, "length", finish)
}
