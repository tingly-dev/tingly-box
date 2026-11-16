package stream

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// chatToolCall is a tool call as a Chat client reassembles it from streamed
// deltas, keyed by tool_calls[].index.
type chatToolCall struct {
	ID, Name, Arguments string
}

func toolUseEvents(calls ...[2]string) []string {
	ev := []string{`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"x","usage":{"input_tokens":1,"output_tokens":0}}}`}
	for i, call := range calls {
		idx, _ := json.Marshal(i)
		args, _ := json.Marshal(`{"command":"echo ` + call[1] + `"}`)
		ev = append(ev,
			`{"type":"content_block_start","index":`+string(idx)+`,"content_block":{"type":"tool_use","id":"toolu_`+call[1]+`","name":"`+call[0]+`","input":{}}}`,
			`{"type":"content_block_delta","index":`+string(idx)+`,"delta":{"type":"input_json_delta","partial_json":`+string(args)+`}}`,
			`{"type":"content_block_stop","index":`+string(idx)+`}`,
		)
	}
	return append(ev,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
		`{"type":"message_stop"}`,
	)
}

// collectChatToolCalls runs the converter and merges tool call deltas by
// index, the way Chat clients (openai SDK, AI SDK) do.
func collectChatToolCalls(t *testing.T, conv *anthropicToOpenAIConverter) map[int]*chatToolCall {
	t.Helper()
	calls := map[int]*chatToolCall{}
	for {
		evt, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			return calls
		}
		chunk, ok := evt.(wire.ChatStreamChunk)
		if !ok {
			continue
		}
		for _, choice := range chunk.Choices {
			for _, tc := range choice.Delta.ToolCalls {
				call := calls[tc.Index]
				if call == nil {
					call = &chatToolCall{}
					calls[tc.Index] = call
				}
				if tc.ID != "" {
					call.ID = tc.ID
				}
				if tc.Function.Name != "" {
					call.Name = tc.Function.Name
				}
				if tc.Function.Arguments != nil {
					call.Arguments += *tc.Function.Arguments
				}
			}
		}
	}
}

func newToolIndexConverter(events []string, hooks *AnthropicToOpenAIMCPHooks) *anthropicToOpenAIConverter {
	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](newFakeAnthropicDecoder(events), nil)
	return newAnthropicToOpenAIConverter(stream, "m", false, hooks)
}

// Claude's parallel tool use emits several tool_use blocks in one turn; each
// must get its own tool_calls[].index or clients merge them into one call
// with concatenated (invalid) arguments (#1923).
func TestAnthropicToOpenAIChat_ParallelToolCallsGetDistinctIndexes(t *testing.T) {
	calls := collectChatToolCalls(t, newToolIndexConverter(toolUseEvents([2]string{"Bash", "one"}, [2]string{"Bash", "two"}), nil))

	require.Len(t, calls, 2)
	assert.Equal(t, chatToolCall{ID: "toolu_one", Name: "Bash", Arguments: `{"command":"echo one"}`}, *calls[0])
	assert.Equal(t, chatToolCall{ID: "toolu_two", Name: "Bash", Arguments: `{"command":"echo two"}`}, *calls[1])
}

// A tool call the gateway handles itself is never sent to the client, so it
// must not consume an index: the client-visible calls stay 0..n-1.
func TestAnthropicToOpenAIChat_SuppressedToolCallTakesNoIndex(t *testing.T) {
	hooks := &AnthropicToOpenAIMCPHooks{ShouldSuppressTool: func(name string) bool { return name == "web_search" }}
	calls := collectChatToolCalls(t, newToolIndexConverter(toolUseEvents([2]string{"web_search", "srv"}, [2]string{"Bash", "one"}), hooks))

	require.Len(t, calls, 1)
	require.Contains(t, calls, 0)
	assert.Equal(t, chatToolCall{ID: "toolu_one", Name: "Bash", Arguments: `{"command":"echo one"}`}, *calls[0])
}
