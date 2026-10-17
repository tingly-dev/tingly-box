package client

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fitChat(t *testing.T, body string) (map[string]any, map[string]string) {
	t.Helper()
	out, names := workbuddyFitChat([]byte(body))
	var v map[string]any
	require.NoError(t, json.Unmarshal(out, &v))
	return v, names
}

func TestWorkBuddyFitChatSpellings(t *testing.T) {
	v, _ := fitChat(t, `{"model":"m","max_completion_tokens":100,"messages":[{"role":"developer","content":"be brief"},{"role":"user","content":"hi"}],
		"tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object","properties":{}}}}],
		"tool_choice":{"type":"function","function":{"name":"read"}}}`)
	assert.Equal(t, float64(100), v["max_tokens"])
	assert.NotContains(t, v, "max_completion_tokens")
	assert.Equal(t, "system", v["messages"].([]any)[0].(map[string]any)["role"])
	assert.Equal(t, "read", v["tool_choice"])

	v, _ = fitChat(t, `{"messages":[],"tools":[{"type":"function","function":{"name":"x"}}],"tool_choice":"none"}`)
	assert.NotContains(t, v, "tools", "tool_choice none drops the tools")

	// A body that needs nothing is passed through byte for byte.
	plain := `{"model":"m",  "messages":[{"role":"user","content":"hi"}]}`
	out, names := workbuddyFitChat([]byte(plain))
	assert.Equal(t, plain, string(out))
	assert.Nil(t, names)
}

func TestWorkBuddyFitToolNames(t *testing.T) {
	v, names := fitChat(t, `{"messages":[
		{"role":"assistant","tool_calls":[{"id":"1","type":"function","function":{"name":"fs.read","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"1","name":"fs.read","content":"ok"}],
		"tools":[{"type":"function","function":{"name":"fs.read","parameters":{"type":"object"}}},
		         {"type":"function","function":{"name":"fs_read","parameters":{"type":"object"}}}],
		"tool_choice":{"type":"function","function":{"name":"fs.read"}}}`)
	tools := v["tools"].([]any)
	// The safe name steers clear of a tool already called fs_read.
	assert.Equal(t, "fs_read_2", tools[0].(map[string]any)["function"].(map[string]any)["name"])
	assert.Equal(t, map[string]string{"fs_read_2": "fs.read"}, names)
	msgs := v["messages"].([]any)
	assert.Equal(t, "fs_read_2", msgs[0].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["name"])
	assert.Equal(t, "fs_read_2", msgs[1].(map[string]any)["name"])
	assert.Equal(t, "fs_read_2", v["tool_choice"])
}

func TestWorkBuddyFitToolSchemas(t *testing.T) {
	v, _ := fitChat(t, `{"messages":[],"tools":[{"type":"function","function":{"name":"t","parameters":{
		"anyOf":[
			{"type":"object","properties":{"a":{"type":"String"},"mode":{"enum":["x","y"]}},"required":["a","mode"]},
			{"type":"object","properties":{"b":{"type":"integer"},"mode":{"enum":["x","y"]}},"required":["b","mode"]}],
		"$defs":{"z":true}}}},
		{"type":"function","function":{"name":"u","parameters":{"type":"object","properties":{
			"p":{"type":"string","pattern":"^[^\\0]+$"},
			"tuple":{"type":"array","items":[{"type":"string"},{"type":"integer"}]},
			"mixed":{"enum":[1,"a",null]},
			"empty":{"type":"string","enum":[]},
			"flag":false},
			"required":"p"}}}]}`)
	params := v["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	assert.Equal(t, "object", params["type"])
	assert.NotContains(t, params, "anyOf")
	props := params["properties"].(map[string]any)
	assert.Equal(t, "string", props["a"].(map[string]any)["type"], "type names are lowercased")
	assert.Equal(t, "string", props["mode"].(map[string]any)["type"], "an untyped enum gets its values' type")
	assert.Contains(t, props, "b")
	assert.Equal(t, []any{"mode"}, params["required"], "only what every branch requires stays required")
	assert.Equal(t, map[string]any{}, params["$defs"].(map[string]any)["z"], "true is {}")

	u := v["tools"].([]any)[1].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	up := u["properties"].(map[string]any)
	assert.Equal(t, `^[^\u0000]+$`, up["p"].(map[string]any)["pattern"])
	assert.Equal(t, map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}}}, up["tuple"].(map[string]any)["items"])
	assert.Equal(t, []any{
		map[string]any{"type": "integer", "enum": []any{float64(1)}},
		map[string]any{"type": "string", "enum": []any{"a"}},
		map[string]any{"type": "null"},
	}, up["mixed"].(map[string]any)["anyOf"])
	assert.NotContains(t, up["empty"].(map[string]any), "enum")
	assert.Equal(t, map[string]any{"not": map[string]any{}}, up["flag"])
	assert.NotContains(t, u, "required", "a required that is not a list is dropped")
}

func TestWorkBuddyNameBackStream(t *testing.T) {
	src := "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"fs_read_2\",\"arguments\":\"\"}}]}}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"keep\"}}]}\n\n" +
		"data: [DONE]\n\n"
	body := newWorkBuddyNameBackBody(io.NopCloser(strings.NewReader(src)), map[string]string{"fs_read_2": "fs.read"})
	out, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"name":"fs.read"`)
	assert.Contains(t, string(out), "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"keep\"}}]}\n\n", "untouched lines pass through as sent")
	assert.True(t, strings.HasSuffix(string(out), "data: [DONE]\n\n"))
}
