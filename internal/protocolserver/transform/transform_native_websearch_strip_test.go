package transform

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNativeWebToolsAreStrippedOnlyAfterServerReplacementInjection(t *testing.T) {
	cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{
		{ID: "webtools", Transport: "stdio", Command: "unused", Enabled: typ.BoolPtr(true), Tools: []string{"mcp_web_fetch"}},
		{ID: "server-webtools", Transport: "stdio", Command: "unused", Enabled: typ.BoolPtr(true), Tools: []string{"mcp_web_search", "mcp_web_fetch"}},
	}}
	t.Setenv("SERPER_API_KEY", "")
	rt := runtime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer rt.Close()
	native := func() *openai.ChatCompletionNewParams {
		return &openai.ChatCompletionNewParams{Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "web_search"}),
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "web_fetch"}),
		}}
	}
	strip := NewNativeWebSearchStripTransform(rt)
	ctx := protocoltransform.NewTransformContext(native())
	require.NoError(t, strip.Apply(ctx))
	require.Len(t, ctx.Request.(*openai.ChatCompletionNewParams).Tools, 2, "availability alone is not injection")
	inject := NewMCPToolInjectionTransform(rt).WithConnectedTools(func() bool { return false })
	require.NoError(t, inject.Apply(ctx))
	require.NoError(t, strip.Apply(ctx))
	require.Equal(t, []string{"web_search", "tingly_box_mcp__server-webtools__mcp_web_fetch"}, extractOpenAIToolNames(ctx.Request.(*openai.ChatCompletionNewParams).Tools))
	selected := native()
	selected.ToolChoice.OfFunctionToolChoice = &openai.ChatCompletionNamedToolChoiceParam{Function: openai.ChatCompletionNamedToolChoiceFunctionParam{Name: "web_fetch"}}
	selectedCtx := protocoltransform.NewTransformContext(selected)
	require.NoError(t, inject.Apply(selectedCtx))
	require.NoError(t, strip.Apply(selectedCtx))
	require.Contains(t, extractOpenAIToolNames(selected.Tools), "web_fetch", "explicit native choice must remain valid")
	advisorCtx := protocoltransform.NewTransformContext(native())
	advisorCtx.IsAdvisorRequest = true
	require.NoError(t, inject.Apply(advisorCtx))
	require.NoError(t, strip.Apply(advisorCtx))
	require.Len(t, advisorCtx.Request.(*openai.ChatCompletionNewParams).Tools, 2)
}

func TestIsNativeWebToolName_Search(t *testing.T) {
	cases := []struct {
		name   string
		expect bool
	}{
		{"web_search", true},
		{"WebSearch", true},
		{"websearch", true},
		{"WEB_SEARCH", true},
		{"web_fetch", false},
		{"mcp_web_search", false},
		{"mcp__tb__tingly_box_mcp__webtools__mcp_web_search", false},
	}
	for _, tc := range cases {
		got := isNativeWebToolName(tc.name, true, false)
		if got != tc.expect {
			t.Errorf("isNativeWebToolName(%q, stripSearch=true): got %v, want %v", tc.name, got, tc.expect)
		}
	}
}

func TestIsNativeWebToolName_Fetch(t *testing.T) {
	cases := []struct {
		name   string
		expect bool
	}{
		{"web_fetch", true},
		{"WebFetch", true},
		{"webfetch", true},
		{"web_search", false},
		{"WebSearch", false},
	}
	for _, tc := range cases {
		got := isNativeWebToolName(tc.name, false, true)
		if got != tc.expect {
			t.Errorf("isNativeWebToolName(%q, stripFetch=true): got %v, want %v", tc.name, got, tc.expect)
		}
	}
}
