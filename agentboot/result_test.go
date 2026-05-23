package agentboot_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// assistantEvent builds an assistant event in the real CLI shape:
// {"message":{"content":[{"type":"text","text":...}]}}.
func assistantEvent(text string) agentboot.Event {
	return agentboot.Event{Type: "assistant", Data: map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": text},
			},
		},
	}}
}

func streamResult() *agentboot.Result {
	return &agentboot.Result{
		Format: agentboot.OutputFormatStreamJSON,
		Events: []agentboot.Event{
			{Type: "system", Data: map[string]any{"session_id": "sess-1"}},
			assistantEvent("hello "),
			{Type: "tool_use", Data: map[string]any{"name": "bash"}},
			{Type: "user", Data: map[string]any{"message": "result"}},
			assistantEvent("world"),
			{Type: "control_request", Data: map[string]any{}},
			{Type: "result", Data: map[string]any{"total_cost_usd": 0.42}},
		},
	}
}

func TestResult_TextOutput_ConcatenatesAssistantMessages(t *testing.T) {
	// Real CLI nested content-block shape.
	assert.Equal(t, "hello world", streamResult().TextOutput())

	// Legacy/simple shape where message is already a string.
	legacy := &agentboot.Result{
		Format: agentboot.OutputFormatStreamJSON,
		Events: []agentboot.Event{{Type: "assistant", Data: map[string]any{"message": "hi"}}},
	}
	assert.Equal(t, "hi", legacy.TextOutput())

	text := &agentboot.Result{Format: agentboot.OutputFormatText, Output: "raw text"}
	assert.Equal(t, "raw text", text.TextOutput())
}

func TestResult_MessageQueries(t *testing.T) {
	r := streamResult()

	assert.Len(t, r.GetMessagesByType("assistant"), 2)
	assert.Len(t, r.GetAssistantMessages(), 2)
	assert.Len(t, r.GetUserMessages(), 1)

	// GetMessageChain excludes system, result, and control_* events.
	chain := r.GetMessageChain()
	for _, ev := range chain {
		assert.NotContains(t, []string{"system", "result"}, ev.Type)
		assert.NotEqual(t, "control_request", ev.Type)
	}
	assert.Len(t, chain, 4) // 2 assistant + tool_use + user
}

func TestResult_SessionIDAndCost(t *testing.T) {
	// From events.
	r := streamResult()
	assert.Equal(t, "sess-1", r.GetSessionID())
	assert.InDelta(t, 0.42, r.GetCostUSD(), 1e-9)

	// Metadata takes precedence for the session ID.
	r.Metadata = map[string]any{"session_id": "meta-sess"}
	assert.Equal(t, "meta-sess", r.GetSessionID())
}

func TestResult_IsSuccess(t *testing.T) {
	assert.True(t, (&agentboot.Result{ExitCode: 0}).IsSuccess())
	assert.False(t, (&agentboot.Result{ExitCode: 1}).IsSuccess())
	assert.False(t, (&agentboot.Result{Error: "boom"}).IsSuccess())
}

func TestResult_NilReceiverSafe(t *testing.T) {
	var r *agentboot.Result
	assert.Equal(t, "", r.TextOutput())
	assert.Equal(t, "", r.GetSessionID())
	assert.Zero(t, r.GetCostUSD())
	assert.False(t, r.IsSuccess())
	assert.Nil(t, r.GetMessagesByType("assistant"))
	assert.Nil(t, r.GetMessageChain())
}
