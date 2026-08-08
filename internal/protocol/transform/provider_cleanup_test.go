package transform

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
)

func TestOpenAIChatProviderCleanupRemovesGatewayFields(t *testing.T) {
	var request openai.ChatCompletionNewParams
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"provider-model",
		"messages":[
			{"role":"assistant","content":"prior","x_thinking":"private"}
		],
		"tools":[]
	}`), &request))
	ctx := NewTransformContext(&request)

	require.NoError(t, NewOpenAIChatProviderCleanupTransform().Apply(ctx))
	cleaned, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	require.True(t, ok)
	body, err := json.Marshal(cleaned)
	require.NoError(t, err)
	require.NotContains(t, string(body), "x_thinking")
	require.NotContains(t, string(body), `"tools"`)
}
