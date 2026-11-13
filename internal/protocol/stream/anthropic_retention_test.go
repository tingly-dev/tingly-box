package stream

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
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

func TestAnthropicToResponsesInitialToolInput(t *testing.T) {
	for _, tc := range []struct{ name, initial, delta, want string }{
		{"complete start input", `{"cmd":"printf hello"}`, "", `{"cmd":"printf hello"}`},
		{"empty arguments", `{}`, "", `{}`},
		{"large integer input", `{"id":9007199254740993}`, "", `{"id":9007199254740993}`},
		{"streamed input", `{}`, `{"cmd":"printf streamed"}`, `{"cmd":"printf streamed"}`},
		{"deltas replace start snapshot", `{"cmd":"old"}`, `{"cmd":"new"}`, `{"cmd":"new"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conv := newAnthropicBetaToResponsesConverter(nil, "model", nil)
			feed := func(raw string) {
				var event anthropic.BetaRawMessageStreamEventUnion
				require.NoError(t, json.Unmarshal([]byte(raw), &event))
				conv.processEvent(&event)
			}
			feed(`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[]}}`)
			feed(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"exec_command","input":` + tc.initial + `}}`)
			if tc.delta != "" {
				encoded, _ := json.Marshal(tc.delta)
				feed(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":` + string(encoded) + `}}`)
			}
			feed(`{"type":"content_block_stop","index":0}`)
			feed(`{"type":"message_stop"}`)
			var delta strings.Builder
			var doneArguments, completedArguments string
			for _, e := range conv.pending {
				switch e := e.(type) {
				case wire.ResponsesFunctionCallArgumentsDeltaEvent:
					delta.WriteString(e.Delta)
				case wire.ResponsesOutputItemDoneEvent:
					if e.Item.Arguments != nil {
						doneArguments = *e.Item.Arguments
					}
				case wire.ResponsesCompletedEvent:
					completedArguments = *e.Response.Output[0].Arguments
				}
			}
			require.JSONEq(t, tc.want, delta.String())
			require.JSONEq(t, tc.want, doneArguments)
			require.JSONEq(t, tc.want, completedArguments)
			if tc.name == "large integer input" {
				require.Contains(t, doneArguments, "9007199254740993")
				require.Contains(t, completedArguments, "9007199254740993")
			}
		})
	}
}
