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
