package adapter

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

func AdaptMessagesFromOpenAIChat(messages []openai.ChatCompletionMessageParamUnion) []guardrailscore.Message {
	out := make([]guardrailscore.Message, 0, len(messages))
	for _, msg := range messages {
		m := openAIParamAsMap(msg)
		role, _ := m["role"].(string)
		content := textFromOpenAIValue(m["content"])
		if role == "assistant" {
			content = strings.TrimSpace(strings.Join(nonEmpty(content, commandTextFromOpenAIChatMessage(m)), "\n"))
		}
		if role == "" && content == "" {
			continue
		}
		out = append(out, guardrailscore.Message{Role: role, Content: content})
	}
	return out
}

func RefreshInputFromOpenAIChatRequest(input guardrailscore.Input) guardrailscore.Input {
	req, _ := input.Payload.Request.(*openai.ChatCompletionNewParams)
	if req == nil {
		return input
	}
	text, blockCount, partCount := ExtractOpenAIChatToolResultText(req.Messages)
	input.Direction = guardrailscore.DirectionRequest
	input.Content = guardrailscore.Content{
		Text:     text,
		Messages: AdaptMessagesFromOpenAIChat(req.Messages),
	}
	input.HasToolResult = blockCount > 0
	input.ToolResultBlockCount = blockCount
	input.ToolResultPartCount = partCount
	if input.Payload.Protocol == "" {
		input.Payload.Protocol = "openai_chat"
	}
	input.Payload.Request = req
	return input
}

func RefreshInputFromOpenAIChatResponse(input guardrailscore.Input, resp *openai.ChatCompletion) guardrailscore.Input {
	input.Direction = guardrailscore.DirectionResponse
	input.Content = guardrailscore.Content{Messages: input.Content.Messages}
	if resp == nil || len(resp.Choices) == 0 {
		return input
	}
	msg := resp.Choices[0].Message
	input.Content.Text = strings.TrimSpace(strings.Join(nonEmpty(msg.Content, msg.Refusal), "\n"))
	if len(msg.ToolCalls) > 0 {
		tc := msg.ToolCalls[0]
		switch v := tc.AsAny().(type) {
		case openai.ChatCompletionMessageFunctionToolCall:
			input.Content.Command = BuildCommandFromRawArguments(v.Function.Name, v.Function.Arguments)
		case openai.ChatCompletionMessageCustomToolCall:
			input.Content.Command = BuildCommandFromRawArguments(v.Custom.Name, v.Custom.Input)
		}
	} else if msg.FunctionCall.Name != "" || msg.FunctionCall.Arguments != "" {
		input.Content.Command = BuildCommandFromRawArguments(msg.FunctionCall.Name, msg.FunctionCall.Arguments)
	}
	return input
}

func ExtractOpenAIChatToolResultText(messages []openai.ChatCompletionMessageParamUnion) (string, int, int) {
	for i := len(messages) - 1; i >= 0; i-- {
		m := openAIParamAsMap(messages[i])
		role, _ := m["role"].(string)
		if role != "tool" && role != "function" {
			continue
		}
		text := textFromOpenAIValue(m["content"])
		if text == "" {
			if raw, err := json.Marshal(m); err == nil {
				text = string(raw)
			}
		}
		return text, 1, countOpenAIContentParts(m["content"])
	}
	return "", 0, 0
}

func AdaptMessagesFromOpenAIResponses(req *responses.ResponseNewParams) []guardrailscore.Message {
	if req == nil {
		return nil
	}
	if !param.IsOmitted(req.Input.OfString) {
		return []guardrailscore.Message{{Role: "user", Content: req.Input.OfString.Value}}
	}
	items := req.Input.OfInputItemList
	out := make([]guardrailscore.Message, 0, len(items))
	for _, item := range items {
		m := openAIParamAsMap(item)
		role, _ := m["role"].(string)
		itemType, _ := m["type"].(string)
		if role == "" {
			switch itemType {
			case "function_call", "custom_tool_call", "mcp_call":
				role = "assistant"
			case "function_call_output", "custom_tool_call_output", "mcp_call_output":
				role = "tool"
			}
		}
		content := textFromOpenAIValue(m["content"])
		if content == "" {
			content = textFromOpenAIValue(m["output"])
		}
		if role == "assistant" {
			content = strings.TrimSpace(strings.Join(nonEmpty(content, commandTextFromOpenAIResponsesItem(m)), "\n"))
		}
		if role == "" && content == "" {
			continue
		}
		out = append(out, guardrailscore.Message{Role: role, Content: content})
	}
	return out
}

func RefreshInputFromOpenAIResponsesRequest(input guardrailscore.Input) guardrailscore.Input {
	req, _ := input.Payload.Request.(*responses.ResponseNewParams)
	if req == nil {
		return input
	}
	text, blockCount, partCount := ExtractOpenAIResponsesToolResultText(req)
	input.Direction = guardrailscore.DirectionRequest
	input.Content = guardrailscore.Content{
		Text:     text,
		Messages: AdaptMessagesFromOpenAIResponses(req),
	}
	input.HasToolResult = blockCount > 0
	input.ToolResultBlockCount = blockCount
	input.ToolResultPartCount = partCount
	if input.Payload.Protocol == "" {
		input.Payload.Protocol = "openai_responses"
	}
	input.Payload.Request = req
	return input
}

func RefreshInputFromOpenAIResponsesResponse(input guardrailscore.Input, resp *responses.Response) guardrailscore.Input {
	input.Direction = guardrailscore.DirectionResponse
	input.Content = guardrailscore.Content{Messages: input.Content.Messages}
	if resp == nil {
		return input
	}
	var textParts []string
	for _, item := range resp.Output {
		m := openAIParamAsMap(item)
		switch m["type"] {
		case "message":
			textParts = append(textParts, textFromOpenAIValue(m["content"]))
		case "function_call", "custom_tool_call", "mcp_call":
			if input.Content.Command == nil {
				input.Content.Command = commandFromOpenAIResponsesMap(m)
			}
		}
	}
	input.Content.Text = strings.TrimSpace(strings.Join(nonEmpty(textParts...), "\n"))
	return input
}

func ExtractOpenAIResponsesToolResultText(req *responses.ResponseNewParams) (string, int, int) {
	if req == nil || param.IsOmitted(req.Input.OfInputItemList) {
		return "", 0, 0
	}
	items := req.Input.OfInputItemList
	for i := len(items) - 1; i >= 0; i-- {
		m := openAIParamAsMap(items[i])
		itemType, _ := m["type"].(string)
		if itemType != "function_call_output" && itemType != "custom_tool_call_output" && itemType != "mcp_call_output" {
			continue
		}
		text := textFromOpenAIValue(m["output"])
		if text == "" {
			if raw, err := json.Marshal(m); err == nil {
				text = string(raw)
			}
		}
		return text, 1, countOpenAIContentParts(m["output"])
	}
	return "", 0, 0
}

func commandFromOpenAIResponsesMap(m map[string]interface{}) *guardrailscore.Command {
	name, _ := m["name"].(string)
	args := ""
	if raw, ok := m["arguments"].(string); ok {
		args = raw
	} else if raw, ok := m["input"].(string); ok {
		args = raw
	}
	return BuildCommandFromRawArguments(name, args)
}

func commandTextFromOpenAIChatMessage(m map[string]interface{}) string {
	if fc, ok := m["function_call"].(map[string]interface{}); ok {
		name, _ := fc["name"].(string)
		args, _ := fc["arguments"].(string)
		return commandPreview(name, args)
	}
	calls, ok := m["tool_calls"].([]interface{})
	if !ok || len(calls) == 0 {
		return ""
	}
	call, _ := calls[0].(map[string]interface{})
	if fn, ok := call["function"].(map[string]interface{}); ok {
		name, _ := fn["name"].(string)
		args, _ := fn["arguments"].(string)
		return commandPreview(name, args)
	}
	if custom, ok := call["custom"].(map[string]interface{}); ok {
		name, _ := custom["name"].(string)
		args, _ := custom["input"].(string)
		return commandPreview(name, args)
	}
	return ""
}

func commandTextFromOpenAIResponsesItem(m map[string]interface{}) string {
	cmd := commandFromOpenAIResponsesMap(m)
	if cmd == nil {
		return ""
	}
	raw := ""
	if len(cmd.Arguments) > 0 {
		if b, err := json.Marshal(cmd.Arguments); err == nil {
			raw = string(b)
		}
	}
	return commandPreview(cmd.Name, raw)
}

func commandPreview(name, args string) string {
	if name == "" && args == "" {
		return ""
	}
	if args == "" {
		return "command: " + name
	}
	return "command: " + name + " arguments: " + args
}

func openAIParamAsMap(v interface{}) map[string]interface{} {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func textFromOpenAIValue(v interface{}) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if m, ok := item.(map[string]interface{}); ok {
				for _, key := range []string{"text", "refusal", "output_text"} {
					if text, ok := m[key].(string); ok && text != "" {
						parts = append(parts, text)
						break
					}
				}
			} else if text := textFromOpenAIValue(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]interface{}:
		for _, key := range []string{"text", "refusal", "output", "content"} {
			if text := textFromOpenAIValue(value[key]); text != "" {
				return text
			}
		}
	}
	return ""
}

func countOpenAIContentParts(v interface{}) int {
	if parts, ok := v.([]interface{}); ok {
		if len(parts) > 0 {
			return len(parts)
		}
	}
	if v != nil {
		return 1
	}
	return 0
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
