package protocolserver

import (
	"testing"

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
				Name: "tingly_box_mcp__websearch__search",
			}),
		},
	}

	require.True(t, HasDeclaredMCPTools(req))
}
