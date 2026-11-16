package stream

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func retentionAnthropicStream(events []string) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	return anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](newFakeAnthropicDecoder(events), nil)
}

func TestAnthropicChatRetainsStopReasonAcrossUsageTail(t *testing.T) {
	for _, tool := range []bool{false, true} {
		for _, usageEnabled := range []bool{false, true} {
			name := "text"
			if tool {
				name = "tool"
			}
			if usageEnabled {
				name += "/usage"
			} else {
				name += "/no_usage"
			}
			t.Run(name, func(t *testing.T) {
				events := []string{`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`}
				block := `{"type":"text","text":"prefix"}`
				if tool {
					block = `{"type":"tool_use","id":"t1","name":"lookup","input":{"id":9007199254740993}}`
				}
				events = append(events, `{"type":"content_block_start","index":0,"content_block":`+block+`}`, `{"type":"content_block_stop","index":0}`,
					`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":4}}`,
					`{"type":"message_delta","delta":{},"usage":{"output_tokens":7}}`, `{"type":"message_stop"}`)
				upstream := retentionAnthropicStream(events)
				defer upstream.Close()
				conv := newAnthropicToOpenAIConverter(upstream, "model", !usageEnabled, nil)
				terminals := 0
				for i := 0; ; i++ {
					require.Less(t, i, 30)
					event, done, err := conv.Next()
					require.NoError(t, err)
					if done {
						break
					}
					chunk := event.(wire.ChatStreamChunk)
					if reason := chunk.Choices[0].FinishReason; reason != nil {
						terminals++
						require.Equal(t, "length", *reason)
						if usageEnabled {
							require.NotNil(t, chunk.Usage)
							require.EqualValues(t, 7, chunk.Usage.CompletionTokens)
						} else {
							require.Nil(t, chunk.Usage)
						}
					}
				}
				require.Equal(t, 1, terminals)
				require.Equal(t, 7, conv.Usage().OutputTokens)
			})
		}
	}
}

func TestAnthropicInitialTextSurvivesBothStreamRoutes(t *testing.T) {
	for _, suffix := range []string{"", " tail"} {
		t.Run(suffix, func(t *testing.T) {
			events := []string{`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[]}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"prefix"}}`}
			if suffix != "" {
				events = append(events, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" tail"}}`)
			}
			events = append(events, `{"type":"content_block_stop","index":0}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`, `{"type":"message_stop"}`)
			for _, route := range []string{"chat", "responses"} {
				t.Run(route, func(t *testing.T) {
					upstream := retentionAnthropicStream(events)
					defer upstream.Close()
					var conv StreamConverter
					if route == "chat" {
						conv = NewAnthropicBetaToOpenAIChatConverter(upstream, "model", false)
					} else {
						conv = NewAnthropicBetaToOpenAIResponsesConverter(upstream, "model", nil)
					}
					var streamed strings.Builder
					var snapshots []string
					for i := 0; ; i++ {
						require.Less(t, i, 30)
						event, done, err := conv.Next()
						require.NoError(t, err)
						if done {
							break
						}
						switch e := event.(type) {
						case wire.ChatStreamChunk:
							streamed.WriteString(e.Choices[0].Delta.Content)
						case wire.ResponsesOutputTextDeltaEvent:
							streamed.WriteString(e.Delta)
						case wire.ResponsesOutputTextDoneEvent:
							snapshots = append(snapshots, e.Text)
						case wire.ResponsesCompletedEvent:
							snapshots = append(snapshots, e.Response.Output[0].Content[0].Text)
						}
					}
					require.Equal(t, "prefix"+suffix, streamed.String())
					if route == "responses" {
						require.Equal(t, []string{"prefix" + suffix, "prefix" + suffix}, snapshots)
					}
				})
			}
		})
	}
}

func TestAnthropicChatInitialThinkingIsNotDuplicated(t *testing.T) {
	events := []string{`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[]}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"initial","signature":"opaque"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" tail"}}`,
		`{"type":"content_block_stop","index":0}`, `{"type":"message_stop"}`}
	upstream := retentionAnthropicStream(events)
	defer upstream.Close()
	conv := NewAnthropicBetaToOpenAIChatConverter(upstream, "model", false)
	var reasoning strings.Builder
	for i := 0; ; i++ {
		require.Less(t, i, 30)
		event, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		reasoning.WriteString(event.(wire.ChatStreamChunk).Choices[0].Delta.ReasoningContent)
	}
	require.Equal(t, "initial tail", reasoning.String())
}

func TestAnthropicChatInitialToolInputsStayIsolated(t *testing.T) {
	events := []string{
		`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[]}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"hidden","name":"server_lookup","input":{"id":9007199254740993}}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"visible","name":"lookup","input":{"id":1}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"id\":"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"9007199254740995}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"empty","name":"noop","input":{}}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	}
	var hookCalls []AnthropicToOpenAIToolCall
	hooks := &AnthropicToOpenAIMCPHooks{
		ShouldSuppressTool: func(name string) bool { return name == "server_lookup" },
		OnToolCallsFinal:   func(calls []AnthropicToOpenAIToolCall) error { hookCalls = append(hookCalls, calls...); return nil },
	}
	upstream := retentionAnthropicStream(events)
	defer upstream.Close()
	conv := newAnthropicToOpenAIConverter(upstream, "model", false, hooks)
	calls := collectChatToolCalls(t, conv)
	require.Len(t, calls, 2)
	require.Equal(t, chatToolCall{ID: "visible", Name: "lookup", Arguments: `{"id":9007199254740995}`}, *calls[0])
	require.Equal(t, chatToolCall{ID: "empty", Name: "noop", Arguments: `{}`}, *calls[1])
	require.Equal(t, []AnthropicToOpenAIToolCall{
		{ID: "hidden", Name: "server_lookup", Arguments: `{"id":9007199254740993}`},
		{ID: "visible", Name: "lookup", Arguments: `{"id":9007199254740995}`},
		{ID: "empty", Name: "noop", Arguments: `{}`},
	}, hookCalls)
}
