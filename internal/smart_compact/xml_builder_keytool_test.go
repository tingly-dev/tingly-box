package smart_compact

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildConversationXML_KeyToolPreserved_V1 verifies that a key tool (Task)
// has its call inlined as <tool name="Task"> and its result immediately adjacent
// as <tool_result>, inside the assistant turn that issued it. FIDELITY + POSITION.
func TestBuildConversationXML_KeyToolPreserved_V1(t *testing.T) {
	pu := NewPathUtil()
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("用子 agent 调研 X")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("好"),
				anthropic.NewToolUseBlock("task-1", map[string]any{"description": "调研X", "prompt": "请调研"}, "Task"),
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock("task-1", "X 的调研结论是 …", false),
			},
		},
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("综合如下")),
	}

	out := buildConversationXML(msgs, pu)
	t.Logf("V1 out:\n%s", out)

	// Call inlined.
	assert.Contains(t, out, `<tool name="Task">`)
	// Result adjacent after the call.
	toolIdx := strings.Index(out, `<tool name="Task">`)
	resultIdx := strings.Index(out, "<tool_result>")
	require.GreaterOrEqual(t, toolIdx, 0)
	require.GreaterOrEqual(t, resultIdx, 0)
	assert.Greater(t, resultIdx, toolIdx, "tool_result must come after its <tool>")
	// Conclusion text present in <tool_result>, NOT leaked into <user>.
	assert.Contains(t, out, "X 的调研结论是 …")
	assert.NotContains(t, "<user>\n用子 agent 调研 X\n</user>", out) // sanity: user text stays its own
	// No residual global <tool_calls> since the only tool is a key tool.
	assert.NotContains(t, out, "<tool_calls>")
}

// TestBuildConversationXML_NonKeyToolSummarized_V1 verifies that a non-key tool
// (read_file) is NOT inlined as <tool>/<tool_result>; its file goes to a per-turn
// <tool_calls> and its result is not emitted as a standalone <tool_result>.
func TestBuildConversationXML_NonKeyToolSummarized_V1(t *testing.T) {
	pu := NewPathUtil()
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("读 a.go")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolUseBlock("r-1", map[string]any{"path": "a.go"}, "read_file"),
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock("r-1", "<a.go 内容>", false),
			},
		},
	}

	out := buildConversationXML(msgs, pu)
	t.Logf("V1 out:\n%s", out)

	// Non-key: file summarized into per-turn <tool_calls>.
	assert.Contains(t, out, "<tool_calls>")
	assert.Contains(t, out, "a.go")
	// Non-key tool is NOT inlined.
	assert.NotContains(t, out, `<tool name="read_file">`)
	assert.NotContains(t, out, "<tool_result>")
}

// TestBuildConversationXML_MultiRoundFileAttribution_V1 verifies the
// collectedFiles=nil bug is fixed: two rounds each reading a different file
// produce TWO <tool_calls> blocks (one per assistant turn), each with its own file.
func TestBuildConversationXML_MultiRoundFileAttribution_V1(t *testing.T) {
	pu := NewPathUtil()
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("读 a.go")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolUseBlock("r-1", map[string]any{"path": "a.go"}, "read_file"),
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock("r-1", "a-content", false),
			},
		},
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("ok")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("再读 b.go")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolUseBlock("r-2", map[string]any{"path": "b.go"}, "read_file"),
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock("r-2", "b-content", false),
			},
		},
	}

	out := buildConversationXML(msgs, pu)
	t.Logf("V1 out:\n%s", out)

	assert.Equal(t, 2, strings.Count(out, "<tool_calls>"), "each assistant turn gets its own <tool_calls>")
	assert.Contains(t, out, "a.go")
	assert.Contains(t, out, "b.go")
}

// TestBuildConversationXML_MixedTextAndTools_V1 verifies that within one assistant
// turn, text + key tool + non-key tool all appear in the right order and place.
func TestBuildConversationXML_MixedTextAndTools_V1(t *testing.T) {
	pu := NewPathUtil()
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("用子 agent 调研 X，再读 a.py")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("好"),
				anthropic.NewToolUseBlock("task-1", map[string]any{"description": "调研X"}, "Task"),
				anthropic.NewToolUseBlock("r-1", map[string]any{"path": "a.py"}, "read_file"),
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock("task-1", "X 结论", false),
				anthropic.NewToolResultBlock("r-1", "a.py 内容", false),
			},
		},
	}

	out := buildConversationXML(msgs, pu)
	t.Logf("V1 out:\n%s", out)

	// All three live inside the single <assistant> turn.
	assert.Contains(t, out, "好")
	assert.Contains(t, out, `<tool name="Task">`)
	assert.Contains(t, out, "<tool_result>X 结论</tool_result>")
	assert.Contains(t, out, "<tool_calls>")
	assert.Contains(t, out, "a.py")
	// Only the key tool gets a <tool_result>; non-key result is NOT a standalone tag.
	assert.Equal(t, 1, strings.Count(out, "<tool_result>"))
}

// TestBuildConversationXML_KeyToolErroredResult_V1 verifies is_error results are
// surfaced with an [error] prefix inside <tool_result>.
func TestBuildConversationXML_KeyToolErroredResult_V1(t *testing.T) {
	pu := NewPathUtil()
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("run it")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolUseBlock("task-1", map[string]any{"description": "do"}, "Task"),
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock("task-1", "boom", true),
			},
		},
	}

	out := buildConversationXML(msgs, pu)
	t.Logf("V1 out:\n%s", out)

	assert.Contains(t, out, "<tool_result>[error] boom</tool_result>")
}

// TestBuildConversationXML_DisabledCandidateTreatedAsNonKey verifies that a tool
// listed in keyToolsPreserve but set to false (e.g. WebSearch) is NOT inlined —
// it behaves like any non-key tool. This guards the enable/disable map semantics.
func TestBuildConversationXML_DisabledCandidateTreatedAsNonKey(t *testing.T) {
	pu := NewPathUtil()
	// WebSearch has no "path" key, so as a non-key tool it produces no <tool_calls>
	// either — we assert it is simply not inlined.
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("搜一下")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolUseBlock("ws-1", map[string]any{"query": "golang context"}, "WebSearch"),
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock("ws-1", "search result snippet", false),
			},
		},
	}

	out := buildConversationXML(msgs, pu)
	t.Logf("V1 out:\n%s", out)

	// Disabled → not inlined.
	assert.NotContains(t, out, `<tool name="WebSearch">`)
	assert.NotContains(t, out, "<tool_result>")
}

// TestBuildConversationXML_AskUserQuestionPreserved verifies that AskUserQuestion
// (user decision) is inlined as a key tool with its result adjacent.
func TestBuildConversationXML_AskUserQuestionPreserved(t *testing.T) {
	pu := NewPathUtil()
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("配置选哪个")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolUseBlock("q-1", map[string]any{"question": "用哪个库", "answer": "Recharts"}, "AskUserQuestion"),
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock("q-1", "用户选了 Recharts", false),
			},
		},
	}

	out := buildConversationXML(msgs, pu)
	t.Logf("V1 out:\n%s", out)

	assert.Contains(t, out, `<tool name="AskUserQuestion">`)
	toolIdx := strings.Index(out, `<tool name="AskUserQuestion">`)
	resultIdx := strings.Index(out, "<tool_result>")
	require.GreaterOrEqual(t, toolIdx, 0)
	require.GreaterOrEqual(t, resultIdx, 0)
	assert.Greater(t, resultIdx, toolIdx)
	assert.Contains(t, out, "用户选了 Recharts")
}

// TestBuildConversationXML_WebFetchPreserved verifies that WebFetch (external
// fetched info) is inlined as a key tool with its result adjacent.
func TestBuildConversationXML_WebFetchPreserved(t *testing.T) {
	pu := NewPathUtil()
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("看下这个文档")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolUseBlock("wf-1", map[string]any{"url": "https://example.com/docs"}, "WebFetch"),
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolResultBlock("wf-1", "文档关键内容 …", false),
			},
		},
	}

	out := buildConversationXML(msgs, pu)
	t.Logf("V1 out:\n%s", out)

	assert.Contains(t, out, `<tool name="WebFetch">`)
	toolIdx := strings.Index(out, `<tool name="WebFetch">`)
	resultIdx := strings.Index(out, "<tool_result>")
	require.GreaterOrEqual(t, toolIdx, 0)
	require.GreaterOrEqual(t, resultIdx, 0)
	assert.Greater(t, resultIdx, toolIdx)
	assert.Contains(t, out, "文档关键内容 …")
}

// TestBuildConversationXML_KeyToolPreserved_Beta is the beta-API mirror of the
// V1 key-tool preservation test. Output structure must match the V1 case.
func TestBuildConversationXML_KeyToolPreserved_Beta(t *testing.T) {
	pu := NewPathUtil()
	msgs := []anthropic.BetaMessageParam{
		{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("用子 agent 调研 X")}},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("好"),
				anthropic.NewBetaToolUseBlock("task-1", map[string]any{"description": "调研X"}, "Task"),
			},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolResultBlock("task-1", "X 的调研结论是 …", false)},
		},
		{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("综合如下")}},
	}

	out := buildBetaConversationXML(msgs, pu)
	t.Logf("Beta out:\n%s", out)

	assert.Contains(t, out, `<tool name="Task">`)
	toolIdx := strings.Index(out, `<tool name="Task">`)
	resultIdx := strings.Index(out, "<tool_result>")
	require.GreaterOrEqual(t, toolIdx, 0)
	require.GreaterOrEqual(t, resultIdx, 0)
	assert.Greater(t, resultIdx, toolIdx)
	assert.Contains(t, out, "X 的调研结论是 …")
	assert.NotContains(t, out, "<tool_calls>")
}
