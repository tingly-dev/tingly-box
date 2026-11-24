package nonstream

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
)

func TestAnthropicConversionRetainsToolPrecisionAndThinking(t *testing.T) {
	var msg anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(`{"id":"m1","type":"message","role":"assistant","stop_reason":"tool_use","content":[{"type":"thinking","thinking":"visible thought","signature":"opaque"},{"type":"tool_use","id":"t1","name":"lookup","input":{"id":9007199254740993}}]}`), &msg))
	chat := ConvertAnthropicBetaToOpenAIChat(&msg, "model")
	require.Equal(t, `{"id":9007199254740993}`, chat.Choices[0].Message.ToolCalls[0].Function.Arguments)
	require.Equal(t, "visible thought", chat.Choices[0].Message.ReasoningContent)
	resp := ConvertAnthropicBetaToResponsesWire(&msg, "model", "model", nil)
	require.Equal(t, `{"id":9007199254740993}`, *resp.Output[0].Arguments)
}

func TestAnthropicConversionUsesCurrentRawToolInput(t *testing.T) {
	for _, input := range []json.RawMessage{json.RawMessage(`{"id":9007199254740995}`), nil, json.RawMessage(`null`)} {
		t.Run(string(input), func(t *testing.T) {
			var msg anthropic.BetaMessage
			require.NoError(t, json.Unmarshal([]byte(`{"id":"m1","type":"message","role":"assistant","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t1","name":"lookup","input":{"id":1}}]}`), &msg))
			// RawJSON remains the original wire snapshot when the current SDK field changes.
			msg.Content[0].Input = input
			want := string(input)
			if len(input) == 0 || want == "null" {
				want = "{}"
			}
			chat := ConvertAnthropicBetaToOpenAIChat(&msg, "model")
			require.Equal(t, want, chat.Choices[0].Message.ToolCalls[0].Function.Arguments)
			response := ConvertAnthropicBetaToResponsesWire(&msg, "model", "model", nil)
			require.Equal(t, want, *response.Output[0].Arguments)
		})
	}
}
