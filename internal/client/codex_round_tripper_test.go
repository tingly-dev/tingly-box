package client

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestSanitizeCodexInputIDsJSON_DropsRequiredEmptyID(t *testing.T) {
	body := `{
		"model": "gpt-5-codex",
		"input": [
			{"type": "message", "role": "user", "content": "hello"},
			{"type": "reasoning", "id": "", "summary": []},
			{"type": "reasoning", "id": "rs_abc123", "summary": []}
		]
	}`

	out := sanitizeCodexInputIDsJSON(body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 2, "reasoning item with empty id must be dropped")
	assert.Equal(t, "message", input[0].Get("type").String())
	assert.Equal(t, "reasoning", input[1].Get("type").String())
	assert.Equal(t, "rs_abc123", input[1].Get("id").String())
}

func TestSanitizeCodexInputIDsJSON_ClearsOptionalEmptyID(t *testing.T) {
	body := `{
		"input": [
			{"type": "function_call", "id": "", "call_id": "c1", "name": "f", "arguments": "{}"},
			{"type": "shell_call", "id": "", "call_id": "c2", "action": {}},
			{"type": "apply_patch_call", "id": "", "call_id": "c3", "operation": {}}
		]
	}`

	out := sanitizeCodexInputIDsJSON(body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 3, "optional-id items must not be dropped")
	for _, item := range input {
		assert.False(t, item.Get("id").Exists(), "empty id should be deleted from item type %s", item.Get("type").String())
	}
}

func TestSanitizeCodexInputIDsJSON_DropsInvalidChars(t *testing.T) {
	body := `{
		"input": [
			{"type": "reasoning", "id": "rs/bad chars!", "summary": []},
			{"type": "function_call", "id": "fc/bad", "call_id": "c1", "name": "f", "arguments": "{}"}
		]
	}`

	out := sanitizeCodexInputIDsJSON(body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 1, "reasoning with invalid chars must be dropped")
	assert.Equal(t, "function_call", input[0].Get("type").String())
	assert.False(t, input[0].Get("id").Exists(), "function_call id with invalid chars must be cleared")
}

func TestSanitizeCodexInputIDsJSON_NoOpWhenAllValid(t *testing.T) {
	body := `{"input":[{"type":"reasoning","id":"rs_abc","summary":[]},{"type":"function_call","id":"fc_123","call_id":"c","name":"f","arguments":"{}"}]}`

	out := sanitizeCodexInputIDsJSON(body)

	assert.Equal(t, body, out, "valid ids must not be modified")
}

func TestSanitizeCodexInputIDsJSON_NoInputArray(t *testing.T) {
	body := `{"model":"gpt-5-codex","input":"hello"}`

	out := sanitizeCodexInputIDsJSON(body)

	assert.Equal(t, body, out, "non-array input must be passed through")
}

func TestSanitizeCodexInputIDsJSON_WhitespaceOnlyID(t *testing.T) {
	body := `{"input":[{"type":"function_call_output","id":"   ","call_id":"c1","output":"ok"}]}`

	out := sanitizeCodexInputIDsJSON(body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 1)
	assert.False(t, input[0].Get("id").Exists(), "whitespace-only id should be cleared")
}

func TestSanitizeCodexInputIDsJSON_HighIndex(t *testing.T) {
	// Mirrors the production failure: input[186].id == ""
	var items []string
	for i := 0; i < 186; i++ {
		items = append(items, `{"type":"message","role":"user","content":"x"}`)
	}
	items = append(items, `{"type":"shell_call","id":"","call_id":"c","action":{}}`)
	body := `{"input":[` + strings.Join(items, ",") + `]}`

	out := sanitizeCodexInputIDsJSON(body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 187)
	assert.False(t, input[186].Get("id").Exists())
}

func TestSanitizeCodexEmptyContentJSON_DropsEmptyStringContent(t *testing.T) {
	body := `{
		"input": [
			{"type": "message", "role": "user", "content": "hello"},
			{"type": "message", "role": "assistant", "content": ""},
			{"type": "message", "role": "user", "content": ""},
			{"type": "function_call_output", "call_id": "c1", "output": ""}
		]
	}`

	out := sanitizeCodexEmptyContentJSON(body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 2, "message items with empty string content must be dropped")
	assert.Equal(t, "hello", input[0].Get("content").String())
	assert.Equal(t, "function_call_output", input[1].Get("type").String(), "non-message items with empty content must be kept")
}

func TestSanitizeCodexEmptyContentJSON_KeepsNonEmptyContent(t *testing.T) {
	body := `{"input":[{"type":"message","role":"user","content":"hi"},{"type":"message","role":"assistant","content":"hello"}]}`

	out := sanitizeCodexEmptyContentJSON(body)

	assert.Equal(t, body, out, "non-empty content must not be modified")
}

func TestSanitizeCodexEmptyContentJSON_NoInputArray(t *testing.T) {
	body := `{"model":"gpt-5-codex","input":"hello"}`

	out := sanitizeCodexEmptyContentJSON(body)

	assert.Equal(t, body, out)
}

func TestSanitizeCodexEmptyContentJSON_HighIndex(t *testing.T) {
	// Mirrors the production failure: input[1012].content == ""
	var items []string
	for i := 0; i < 1012; i++ {
		items = append(items, `{"type":"message","role":"user","content":"x"}`)
	}
	items = append(items, `{"type":"message","role":"assistant","content":""}`)
	body := `{"input":[` + strings.Join(items, ",") + `]}`

	out := sanitizeCodexEmptyContentJSON(body)

	input := gjson.Get(out, "input").Array()
	assert.Len(t, input, 1012, "empty-content message at high index must be dropped")
}

func TestCodexInputItemIDRequired(t *testing.T) {
	required := []string{
		"reasoning", "code_interpreter_call", "computer_call", "file_search_call",
		"web_search_call", "image_generation_call", "local_shell_call",
		"local_shell_call_output", "mcp_list_tools", "mcp_approval_request",
		"mcp_call", "item_reference",
	}
	optional := []string{
		"function_call", "function_call_output", "shell_call", "shell_call_output",
		"apply_patch_call", "apply_patch_call_output", "computer_call_output",
		"custom_tool_call", "custom_tool_call_output", "mcp_approval_response",
		"compaction", "message", "output_message",
	}
	for _, typ := range required {
		assert.True(t, codexInputItemIDRequired(typ), "type %q should require id", typ)
	}
	for _, typ := range optional {
		assert.False(t, codexInputItemIDRequired(typ), "type %q should not require id", typ)
	}
}
