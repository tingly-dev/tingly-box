package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const workbuddyToolUserChat = `{"model":"m","messages":[{"role":"system","content":"Keep all instructions."},{"role":"user","content":"Read the file."},{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"read","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call-1","content":"file contents"},{"role":"user","content":[{"type":"text","text":"Now summarize it."},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`

func TestWorkBuddyMessageShapes(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantSystem bool
	}{
		{"user first", `{"messages":[{"role":"user","content":"hello"}]}`, true},
		{"developer first", `{"messages":[{"role":"developer","content":"My instructions"},{"role":"user","content":"hello"}]}`, true},
		{"empty", `{"messages":[]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fitted, _ := workbuddyFitChat([]byte(tc.body))
			body := workbuddyWithSystem(fitted)
			_, _, messages := workbuddyChatMessages(body)
			if tc.wantSystem {
				require.Equal(t, "system", messages[0].Role)
			} else {
				require.Empty(t, messages)
			}
			if tc.name == "developer first" {
				require.JSONEq(t, `"My instructions"`, string(messages[0].Content))
				require.Len(t, messages, 2)
			}
		})
	}
	// Existing system prompts and client identities are preserved byte for byte.
	withSystem := `{"messages": [{"role":"system","content":"You are an AI agent powered by DeepSeek Harness."}]}`
	require.Equal(t, withSystem, string(workbuddyWithSystem([]byte(withSystem))))
}

func TestWorkBuddyReplyAfterToolsPreservesMessages(t *testing.T) {
	out, ok := workbuddyReplyAfterTools([]byte(workbuddyToolUserChat))
	require.True(t, ok)
	_, raw, before := workbuddyChatMessages([]byte(workbuddyToolUserChat))
	_, fixed, after := workbuddyChatMessages(out)
	require.Len(t, after, len(before)+1)
	require.Equal(t, "assistant", after[4].Role)
	require.JSONEq(t, `""`, string(after[4].Content))
	for i, m := range raw {
		j := i
		if i >= 4 {
			j++
		}
		require.JSONEq(t, string(m), string(fixed[j]))
	}
	// Applying it again is a no-op, not another empty message.
	_, ok = workbuddyReplyAfterTools(out)
	require.False(t, ok)
	for _, body := range []string{
		`{"messages":[{"role":"tool","tool_call_id":"missing","content":"x"},{"role":"user","content":"hi"}]}`,
		`{"messages":[{"role":"assistant","tool_calls":[{"id":"one"},{"id":"two"}]},{"role":"tool","tool_call_id":"one","content":"x"},{"role":"user","content":"hi"}]}`,
		`{"messages":[{"role":"assistant","content":"prefill"}]}`,
	} {
		_, ok = workbuddyReplyAfterTools([]byte(body))
		require.False(t, ok)
	}
	for _, code := range []int{11128, 11140, 11133} {
		body, _ := json.Marshal(map[string]any{"code": code, "msg": "the conversation must end with a user message"})
		require.False(t, workbuddyAfterToolsRefused(body))
	}
	require.True(t, workbuddyAfterToolsRefused([]byte(`{"code":11153,"msg":"the conversation must end with a user message"}`)))
}
