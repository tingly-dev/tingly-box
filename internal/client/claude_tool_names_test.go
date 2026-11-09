package client

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/msgparam"
)

func TestTitleCaseToolName(t *testing.T) {
	// Purely mechanical: the MCP-namespace exemption lives in
	// claudeCodeToolName, not here.
	cases := map[string]string{
		"read_file":     "ReadFile",
		"ha_get_state":  "HaGetState",
		"terminal":      "Terminal",
		"browser_exec":  "BrowserExec",
		"Bash":          "Bash", // already TitleCase
		"WebFetch":      "WebFetch",
		"":              "",
		"mcp_foo_bar":   "McpFooBar",
		"mcp_":          "Mcp",
		"tool_2_thing":  "Tool2Thing",
		"__weird__name": "WeirdName",
	}
	for in, want := range cases {
		assert.Equal(t, want, titleCaseToolName(in), "titleCaseToolName(%q)", in)
	}
}

func TestClaudeCodeToolName(t *testing.T) {
	t.Run("map wins over mechanical fold", func(t *testing.T) {
		// The official spelling must win: mechanical folding would give "Ls".
		assert.Equal(t, "LS", claudeCodeToolName("ls"))
		assert.Equal(t, "TodoWrite", claudeCodeToolName("todowrite"))
		assert.Equal(t, "NotebookEdit", claudeCodeToolName("notebookedit"))
	})

	t.Run("MCP-namespaced names pass through verbatim", func(t *testing.T) {
		// Claude Code sends MCP tools as mcp__<server>__<tool> — lowercase,
		// unfolded — and Tingly-Box's own server tools follow the same
		// convention (internal/tool.NormalizeToolName).
		for _, name := range []string{
			"mcp__github__get_pull_request",
			"mcp__linear__create_issue",
			"mcp__read_file",
			"tingly_box_mcp__webtools__mcp_web_search",
			"tingly_box_mcp__brave-search__brave_web_search",
		} {
			assert.Equal(t, name, claudeCodeToolName(name), "claudeCodeToolName(%q)", name)
		}
	})

	t.Run("single-underscore mcp_ is not a namespace and folds", func(t *testing.T) {
		assert.Equal(t, "McpFooBar", claudeCodeToolName("mcp_foo_bar"))
		assert.Equal(t, "McpLinearGetIssue", claudeCodeToolName("mcp_linear_get_issue"))
		assert.Equal(t, "Mcp", claudeCodeToolName("mcp_"))
	})

	t.Run("unknown names fold mechanically", func(t *testing.T) {
		assert.Equal(t, "SearchFiles", claudeCodeToolName("search_files"))
	})
}

func TestPlanToolRenames(t *testing.T) {
	t.Run("independent names all rename", func(t *testing.T) {
		plan := planToolRenames([]string{"read_file", "write_file"})
		assert.Equal(t, map[string]string{
			"read_file":  "ReadFile",
			"write_file": "WriteFile",
		}, plan)
	})

	t.Run("target already taken by another tool", func(t *testing.T) {
		assert.Empty(t, planToolRenames([]string{"my_tool", "MyTool"}))
	})

	t.Run("two sources folding to the same target", func(t *testing.T) {
		// "foo_bar" and "foo_Bar" both fold to "FooBar"; only the first may rename.
		plan := planToolRenames([]string{"foo_bar", "foo_Bar"})
		assert.Len(t, plan, 1)
		assert.Equal(t, "FooBar", plan["foo_bar"])
	})

	t.Run("MCP-namespaced names are excluded from the plan", func(t *testing.T) {
		plan := planToolRenames([]string{
			"mcp__github__get_pull_request",
			"tingly_box_mcp__webtools__mcp_web_search",
			"read_file",
		})
		assert.Equal(t, map[string]string{"read_file": "ReadFile"}, plan)
	})
}

func v1Tool(name string) anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{Name: name}}
}

// TestRemapRequestToolNames covers the whole plan-application step against the
// v1 request type: tools[], tool_choice, and prior-turn tool_use blocks must
// all agree. The Beta twin is exercised end-to-end on the wire
// (TestWire_BetaStreaming_RenamesEverySiteAndRestoresTheStream), so only a
// smoke test of it lives here.
func TestRemapRequestToolNames(t *testing.T) {
	t.Run("every site follows one plan", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Tools: []anthropic.ToolUnionParam{
				v1Tool("read_file"),
				v1Tool("mcp__github__get_pull_request"),
				v1Tool("ls"),
			},
			ToolChoice: anthropic.ToolChoiceParamOfTool("read_file"),
			Messages: []anthropic.MessageParam{
				msgparam.New("assistant", []anthropic.ContentBlockParamUnion{
					{OfToolUse: &anthropic.ToolUseBlockParam{ID: "t1", Name: "read_file"}},
					{OfToolUse: &anthropic.ToolUseBlockParam{ID: "t2", Name: "retired_tool"}},
				}),
			},
		}
		rev := remapRequestToolNames(req)

		assert.Equal(t, "ReadFile", req.Tools[0].OfTool.Name)
		assert.Equal(t, "mcp__github__get_pull_request", req.Tools[1].OfTool.Name)
		assert.Equal(t, "LS", req.Tools[2].OfTool.Name)
		assert.Equal(t, "ReadFile", req.ToolChoice.OfTool.Name,
			"tool_choice must name a tool that exists in tools[]")
		assert.Equal(t, "ReadFile", msgparam.Content(req.Messages[0])[0].OfToolUse.Name)
		// History names absent from the plan are untouched.
		assert.Equal(t, "retired_tool", msgparam.Content(req.Messages[0])[1].OfToolUse.Name)
		assert.Equal(t, map[string]string{"ReadFile": "read_file", "LS": "ls"}, rev)
	})

	t.Run("pin follows the plan, not an independent fold", func(t *testing.T) {
		// planToolRenames skips "my_tool" because "MyTool" is already taken.
		// Folding tool_choice independently would pin "MyTool" — a name that
		// is in tools[], but the *wrong* tool.
		req := &anthropic.MessageNewParams{
			Tools:      []anthropic.ToolUnionParam{v1Tool("my_tool"), v1Tool("MyTool")},
			ToolChoice: anthropic.ToolChoiceParamOfTool("my_tool"),
		}
		rev := remapRequestToolNames(req)
		assert.Empty(t, rev)
		assert.Equal(t, "my_tool", req.ToolChoice.OfTool.Name)
	})

	t.Run("skips built-in tools and non-tool tool_choice", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Tools:      []anthropic.ToolUnionParam{{OfBashTool20250124: &anthropic.ToolBash20250124Param{}}},
			ToolChoice: anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}},
		}
		require.NotPanics(t, func() { remapRequestToolNames(req) })
		assert.Nil(t, remapRequestToolNames(nil))
	})
}

func TestRemapBetaRequestToolNames_Smoke(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Tools:      []anthropic.BetaToolUnionParam{{OfTool: &anthropic.BetaToolParam{Name: "bash"}}},
		ToolChoice: anthropic.BetaToolChoiceParamOfTool("bash"),
	}
	rev := remapBetaRequestToolNames(req)
	assert.Equal(t, "Bash", req.Tools[0].OfTool.Name)
	assert.Equal(t, "Bash", req.ToolChoice.OfTool.Name)
	assert.Equal(t, map[string]string{"Bash": "bash"}, rev)
}

// sseLine builds one SSE event frame.
func sseLine(event string, payload any) string {
	b, _ := json.Marshal(payload)
	return "event: " + event + "\ndata: " + string(b) + "\n\n"
}

func TestSSEToolNameRewriter(t *testing.T) {
	reverse := map[string]string{"ReadFile": "read_file"}

	t.Run("rewrites tool_use name in content_block_start only", func(t *testing.T) {
		body := sseLine("message_start", map[string]any{"type": "message_start"}) +
			sseLine("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": 0,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    "toolu_1",
					"name":  "ReadFile",
					"input": map[string]any{},
				},
			}) +
			sseLine("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `{"path":`},
			}) +
			sseLine("message_stop", map[string]any{"type": "message_stop"})

		r := newSSEToolNameRewriter(io.NopCloser(strings.NewReader(body)), reverse)
		var sb strings.Builder
		buf := make([]byte, 3) // force many partial reads
		for {
			n, err := r.Read(buf)
			sb.Write(buf[:n])
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		out := sb.String()
		assert.Contains(t, out, `"name":"read_file"`)
		assert.NotContains(t, out, "ReadFile")
		// Non-tool events pass through, framing intact.
		assert.Contains(t, out, `"type":"message_start"`)
		assert.Contains(t, out, `"type":"message_stop"`)
	})

	t.Run("passthrough cases", func(t *testing.T) {
		cases := []string{
			// Name absent from the reverse map.
			sseLine("content_block_start", map[string]any{
				"type":          "content_block_start",
				"content_block": map[string]any{"type": "tool_use", "id": "t", "name": "SomethingElse"},
			}),
			// Text block, not tool_use.
			sseLine("content_block_start", map[string]any{
				"type":          "content_block_start",
				"content_block": map[string]any{"type": "text", "text": ""},
			}),
			// Malformed JSON.
			"data: {\"type\":\"content_block_start\",\"tool_use\" BROKEN\n\n",
			// Empty reverse map is inert.
			sseLine("content_block_start", map[string]any{
				"type":          "content_block_start",
				"content_block": map[string]any{"type": "tool_use", "id": "t", "name": "ReadFile"},
			}),
		}
		for i, body := range cases[:3] {
			assert.Equal(t, body, readAll(t, body, reverse), "case %d", i)
		}
		assert.Equal(t, cases[3], readAll(t, cases[3], map[string]string{}))
	})
}

func readAll(t *testing.T, body string, reverse map[string]string) string {
	t.Helper()
	r := newSSEToolNameRewriter(io.NopCloser(strings.NewReader(body)), reverse)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	return string(out)
}
