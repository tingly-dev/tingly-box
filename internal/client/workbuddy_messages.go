package client

import (
	"encoding/json"
	"strings"
)

type workbuddyMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id"`
	ToolCalls  []struct {
		ID string `json:"id"`
	} `json:"tool_calls"`
}

func workbuddyChatMessages(body []byte) (map[string]json.RawMessage, []json.RawMessage, []workbuddyMessage) {
	var chat map[string]json.RawMessage
	var raw []json.RawMessage
	var messages []workbuddyMessage
	if json.Unmarshal(body, &chat) != nil || json.Unmarshal(chat["messages"], &raw) != nil ||
		json.Unmarshal(chat["messages"], &messages) != nil || len(messages) == 0 {
		return nil, nil, nil
	}
	return chat, raw, messages
}

func workbuddyWriteMessages(chat map[string]json.RawMessage, messages []json.RawMessage, fallback []byte) []byte {
	chat["messages"], _ = json.Marshal(messages)
	if body, err := json.Marshal(chat); err == nil {
		return body
	}
	return fallback
}

// The desktop endpoint requires a leading system message. Keep the user's
// instructions and every original message; never reword a client identity.
func workbuddyWithSystem(body []byte) []byte {
	chat, raw, messages := workbuddyChatMessages(body)
	if len(messages) == 0 || messages[0].Role == "system" {
		return body
	}
	leading := json.RawMessage(`{"role":"system","content":"You are a helpful assistant."}`)
	return workbuddyWriteMessages(chat, append([]json.RawMessage{leading}, raw...), body)
}

// Some WorkBuddy models reject a complete tool-result batch immediately
// followed by user input. On that specific refusal only, an empty assistant
// reply separates the two without turning user instructions into tool output.
// Incomplete tool batches and assistant prefills are left for the upstream to
// reject, rather than repaired by inventing results or dropping messages.
func workbuddyReplyAfterTools(body []byte) ([]byte, bool) {
	chat, raw, messages := workbuddyChatMessages(body)
	i := len(messages) - 1
	for i >= 0 && messages[i].Role == "user" {
		i--
	}
	if i < 0 || i == len(messages)-1 || messages[i].Role != "tool" {
		return body, false
	}
	lastTool := i
	for i >= 0 && messages[i].Role == "tool" {
		i--
	}
	if i < 0 || messages[i].Role != "assistant" || len(messages[i].ToolCalls) == 0 {
		return body, false
	}
	pending := make(map[string]bool)
	for _, call := range messages[i].ToolCalls {
		if call.ID == "" || pending[call.ID] {
			return body, false
		}
		pending[call.ID] = true
	}
	for _, result := range messages[i+1 : lastTool+1] {
		if !pending[result.ToolCallID] {
			return body, false
		}
		delete(pending, result.ToolCallID)
	}
	if len(pending) != 0 {
		return body, false
	}
	out := make([]json.RawMessage, 0, len(raw)+1)
	out = append(out, raw[:lastTool+1]...)
	out = append(out, json.RawMessage(`{"role":"assistant","content":""}`))
	out = append(out, raw[lastTool+1:]...)
	return workbuddyWriteMessages(chat, out, body), true
}

func workbuddyAfterToolsRefused(raw []byte) bool {
	var env struct {
		Code    json.RawMessage `json:"code"`
		Msg     string          `json:"msg"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return false
	}
	return (string(env.Code) == "11153" || string(env.Code) == `"11153"`) &&
		strings.Contains(strings.ToLower(env.Msg+" "+env.Message), "must end with a user message")
}
