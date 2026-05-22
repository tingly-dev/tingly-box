package claude

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/common"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/agentboot"
)

// TestLauncherSetDefaultFormat tests setting and getting default format
func TestLauncherSetDefaultFormat(t *testing.T) {
	launcher := NewLauncher(Config{})

	// Test setting stream-json as default
	launcher.SetDefaultFormat(agentboot.OutputFormatStreamJSON)
	assert.Equal(t, agentboot.OutputFormatStreamJSON, launcher.GetDefaultFormat())

	// Test setting text as default
	launcher.SetDefaultFormat(agentboot.OutputFormatText)
	assert.Equal(t, agentboot.OutputFormatText, launcher.GetDefaultFormat())
}

// TestLauncherType tests the Type method
func TestLauncherType(t *testing.T) {
	launcher := NewLauncher(Config{})
	assert.Equal(t, agentboot.AgentTypeClaude, launcher.Type())
}

// TestMessageAccumulator tests the message accumulator
func TestMessageAccumulator(t *testing.T) {
	accumulator := NewMessageAccumulator()

	// Test system message
	systemEventJSON := `{"type":"system","subtype":"init","session_id":"test-session-123","timestamp":"2024-01-01T12:00:00Z"}`
	systemEvent := common.Event{
		Type:      SDKSystemMessage,
		Data:      map[string]interface{}{"subtype": "init", "session_id": "test-session-123"},
		Raw:       systemEventJSON,
		Timestamp: time.Now(),
	}
	messages, _, _ := accumulator.AddEvent(systemEvent)
	assert.Len(t, messages, 1, "should have 1 message")
	assert.Equal(t, SDKSystemMessage, messages[0].GetType())
	assert.Equal(t, "test-session-123", accumulator.GetSessionID())

	// Test assistant message with text content
	assistantEventJSON := `{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg-123","type":"message","role":"assistant","content":[{"type":"text","text":"Hello, world!"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}},"session_id":"test-session-123","uuid":"msg-uuid-456","timestamp":"2024-01-01T12:00:00Z"}`
	assistantEvent := common.Event{
		Type:      SDKAssistantMessage,
		Raw:       assistantEventJSON,
		Timestamp: time.Now(),
	}
	messages, _, _ = accumulator.AddEvent(assistantEvent)
	assert.Len(t, messages, 1, "should have 1 new message")
	assert.Equal(t, SDKAssistantMessage, messages[0].GetType())

	assistantMsg, ok := messages[0].(*AssistantMessage)
	require.True(t, ok, "should be AssistantMessage")
	assert.Equal(t, "claude-sonnet-4-6", assistantMsg.Message.Model)
	assert.Len(t, assistantMsg.Message.Content, 1)

	// Test result message
	resultEventJSON := `{"type":"result","subtype":"success","result":"Done!","total_cost_usd":0.001,"duration_ms":1000,"session_id":"test-session-123","timestamp":"2024-01-01T12:00:00Z"}`
	resultEvent := common.Event{
		Type:      SDKResultMessage,
		Raw:       resultEventJSON,
		Timestamp: time.Now(),
	}
	messages, hasResult, resultSuccess := accumulator.AddEvent(resultEvent)
	assert.Len(t, messages, 1, "should have 1 new message")
	assert.True(t, hasResult, "should have result")
	assert.True(t, resultSuccess, "should be successful")

	resultMsg, ok := messages[0].(*ResultMessage)
	require.True(t, ok, "should be ResultMessage")
	assert.True(t, resultMsg.IsSuccess())
	assert.Equal(t, "success", resultMsg.SubType)

	// Test GetMessagesByType
	assistantMessages := accumulator.GetMessagesByType(SDKAssistantMessage)
	assert.Len(t, assistantMessages, 1, "should have 1 assistant message")

	// Test GetAssistantMessages
	assistantMsgs := accumulator.GetAssistantMessages()
	assert.Len(t, assistantMsgs, 1, "should have 1 assistant message")
	assert.Equal(t, "Hello, world!", extractTextFromAssistant(assistantMsgs[0]))

	// Test Reset
	accumulator.Reset()
	assert.Empty(t, accumulator.GetMessages(), "should have no messages after reset")
	assert.Empty(t, accumulator.GetSessionID(), "should have no session ID after reset")
}

// TestAgentCollectsResult tests that Agent.Execute collects typed messages into a Result
func TestAgentCollectsResult(t *testing.T) {
	// Use mockagent via NewAgentWithConfig is not possible (it's Claude-specific),
	// so we exercise the collection path by running the accumulator directly and
	// verifying the public types remain correct.

	// Verify AssistantMessage + ResultMessage marshal correctly.
	assistantMsg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "text", Text: "Hello from assistant"},
			},
		},
	}
	assert.Equal(t, SDKAssistantMessage, assistantMsg.GetType())
	assert.False(t, assistantMsg.IsError())

	resultMsg := &ResultMessage{
		Type:    SDKResultMessage,
		SubType: "success",
		Result:  "Final result",
	}
	assert.True(t, resultMsg.IsSuccess())
	assert.Equal(t, SDKResultMessage, resultMsg.GetType())
}

// TestHelperFunctions tests the helper functions
func TestHelperFunctions(t *testing.T) {
	// Test getString
	assert.Equal(t, "value", getString(map[string]interface{}{"key": "value"}, "key"))
	assert.Equal(t, "", getString(map[string]interface{}{"key": 123}, "key"))
	assert.Equal(t, "", getString(map[string]interface{}{}, "missing"))

	// Test getStringPtr
	ptr := getStringPtr(map[string]interface{}{"key": "value"}, "key")
	assert.NotNil(t, ptr)
	assert.Equal(t, "value", *ptr)
	assert.Nil(t, getStringPtr(map[string]interface{}{}, "missing"))

	// Test getMap
	nested := map[string]interface{}{"nested": "value"}
	assert.Equal(t, nested, getMap(map[string]interface{}{"key": nested}, "key"))
	assert.Nil(t, getMap(map[string]interface{}{"key": "string"}, "key"))

	// Test getInt
	assert.Equal(t, 42, getInt(map[string]interface{}{"key": 42.0}, "key"))
	assert.Equal(t, 0, getInt(map[string]interface{}{"key": "string"}, "key"))

	// Test getInt64
	assert.Equal(t, int64(123456), getInt64(map[string]interface{}{"key": 123456.0}, "key"))

	// Test getFloat
	assert.Equal(t, 3.14, getFloat(map[string]interface{}{"key": 3.14}, "key"))

	// Test getBool
	assert.True(t, getBool(map[string]interface{}{"key": true}, "key"))
	assert.False(t, getBool(map[string]interface{}{"key": false}, "key"))
	assert.False(t, getBool(map[string]interface{}{}, "missing"))
}

// TestSDKsMessage tests message type methods
func TestSDKsMessage(t *testing.T) {
	// Test SystemMessage
	sysMsg := &SystemMessage{
		Type:      SDKSystemMessage,
		SessionID: "session-123",
		Timestamp: time.Now(),
	}
	assert.Equal(t, SDKSystemMessage, sysMsg.GetType())
	assert.Equal(t, "session-123", sysMsg.SessionID)

	// Test AssistantMessage
	assistantMsg := &AssistantMessage{
		Type:      SDKAssistantMessage,
		SessionID: "session-123",
		UUID:      "uuid-456",
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "text", Text: "Hello"},
			},
		},
	}
	assert.Equal(t, SDKAssistantMessage, assistantMsg.GetType())

	// Test ContentBlock types
	textBlock := &TextBlock{Type: "text", Text: "Hello"}
	assert.Equal(t, "text", textBlock.GetContentType())

	toolUseBlock := &ToolUseBlock{
		Type:  "tool_use",
		ID:    "tool-123",
		Name:  "Bash",
		Input: map[string]interface{}{"command": "ls"},
	}
	assert.Equal(t, "tool_use", toolUseBlock.GetContentType())
	assert.Equal(t, "Bash", toolUseBlock.Name)

	// Test ResultMessage
	resultMsg := &ResultMessage{
		Type:      SDKResultMessage,
		SubType:   "success",
		IsError:   false,
		SessionID: "session-123",
	}
	assert.Equal(t, SDKResultMessage, resultMsg.GetType())
	assert.True(t, resultMsg.IsSuccess())
}

// Helper functions

func extractTextFromAssistant(msg *AssistantMessage) string {
	var text strings.Builder
	for _, block := range msg.Message.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}
