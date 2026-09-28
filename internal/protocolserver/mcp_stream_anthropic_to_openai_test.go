package protocolserver

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	mcp "github.com/tingly-dev/tingly-box/internal/toolengine"
)

func TestAppendAnthropicBetaToolContinuation_ToolUseInputIsAlwaysAnObject(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{}
	calls := []stream.AnthropicToOpenAIToolCall{
		{ID: "call_blank", Name: "get_time", Arguments: ""},
		{ID: "call_null", Name: "get_time", Arguments: "null"},
		{ID: "call_args", Name: "get_weather", Arguments: `{"city":"NYC"}`},
	}
	results := []mcp.ToolExecutionResult{{ToolUseID: "call_blank"}, {ToolUseID: "call_null"}, {ToolUseID: "call_args"}}

	appendAnthropicBetaToolContinuation(req, calls, results)

	require.Len(t, req.Messages, 2)
	want := map[string]string{"call_blank": `{}`, "call_null": `{}`, "call_args": `{"city":"NYC"}`}
	for _, block := range req.Messages[0].Content {
		require.NotNil(t, block.OfToolUse)
		raw, err := json.Marshal(block.OfToolUse)
		require.NoError(t, err)
		var wire map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &wire))
		require.Contains(t, wire, "input", "%s: tool_use sent without input", block.OfToolUse.ID)
		assert.JSONEq(t, want[block.OfToolUse.ID], string(wire["input"]), block.OfToolUse.ID)
	}
}
