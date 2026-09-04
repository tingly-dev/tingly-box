package protocolserver

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// decodeInput mirrors convertToResponsesParams' body → params step so the guard
// is exercised against the same deserialization it protects.
func decodeInput(t *testing.T, body string) ([]byte, *responses.ResponseNewParams) {
	t.Helper()
	processed, err := protocol.PreprocessInputData([]byte(body))
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(processed, &raw))
	modified, err := json.Marshal(raw)
	require.NoError(t, err)

	params := &responses.ResponseNewParams{}
	require.NoError(t, json.Unmarshal(modified, params))
	return processed, params
}

// An input item type the pinned SDK does not model takes the entire input array
// down with it — silently. The guard must catch that and name the item.
func TestCheckResponsesInputSurvived_UnknownItemType(t *testing.T) {
	processed, params := decodeInput(t, `{"model":"gpt-5.6-codex","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
		{"type":"totally_unknown_future","id":"zz_1","foo":"bar"}
	]}`)

	// Precondition: the SDK dropped everything without reporting an error.
	require.Empty(t, params.Input.OfInputItemList)

	err := checkResponsesInputSurvived(processed, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input[1]")
	assert.Contains(t, err.Error(), "totally_unknown_future")
}

func TestCheckResponsesInputSurvived_SupportedItemsPass(t *testing.T) {
	processed, params := decodeInput(t, `{"model":"gpt-5.6-codex","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
		{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"ENC"},
		{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_1","output":"ok"},
		{"type":"custom_tool_call","id":"ctc_1","call_id":"call_2","name":"apply_patch","input":"P"}
	]}`)

	require.Len(t, params.Input.OfInputItemList, 5)
	assert.NoError(t, checkResponsesInputSurvived(processed, params))
}

func TestCheckResponsesInputSurvived_StringAndEmptyInput(t *testing.T) {
	processed, params := decodeInput(t, `{"model":"gpt-5.6-codex","input":"just a string"}`)
	assert.NoError(t, checkResponsesInputSurvived(processed, params))

	processed, params = decodeInput(t, `{"model":"gpt-5.6-codex","input":[]}`)
	assert.NoError(t, checkResponsesInputSurvived(processed, params))
}
