package server

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/require"
)

func TestHasDeclaredMCPTools_OpenAI(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name: "normal_tool",
			}),
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name: "mcp__websearch__search",
			}),
		},
	}

	require.True(t, hasDeclaredMCPTools(req))
}

func TestHasDeclaredMCPTools_AnthropicV1AndBeta(t *testing.T) {
	v1Req := &anthropic.MessageNewParams{
		Tools: []anthropic.ToolUnionParam{
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "normal_tool"),
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "mcp__webfetch__fetch"),
		},
	}
	require.True(t, hasDeclaredMCPAnthropicV1Tools(v1Req))

	betaReq := &anthropic.BetaMessageNewParams{
		Tools: []anthropic.BetaToolUnionParam{
			anthropic.BetaToolUnionParamOfTool(anthropic.BetaToolInputSchemaParam{}, "normal_tool"),
			anthropic.BetaToolUnionParamOfTool(anthropic.BetaToolInputSchemaParam{}, "mcp__websearch__search"),
		},
	}
	require.True(t, hasDeclaredMCPAnthropicBetaTools(betaReq))
}

func TestHasOnlyMCPToolCalls(t *testing.T) {
	var allMCP []openai.ChatCompletionMessageToolCallUnion
	require.NoError(t, json.Unmarshal([]byte(`[
	  {"id":"call_1","type":"function","function":{"name":"mcp__a__x","arguments":"{}"}},
	  {"id":"call_2","type":"function","function":{"name":"mcp__b__y","arguments":"{}"}}
	]`), &allMCP))
	require.True(t, hasOnlyMCPToolCalls(allMCP))

	var mixed []openai.ChatCompletionMessageToolCallUnion
	require.NoError(t, json.Unmarshal([]byte(`[
	  {"id":"call_1","type":"function","function":{"name":"mcp__a__x","arguments":"{}"}},
	  {"id":"call_2","type":"function","function":{"name":"normal_tool","arguments":"{}"}}
	]`), &mixed))
	require.False(t, hasOnlyMCPToolCalls(mixed))
}

func TestHasOnlyMCPToolUsesV1AndBeta(t *testing.T) {
	var v1Content []anthropic.ContentBlockUnion
	require.NoError(t, json.Unmarshal([]byte(`[
	  {"type":"tool_use","id":"toolu_1","name":"mcp__a__x","input":{}}
	]`), &v1Content))
	toolUsesV1, okV1 := hasOnlyMCPToolUsesV1(v1Content)
	require.True(t, okV1)
	require.Len(t, toolUsesV1, 1)

	var betaContent []anthropic.BetaContentBlockUnion
	require.NoError(t, json.Unmarshal([]byte(`[
	  {"type":"tool_use","id":"toolu_1","name":"mcp__a__x","input":{}}
	]`), &betaContent))
	toolUsesBeta, okBeta := hasOnlyMCPToolUsesBeta(betaContent)
	require.True(t, okBeta)
	require.Len(t, toolUsesBeta, 1)
}
