package mutate

import (
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
)

func MaskOpenAIChatRequestCredentials(
	req *openai.ChatCompletionNewParams,
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) (bool, bool) {
	if req == nil || len(credentials) == 0 {
		return false, false
	}
	messages, ok := openAIChatMessagesToMaps(req.Messages)
	if !ok {
		return false, false
	}
	changed := false
	latestTurnChanged := false
	for i := range messages {
		if maskOpenAIMessageMap(messages[i], credentials, state) {
			changed = true
			if i == len(messages)-1 {
				latestTurnChanged = true
			}
		}
	}
	if changed {
		_ = openAIChatMessagesFromMaps(messages, &req.Messages)
	}
	return changed, latestTurnChanged
}

func MaskOpenAIResponsesRequestCredentials(
	req *responses.ResponseNewParams,
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) (bool, bool) {
	if req == nil || len(credentials) == 0 {
		return false, false
	}
	if !param.IsOmitted(req.Input.OfString) {
		if next, ok := guardrailscore.AliasText(req.Input.OfString.Value, credentials, state); ok {
			req.Input.OfString = param.NewOpt(next)
			return true, true
		}
		return false, false
	}
	if param.IsOmitted(req.Input.OfInputItemList) {
		return false, false
	}
	items, ok := openAIResponsesInputItemsToMaps(req.Input.OfInputItemList)
	if !ok {
		return false, false
	}
	changed := false
	latestTurnChanged := false
	for i := range items {
		if maskOpenAIMessageMap(items[i], credentials, state) {
			changed = true
			if i == len(items)-1 {
				latestTurnChanged = true
			}
		}
	}
	if changed {
		_ = openAIResponsesInputItemsFromMaps(items, &req.Input.OfInputItemList)
	}
	return changed, latestTurnChanged
}

func MutateOpenAIChatToolResultRequest(req *openai.ChatCompletionNewParams, evaluation guardrailsevaluate.Evaluation) (bool, string) {
	if req == nil || evaluation.Result.Verdict != guardrailscore.VerdictBlock {
		return false, ""
	}
	message := BlockMessageForToolResult(evaluation.Result)
	if message == "" {
		return false, ""
	}
	messages, ok := openAIChatMessagesToMaps(req.Messages)
	if !ok {
		return false, ""
	}
	changed := false
	for i := range messages {
		role, _ := messages[i]["role"].(string)
		if role != "tool" && role != "function" {
			continue
		}
		messages[i]["content"] = message
		changed = true
	}
	if changed {
		_ = openAIChatMessagesFromMaps(messages, &req.Messages)
	}
	return changed, message
}

func MutateOpenAIResponsesToolResultRequest(req *responses.ResponseNewParams, evaluation guardrailsevaluate.Evaluation) (bool, string) {
	if req == nil || evaluation.Result.Verdict != guardrailscore.VerdictBlock || param.IsOmitted(req.Input.OfInputItemList) {
		return false, ""
	}
	message := BlockMessageForToolResult(evaluation.Result)
	if message == "" {
		return false, ""
	}
	items, ok := openAIResponsesInputItemsToMaps(req.Input.OfInputItemList)
	if !ok {
		return false, ""
	}
	changed := false
	for i := range items {
		itemType, _ := items[i]["type"].(string)
		if itemType != "function_call_output" && itemType != "custom_tool_call_output" && itemType != "mcp_call_output" {
			continue
		}
		items[i]["output"] = message
		changed = true
	}
	if changed {
		_ = openAIResponsesInputItemsFromMaps(items, &req.Input.OfInputItemList)
	}
	return changed, message
}

func MutateOpenAIChatResponse(resp *openai.ChatCompletion, evaluation guardrailsevaluate.Evaluation) (bool, string) {
	if resp == nil || evaluation.Result.Verdict != guardrailscore.VerdictBlock {
		return false, ""
	}
	blockMessage := BlockMessageForEvaluation(evaluation)
	if len(resp.Choices) == 0 {
		resp.Choices = []openai.ChatCompletionChoice{{Index: 0}}
	}
	choice := &resp.Choices[0]
	choice.Message.Content = blockMessage
	choice.Message.Refusal = ""
	choice.Message.ToolCalls = nil
	choice.Message.FunctionCall = openai.ChatCompletionMessageFunctionCall{}
	choice.FinishReason = "stop"
	return true, blockMessage
}

func MutateOpenAIResponsesResponse(resp *responses.Response, evaluation guardrailsevaluate.Evaluation) (bool, string) {
	if resp == nil || evaluation.Result.Verdict != guardrailscore.VerdictBlock {
		return false, ""
	}
	blockMessage := BlockMessageForEvaluation(evaluation)
	itemID := resp.ID + "_guardrails"
	raw := []map[string]interface{}{
		{
			"id":     itemID,
			"type":   "message",
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]interface{}{
				{
					"type": "output_text",
					"text": blockMessage,
				},
			},
		},
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return false, ""
	}
	var output []responses.ResponseOutputItemUnion
	if err := json.Unmarshal(payload, &output); err != nil {
		return false, ""
	}
	resp.Output = output
	resp.Status = "completed"
	return true, blockMessage
}

func RestoreOpenAIChatResponseCredentials(state *guardrailscore.CredentialMaskState, resp *openai.ChatCompletion) bool {
	if state == nil || resp == nil || len(state.AliasToReal) == 0 {
		return false
	}
	changed := false
	for i := range resp.Choices {
		msg := &resp.Choices[i].Message
		if next, ok := guardrailscore.RestoreText(msg.Content, state); ok {
			msg.Content = next
			changed = true
		}
		if next, ok := guardrailscore.RestoreText(msg.Refusal, state); ok {
			msg.Refusal = next
			changed = true
		}
		for j := range msg.ToolCalls {
			if restoreOpenAIChatToolCall(&msg.ToolCalls[j], state) {
				changed = true
			}
		}
	}
	return changed
}

func RestoreOpenAIResponsesResponseCredentials(state *guardrailscore.CredentialMaskState, resp *responses.Response) bool {
	if state == nil || resp == nil || len(state.AliasToReal) == 0 {
		return false
	}
	raw, err := json.Marshal(resp.Output)
	if err != nil || !guardrailscore.MayContainAliasToken(string(raw)) {
		return false
	}
	var parsed interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return false
	}
	restored, changed := guardrailscore.RestoreStructuredValue(parsed, state)
	if !changed {
		return false
	}
	payload, err := json.Marshal(restored)
	if err != nil {
		return false
	}
	var output []responses.ResponseOutputItemUnion
	if err := json.Unmarshal(payload, &output); err != nil {
		return false
	}
	resp.Output = output
	return true
}

func maskOpenAIMessageMap(m map[string]interface{}, credentials []guardrailscore.ProtectedCredential, state *guardrailscore.CredentialMaskState) bool {
	changed := false
	for _, key := range []string{"content", "output"} {
		if next, ok := aliasOpenAIValue(m[key], credentials, state); ok {
			m[key] = next
			changed = true
		}
	}
	for _, key := range []string{"arguments", "input"} {
		if next, ok := aliasOpenAIJSONishString(m[key], credentials, state); ok {
			m[key] = next
			changed = true
		}
	}
	if calls, ok := m["tool_calls"].([]interface{}); ok {
		for _, call := range calls {
			callMap, _ := call.(map[string]interface{})
			for _, key := range []string{"function", "custom"} {
				child, _ := callMap[key].(map[string]interface{})
				for _, argKey := range []string{"arguments", "input"} {
					if next, ok := aliasOpenAIJSONishString(child[argKey], credentials, state); ok {
						child[argKey] = next
						changed = true
					}
				}
			}
		}
	}
	return changed
}

func aliasOpenAIValue(value interface{}, credentials []guardrailscore.ProtectedCredential, state *guardrailscore.CredentialMaskState) (interface{}, bool) {
	switch v := value.(type) {
	case string:
		return guardrailscore.AliasText(v, credentials, state)
	case []interface{}, map[string]interface{}:
		return guardrailscore.AliasStructuredValue(v, credentials, state)
	default:
		return nil, false
	}
}

func aliasOpenAIJSONishString(value interface{}, credentials []guardrailscore.ProtectedCredential, state *guardrailscore.CredentialMaskState) (interface{}, bool) {
	raw, ok := value.(string)
	if !ok || raw == "" {
		return nil, false
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		if next, changed := guardrailscore.AliasStructuredValue(parsed, credentials, state); changed {
			payload, err := json.Marshal(next)
			if err == nil {
				return string(payload), true
			}
		}
	}
	return guardrailscore.AliasText(raw, credentials, state)
}

func restoreOpenAIChatToolCall(call *openai.ChatCompletionMessageToolCallUnion, state *guardrailscore.CredentialMaskState) bool {
	if call == nil {
		return false
	}
	raw := call.RawJSON()
	if raw == "" || !guardrailscore.MayContainAliasToken(raw) {
		return false
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return false
	}
	restored, changed := guardrailscore.RestoreStructuredValue(parsed, state)
	if !changed {
		return false
	}
	payload, err := json.Marshal(restored)
	if err != nil {
		return false
	}
	var next openai.ChatCompletionMessageToolCallUnion
	if err := json.Unmarshal(payload, &next); err != nil {
		return false
	}
	*call = next
	return true
}

func openAIChatMessagesToMaps(messages []openai.ChatCompletionMessageParamUnion) ([]map[string]interface{}, bool) {
	raw, err := json.Marshal(messages)
	if err != nil {
		return nil, false
	}
	var out []map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	return out, true
}

func openAIChatMessagesFromMaps(messages []map[string]interface{}, target *[]openai.ChatCompletionMessageParamUnion) bool {
	raw, err := json.Marshal(messages)
	if err != nil {
		return false
	}
	var out []openai.ChatCompletionMessageParamUnion
	if err := json.Unmarshal(raw, &out); err != nil {
		return false
	}
	*target = out
	return true
}

func openAIResponsesInputItemsToMaps(items []responses.ResponseInputItemUnionParam) ([]map[string]interface{}, bool) {
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, false
	}
	var out []map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	return out, true
}

func openAIResponsesInputItemsFromMaps(items []map[string]interface{}, target *responses.ResponseInputParam) bool {
	raw, err := json.Marshal(items)
	if err != nil {
		return false
	}
	var out responses.ResponseInputParam
	if err := json.Unmarshal(raw, &out); err != nil {
		return false
	}
	*target = out
	return true
}
