package request

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// toolUseInputs marshals a converted request and returns the raw "input" of
// every tool_use block, keyed by tool_use id. A missing key means the block was
// sent without input, which Anthropic rejects ("tool_use.input: Field required").
func toolUseInputs(t *testing.T, req any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(req)
	require.NoError(t, err)

	var wire struct {
		Messages []struct {
			Content []map[string]json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(raw, &wire))

	inputs := map[string]json.RawMessage{}
	for _, msg := range wire.Messages {
		for _, block := range msg.Content {
			if string(block["type"]) != `"tool_use"` {
				continue
			}
			var id string
			require.NoError(t, json.Unmarshal(block["id"], &id))
			inputs[id] = block["input"]
		}
	}
	return inputs
}

func TestConvertOpenAIToAnthropicRequest_ToolCallWithoutArgumentsHasObjectInput(t *testing.T) {
	for name, arguments := range map[string]string{"blank": "", "null": "null", "malformed": `{"city":`} {
		t.Run(name, func(t *testing.T) {
			req := &openai.ChatCompletionNewParams{
				Model: "claude-opus-5",
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("what time is it?"),
					{OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{{
							OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
								ID:       "call_1",
								Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: "get_time", Arguments: arguments},
							},
						}},
					}},
					openai.ToolMessage("12:00", "call_1"),
				},
			}

			inputs := toolUseInputs(t, ConvertOpenAIToAnthropicRequest(req, 1024))
			require.Contains(t, inputs, "call_1")
			assert.JSONEq(t, `{}`, string(inputs["call_1"]))
		})
	}
}

func TestConvertOpenAIResponsesToAnthropicBetaRequest_FunctionCallWithoutArgumentsHasObjectInput(t *testing.T) {
	params := responses.ResponseNewParams{
		Model: "claude-opus-5",
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{OfFunctionCall: &responses.ResponseFunctionToolCallParam{CallID: "call_1", Name: "get_time", Arguments: ""}},
			},
		},
	}

	inputs := toolUseInputs(t, ConvertOpenAIResponsesToAnthropicBetaRequest(params, 1024))
	require.Contains(t, inputs, "call_1")
	assert.JSONEq(t, `{}`, string(inputs["call_1"]))
}

func TestConvertGoogleToAnthropicRequest_FunctionCallWithoutArgsHasObjectInput(t *testing.T) {
	contents := []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "what time is it?"}}},
		{Role: "model", Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{ID: "call_1", Name: "get_time"}}}},
	}

	inputs := toolUseInputs(t, ConvertGoogleToAnthropicRequest("claude-opus-5", contents, nil))
	require.Contains(t, inputs, "call_1")
	assert.JSONEq(t, `{}`, string(inputs["call_1"]))
}
