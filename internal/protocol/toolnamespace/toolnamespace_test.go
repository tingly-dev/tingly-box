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

func flatNames(functions []responses.FunctionToolParam) []string {
	names := make([]string, len(functions))
	for i, fn := range functions {
		names[i] = fn.Name
	}
	return names
}

func TestSameMemberNameInDifferentNamespaces(t *testing.T) {
	tools := parseTools(t, `[
	  {"type":"namespace","name":"mcp__github__","description":"d","tools":[{"type":"function","name":"search","parameters":{"type":"object"}}]},
	  {"type":"namespace","name":"mcp__linear__","description":"d","tools":[{"type":"function","name":"search","parameters":{"type":"object"}}]}
	]`)

	assert.Equal(t, []string{"mcp__github__search", "mcp__linear__search"}, flatNames(Functions(tools)))
	index := IndexOf(tools)
	assert.Equal(t, Entry{Namespace: "mcp__github__", Name: "search"}, index["mcp__github__search"])
	assert.Equal(t, Entry{Namespace: "mcp__linear__", Name: "search"}, index["mcp__linear__search"])
}

// FlatName is not injective ("a_"+"_b" and "a"+"b" both give "a__b"). A clash
// must never let one tool be restored as another: the first namespace member
// keeps the name, the later one is dropped, and a plain function always wins.
func TestFlatNameCollisions(t *testing.T) {
	require.Equal(t, FlatName("a_", "_b"), FlatName("a", "b"))

	t.Run("two namespace members", func(t *testing.T) {
		tools := parseTools(t, `[
		  {"type":"namespace","name":"a_","description":"d","tools":[{"type":"function","name":"_b","parameters":{"type":"object"}}]},
		  {"type":"namespace","name":"a","description":"d","tools":[{"type":"function","name":"b","parameters":{"type":"object"}}]}
		]`)
		assert.Equal(t, []string{"a__b"}, flatNames(Functions(tools)))
		assert.Equal(t, Index{"a__b": {Namespace: "a_", Name: "_b"}}, IndexOf(tools))
	})

	t.Run("plain function declared after the namespace", func(t *testing.T) {
		tools := parseTools(t, `[
		  {"type":"namespace","name":"mcp__a__","description":"d","tools":[{"type":"function","name":"b","parameters":{"type":"object"}}]},
		  {"type":"function","name":"mcp__a__b","parameters":{"type":"object"}}
		]`)
		assert.Equal(t, []string{"mcp__a__b"}, flatNames(Functions(tools)))
		assert.Empty(t, IndexOf(tools), "the plain function keeps the name, so it must not be restored as a namespace member")
	})
}

func TestUnknownNamesAreNotRestored(t *testing.T) {
	index := IndexOf(parseTools(t, codexTools))
	for _, name := range []string{"", "mcp__linear__", "mcp__linear__delete_everything", "get_issue"} {
		namespace, restored := index.Restore(name)
		assert.Empty(t, namespace, name)
		assert.Equal(t, name, restored)
	}
}

func TestNoTools(t *testing.T) {
	assert.Empty(t, Functions(nil))
	assert.Nil(t, IndexOf(nil))
	assert.Empty(t, Functions(parseTools(t, `[{"type":"namespace","name":"x","description":"d","tools":[]}]`)))
}

// A namespace member must reach the upstream with the schema, description and
// strict flag it was declared with.
func TestNamespaceMemberKeepsItsDefinition(t *testing.T) {
	tools := parseTools(t, `[{"type":"namespace","name":"mcp__crm__","description":"d","tools":[
	  {"type":"function","name":"find","description":"Find a customer","strict":true,
	   "parameters":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}},
	  {"type":"function","name":"ping"}
	]}]`)
	functions := Functions(tools)
	require.Len(t, functions, 2)

	find := functions[0]
	assert.Equal(t, "Find a customer", find.Description.Value)
	assert.True(t, find.Strict.Value)
	assert.Equal(t, []any{"id"}, find.Parameters["required"])
	assert.Equal(t, false, find.Parameters["additionalProperties"])

	assert.Nil(t, functions[1].Parameters, "a member without parameters stays without; the converters fill in the empty object schema")
}
