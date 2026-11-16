package wire

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func marshalItem(t *testing.T, item ResponsesOutputItemWire) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(item)
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &raw))
	return raw
}

// Codex ignores a message item without "content" and then drops every
// output_text delta for it (#1922).
func TestResponsesOutputItemWire_MessageAlwaysCarriesContent(t *testing.T) {
	for name, content := range map[string][]ResponsesContentPartWire{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			raw := marshalItem(t, ResponsesOutputItemWire{ID: "msg_1", Type: "message", Role: "assistant", Status: "in_progress", Content: content})
			assert.JSONEq(t, `[]`, string(raw["content"]))
			assert.JSONEq(t, `"msg_1"`, string(raw["id"]))
			assert.JSONEq(t, `"assistant"`, string(raw["role"]))
			assert.JSONEq(t, `"in_progress"`, string(raw["status"]))
			assert.NotContains(t, raw, "call_id", "omitempty still applies to the other fields")
		})
	}

	raw := marshalItem(t, ResponsesOutputItemWire{ID: "msg_1", Type: "message", Content: []ResponsesContentPartWire{{Type: "output_text", Text: "hello"}}})
	assert.JSONEq(t, `[{"type":"output_text","text":"hello","annotations":[]}]`, string(raw["content"]))
}

func TestResponsesOutputItemWire_NonMessageItemsUnchanged(t *testing.T) {
	args := `{"city":"Paris"}`
	raw := marshalItem(t, ResponsesOutputItemWire{ID: "fc_1", Type: "function_call", Status: "in_progress", CallID: "call_1", Name: "get_weather", Arguments: &args})
	assert.NotContains(t, raw, "content")
	assert.NotContains(t, raw, "role")
	assert.JSONEq(t, `"call_1"`, string(raw["call_id"]))
	assert.JSONEq(t, `"{\"city\":\"Paris\"}"`, string(raw["arguments"]))
}

func TestResponsesOutputItemAddedEvent_MessageCarriesContent(t *testing.T) {
	b, err := json.Marshal(ResponsesOutputItemAddedEvent{
		Type: "response.output_item.added",
		Item: ResponsesOutputItemWire{ID: "msg_1", Type: "message", Role: "assistant", Status: "in_progress", Content: []ResponsesContentPartWire{}},
	})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"content":[]`)
}
