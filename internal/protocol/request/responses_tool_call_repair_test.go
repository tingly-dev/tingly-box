package request

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// repairKinds renders the repaired item list as a compact shape string such as
// "user fc:call_a fc:call_b fco:call_a fco:call_b user" for easy assertions.
func repairKinds(t *testing.T, items responses.ResponseInputParam) []string {
	t.Helper()
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch {
		case it.OfMessage != nil:
			out = append(out, string(it.OfMessage.Role))
		case it.OfFunctionCall != nil:
			out = append(out, "fc:"+it.OfFunctionCall.CallID)
		case it.OfFunctionCallOutput != nil:
			out = append(out, "fco:"+it.OfFunctionCallOutput.CallID.Value)
		case it.OfReasoning != nil:
			out = append(out, "reasoning")
		default:
			out = append(out, "other")
		}
	}
	return out
}

func repairOutputText(t *testing.T, it responses.ResponseInputItemUnionParam) string {
	t.Helper()
	require.NotNil(t, it.OfFunctionCallOutput)
	return it.OfFunctionCallOutput.Output.OfString.Value
}

func repairUserText(t *testing.T, it responses.ResponseInputItemUnionParam) string {
	t.Helper()
	require.NotNil(t, it.OfMessage)
	list := it.OfMessage.Content.OfInputItemContentList
	require.NotEmpty(t, list)
	require.NotNil(t, list[0].OfInputText)
	return list[0].OfInputText.Text
}

// parseCodexInput runs raw Codex-style JSON through the same path as the
// server: PreprocessInputData, then SDK unmarshal.
func parseCodexInput(t *testing.T, body string) responses.ResponseInputParam {
	t.Helper()
	pre, err := protocol.PreprocessInputData([]byte(body))
	require.NoError(t, err)
	var p responses.ResponseNewParams
	require.NoError(t, json.Unmarshal(pre, &p))
	return p.Input.OfInputItemList
}

func TestRepairResponsesToolCalls(t *testing.T) {
	t.Run("well-formed input is unchanged", func(t *testing.T) {
		items := parseCodexInput(t, `{"input":[
		 {"type":"message","role":"user","content":"run both"},
		 {"type":"reasoning","id":"rs_1","summary":[]},
		 {"type":"function_call","call_id":"call_a","name":"shell","arguments":"{}"},
		 {"type":"function_call","call_id":"call_b","name":"shell","arguments":"{}"},
		 {"type":"function_call_output","call_id":"call_a","output":"a"},
		 {"type":"function_call_output","call_id":"call_b","output":"b"},
		 {"type":"message","role":"user","content":"next"}]}`)
		out := RepairResponsesToolCalls(items)
		assert.Equal(t, []string{"user", "reasoning", "fc:call_a", "fc:call_b", "fco:call_a", "fco:call_b", "user"}, repairKinds(t, out))
	})

	t.Run("parallel calls interrupted after the first output get a placeholder", func(t *testing.T) {
		// The exact history behind DeepSeek's "insufficient tool messages
		// following tool_calls message".
		items := parseCodexInput(t, `{"input":[
		 {"type":"message","role":"user","content":"run both"},
		 {"type":"function_call","call_id":"call_a","name":"shell","arguments":"{}"},
		 {"type":"function_call","call_id":"call_b","name":"shell","arguments":"{}"},
		 {"type":"function_call_output","call_id":"call_a","output":"a"},
		 {"type":"message","role":"user","content":"stop"}]}`)
		out := RepairResponsesToolCalls(items)
		require.Equal(t, []string{"user", "fc:call_a", "fc:call_b", "fco:call_a", "fco:call_b", "user"}, repairKinds(t, out))
		assert.Equal(t, "a", repairOutputText(t, out[3]))
		assert.Equal(t, missingToolOutputPlaceholder, repairOutputText(t, out[4]))
	})

	t.Run("trailing call without output is answered", func(t *testing.T) {
		items := parseCodexInput(t, `{"input":[
		 {"type":"message","role":"user","content":"run"},
		 {"type":"function_call","call_id":"call_a","name":"shell","arguments":"{}"}]}`)
		out := RepairResponsesToolCalls(items)
		require.Equal(t, []string{"user", "fc:call_a", "fco:call_a"}, repairKinds(t, out))
		assert.Equal(t, missingToolOutputPlaceholder, repairOutputText(t, out[2]))
	})

	t.Run("output separated from its call is moved next to it", func(t *testing.T) {
		items := parseCodexInput(t, `{"input":[
		 {"type":"function_call","call_id":"call_a","name":"shell","arguments":"{}"},
		 {"type":"message","role":"user","content":"interjection"},
		 {"type":"function_call_output","call_id":"call_a","output":"late"}]}`)
		out := RepairResponsesToolCalls(items)
		require.Equal(t, []string{"fc:call_a", "fco:call_a", "user"}, repairKinds(t, out))
		assert.Equal(t, "late", repairOutputText(t, out[1]))
	})

	t.Run("codex automation orphan output without call_id becomes user text", func(t *testing.T) {
		items := parseCodexInput(t, `{"input":[
		 {"type":"function_call_output","id":"fco_01","name":"automation_update","output":"automation: nightly"},
		 {"type":"message","role":"user","content":"do the task"}]}`)
		out := RepairResponsesToolCalls(items)
		require.Equal(t, []string{"user", "user"}, repairKinds(t, out))
		assert.Equal(t, "[tool output: automation_update]\nautomation: nightly", repairUserText(t, out[0]))
	})

	t.Run("orphan output with call_id but no call becomes user text", func(t *testing.T) {
		items := parseCodexInput(t, `{"input":[
		 {"type":"function_call_output","call_id":"call_old","output":"stale"},
		 {"type":"message","role":"user","content":"next"}]}`)
		out := RepairResponsesToolCalls(items)
		require.Equal(t, []string{"user", "user"}, repairKinds(t, out))
		assert.Equal(t, "[tool output: call_old]\nstale", repairUserText(t, out[0]))
	})

	t.Run("orphan output with image content keeps the image", func(t *testing.T) {
		items := responses.ResponseInputParam{{
			OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
				Name: param.NewOpt("screenshot"),
				Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
					OfResponseFunctionCallOutputItemArray: []responses.ResponseFunctionCallOutputItemUnionParam{
						{OfInputText: &responses.ResponseInputTextContentParam{Text: "captured"}},
						{OfInputImage: &responses.ResponseInputImageContentParam{ImageURL: param.NewOpt("data:image/png;base64,AAAA")}},
					},
				},
			},
		}}
		out := RepairResponsesToolCalls(items)
		require.Equal(t, []string{"user"}, repairKinds(t, out))
		list := out[0].OfMessage.Content.OfInputItemContentList
		require.Len(t, list, 2)
		assert.Equal(t, "[tool output: screenshot]\ncaptured", list[0].OfInputText.Text)
		require.NotNil(t, list[1].OfInputImage)
		assert.Equal(t, "data:image/png;base64,AAAA", list[1].OfInputImage.ImageURL.Value)
	})

	t.Run("duplicate output for an answered call becomes user text", func(t *testing.T) {
		items := parseCodexInput(t, `{"input":[
		 {"type":"function_call","call_id":"call_a","name":"shell","arguments":"{}"},
		 {"type":"function_call_output","call_id":"call_a","output":"first"},
		 {"type":"function_call_output","call_id":"call_a","output":"second"}]}`)
		out := RepairResponsesToolCalls(items)
		require.Equal(t, []string{"fc:call_a", "fco:call_a", "user"}, repairKinds(t, out))
		assert.Equal(t, "first", repairOutputText(t, out[1]))
		assert.Equal(t, "[tool output: call_a]\nsecond", repairUserText(t, out[2]))
	})

	t.Run("output listed before its call is placed after it once", func(t *testing.T) {
		items := parseCodexInput(t, `{"input":[
		 {"type":"function_call_output","call_id":"call_a","output":"early"},
		 {"type":"function_call","call_id":"call_a","name":"shell","arguments":"{}"},
		 {"type":"message","role":"user","content":"next"}]}`)
		out := RepairResponsesToolCalls(items)
		require.Equal(t, []string{"fc:call_a", "fco:call_a", "user"}, repairKinds(t, out))
		assert.Equal(t, "early", repairOutputText(t, out[1]))
	})
}
