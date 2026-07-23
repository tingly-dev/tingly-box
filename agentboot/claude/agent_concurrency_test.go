package claude_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// TestAgent_OverlappingExecutionsIsolateTransportContext reproduces the
// production shape where one registered Agent serves multiple chats. The
// first permission event is emitted only after the second Execute has begun;
// a shared transport would stamp both events with chat-2's routing context.
func TestAgent_OverlappingExecutionsIsolateTransportContext(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			_, _ = io.Copy(io.Discard, h.StdinR)
		}()
	}
	agent := claude.NewAgentWithFactory(claude.Config{}, factory)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first, err := agent.Execute(ctx, "first", agentboot.ExecutionOptions{
		SessionID: "session-1",
		ChatID:    "chat-1",
		Platform:  "telegram",
		BotUUID:   "bot-1",
	})
	require.NoError(t, err)
	second, err := agent.Execute(ctx, "second", agentboot.ExecutionOptions{
		SessionID: "session-2",
		ChatID:    "chat-2",
		Platform:  "discord",
		BotUUID:   "bot-2",
	})
	require.NoError(t, err)

	handles := factory.Handles()
	require.Len(t, handles, 2)
	writeFakeEvent(t, handles[0], map[string]any{
		"type":       claude.SDKControlRequestMessage,
		"request_id": "request-1",
		"request": map[string]any{
			"subtype":   claude.ControlRequestSubtypeCanUseTool,
			"tool_name": "Bash",
			"input":     map[string]any{"command": "pwd"},
		},
	})
	writeFakeEvent(t, handles[1], map[string]any{
		"type":       claude.SDKControlRequestMessage,
		"request_id": "request-2",
		"request": map[string]any{
			"subtype":   claude.ControlRequestSubtypeCanUseTool,
			"tool_name": "Read",
			"input":     map[string]any{"file_path": "README.md"},
		},
	})

	firstApproval := receiveApproval(t, first)
	secondApproval := receiveApproval(t, second)
	assert.Equal(t, "session-1", firstApproval.SessionID)
	assert.Equal(t, "chat-1", firstApproval.ChatID)
	assert.Equal(t, "telegram", firstApproval.Platform)
	assert.Equal(t, "bot-1", firstApproval.BotUUID)
	assert.Equal(t, "session-2", secondApproval.SessionID)
	assert.Equal(t, "chat-2", secondApproval.ChatID)
	assert.Equal(t, "discord", secondApproval.Platform)
	assert.Equal(t, "bot-2", secondApproval.BotUUID)

	require.NoError(t, first.Respond(firstApproval.ID, agentboot.ApprovalResponse{Approved: true}))
	require.NoError(t, second.Respond(secondApproval.ID, agentboot.ApprovalResponse{Approved: true}))

	finishSuccessfulRun(t, handles[0])
	finishSuccessfulRun(t, handles[1])
	for range first.Events() {
	}
	for range second.Events() {
	}
	_, err = first.Wait()
	require.NoError(t, err)
	_, err = second.Wait()
	require.NoError(t, err)
}

func receiveApproval(t *testing.T, handle agentboot.ExecutionHandle) agentboot.ApprovalRequestEvent {
	t.Helper()
	select {
	case event := <-handle.Events():
		approval, ok := event.(agentboot.ApprovalRequestEvent)
		require.True(t, ok, "event type = %T, want ApprovalRequestEvent", event)
		return approval
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval event")
		return agentboot.ApprovalRequestEvent{}
	}
}

func finishSuccessfulRun(t *testing.T, handle *process.FakeHandle) {
	t.Helper()
	writeFakeEvent(t, handle, map[string]any{
		"type":     claude.SDKResultMessage,
		"subtype":  claude.ResultSubtypeSuccess,
		"is_error": false,
	})
	handle.FinishOutput()
	handle.SignalExit(nil)
}

func writeFakeEvent(t *testing.T, handle *process.FakeHandle, event map[string]any) {
	t.Helper()
	data, err := json.Marshal(event)
	require.NoError(t, err)
	data = append(data, '\n')
	_, err = handle.WriteOutput(data)
	require.NoError(t, err)
}
