package toolnamespace

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const codexTools = `[
  {"type":"function","name":"shell","parameters":{"type":"object","properties":{}}},
  {"type":"namespace","name":"mcp__linear__","description":"Linear MCP","tools":[
    {"type":"function","name":"list_issues","description":"List issues","parameters":{"type":"object","properties":{"team":{"type":"string"}}}},
    {"type":"function","name":"get_issue","parameters":{"type":"object","properties":{}}}
  ]},
  {"type":"namespace","name":"crm","description":"CRM","tools":[
    {"type":"function","name":"get_customer","parameters":{"type":"object","properties":{}}},
    {"type":"custom","name":"free_form"}
  ]},
  {"type":"web_search"}
]`

func parseTools(t *testing.T, raw string) []responses.ToolUnionParam {
	t.Helper()
	var tools []responses.ToolUnionParam
	require.NoError(t, json.Unmarshal([]byte(raw), &tools))
	return tools
}

func TestFlatName(t *testing.T) {
	assert.Equal(t, "shell", FlatName("", "shell"))
	assert.Equal(t, "mcp__linear__get_issue", FlatName("mcp__linear__", "get_issue"))
	assert.Equal(t, "mcp_x_get", FlatName("mcp_x_", "get"))
	assert.Equal(t, "crm__get_customer", FlatName("crm", "get_customer"))
}

func TestFunctionsExpandsNamespaces(t *testing.T) {
	tools := parseTools(t, codexTools)
	functions, index := Functions(tools), IndexOf(tools)

	names := make([]string, len(functions))
	for i, fn := range functions {
		names[i] = fn.Name
	}
	// Request order is kept; the custom tool and web_search have no flat form.
	assert.Equal(t, []string{"shell", "mcp__linear__list_issues", "mcp__linear__get_issue", "crm__get_customer"}, names)

	assert.Equal(t, "List issues", functions[1].Description.Value)
	assert.Equal(t, "string", functions[1].Parameters["properties"].(map[string]any)["team"].(map[string]any)["type"])

	assert.Equal(t, Index{
		"mcp__linear__list_issues": {Namespace: "mcp__linear__", Name: "list_issues"},
		"mcp__linear__get_issue":   {Namespace: "mcp__linear__", Name: "get_issue"},
		"crm__get_customer":        {Namespace: "crm", Name: "get_customer"},
	}, index)
}

func TestIndexRestore(t *testing.T) {
	index := IndexOf(parseTools(t, codexTools))

	namespace, name := index.Restore("mcp__linear__get_issue")
	assert.Equal(t, "mcp__linear__", namespace)
	assert.Equal(t, "get_issue", name)

	// A top-level function, and any name at all on a nil index, stay as they are.
	namespace, name = index.Restore("shell")
	assert.Empty(t, namespace)
	assert.Equal(t, "shell", name)
	namespace, name = Index(nil).Restore("mcp__linear__get_issue")
	assert.Empty(t, namespace)
	assert.Equal(t, "mcp__linear__get_issue", name)
}
