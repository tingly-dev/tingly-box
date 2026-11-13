package stream

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestAnthropicChatRetainsInitialInputAndTruncation(t *testing.T) {
	for _, delta := range []string{"", `{"id":9007199254740995}`} {
		t.Run(delta, func(t *testing.T) {
			c := newAnthropicToOpenAIConverter(nil, "model", false, nil)
			feed := func(raw string) {
				var e anthropic.BetaRawMessageStreamEventUnion
				require.NoError(t, json.Unmarshal([]byte(raw), &e))
				c.processEvent(&e)
			}
			feed(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"lookup","input":{"id":9007199254740993}}}`)
			want := `{"id":9007199254740993}`
			if delta != "" {
				encoded, _ := json.Marshal(delta)
				feed(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":` + string(encoded) + `}}`)
				want = delta
			}
			feed(`{"type":"content_block_stop","index":0}`)
			feed(`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":20}}`)
			feed(`{"type":"message_stop"}`)
			var args strings.Builder
			for _, e := range c.pending {
				chunk := e.(wire.ChatStreamChunk)
				for _, tc := range chunk.Choices[0].Delta.ToolCalls {
					if tc.Function.Arguments != nil {
						args.WriteString(*tc.Function.Arguments)
					}
				}
			}
			require.Equal(t, want, args.String())
			require.Equal(t, "length", *c.pending[len(c.pending)-1].(wire.ChatStreamChunk).Choices[0].FinishReason)
			require.Equal(t, want, c.pendingToolCalls[0].Arguments, "MCP execution must receive the same input")
		})
	}
}

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

func TestResponsesConversionRetainsFailureCode(t *testing.T) {
	for _, route := range []string{"chat", "anthropic"} {
		for _, eventType := range []string{"error", "response.failed"} {
			t.Run(route+"/"+eventType, func(t *testing.T) {
				event := map[string]any{"type": eventType, "code": "context_length_exceeded", "param": "input", "message": "too long"}
				if eventType == "response.failed" {
					event["response"] = map[string]any{"error": map[string]any{"code": "context_length_exceeded", "message": "too long"}}
				}
				var c StreamConverter
				if route == "chat" {
					c = NewOpenAIResponsesToChatConverter(newResponsesIterFromEvents([]map[string]any{event}), "model", false)
				} else {
					c = NewOpenAIResponsesToAnthropicConverter(context.Background(), newResponsesIterFromEvents([]map[string]any{event}), "model")
				}
				found := false
				for {
					e, done, err := c.Next()
					if err != nil || done {
						break
					}
					raw, _ := json.Marshal(e)
					if v, ok := AsAnthropicEvent(e); ok {
						raw = []byte(v.RawJSON())
					}
					if gjson.GetBytes(raw, "error.code").Exists() {
						found = true
						require.Equal(t, "context_length_exceeded", gjson.GetBytes(raw, "error.code").String())
						require.Equal(t, "too long", gjson.GetBytes(raw, "error.message").String())
					}
				}
				require.True(t, found)
			})
		}
	}
}

func TestChatAnthropicDoesNotSalvageExplicitFailureAfterFinish(t *testing.T) {
	for _, beta := range []bool{false, true} {
		raw := `data: {"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"stop"}]}` + "\n\n" + `data: {"error":{"code":"context_length_exceeded","message":"usage tail failed"}}` + "\n\n"
		var c StreamConverter
		if beta {
			c = NewOpenAIChatToAnthropicBetaConverter(newChatChunkStream(raw), "model", nil)
		} else {
			c = NewOpenAIChatToAnthropicV1Converter(newChatChunkStream(raw), "model", nil)
		}
		for {
			e, done, err := c.Next()
			if err != nil {
				require.ErrorContains(t, err, "usage tail failed")
				require.Equal(t, "context_length_exceeded", BuildErrorEvent(err, "stream_failed")["error"].(map[string]interface{})["code"])
				break
			}
			require.False(t, done)
			v, _ := AsAnthropicEvent(e)
			require.NotEqual(t, "message_stop", v.Type)
		}
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
