package stream

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// FilterSpecialFields removes special fields that have dedicated content blocks
// e.g., reasoning_content is handled as thinking block, not merged into text_delta
func FilterSpecialFields(extras map[string]interface{}) map[string]interface{} {
	if extras == nil || len(extras) == 0 {
		return extras
	}
	result := make(map[string]interface{})
	for k, v := range extras {
		if k != OpenaiFieldReasoningContent {
			result[k] = v
		}
	}
	return result
}

func NewExampleTool() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        "get_weather",
		Description: param.Opt[string]{Value: "Get the current weather for a location"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{
					"type":        "string",
					"description": "The city and state, e.g. San Francisco, CA",
				},
				"unit": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"celsius", "fahrenheit"},
					"description": "The temperature unit to use",
				},
			},
			"required": []string{"location"},
		},
	})
}
