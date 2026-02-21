package claude

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/events"
)

// TestLauncherTextFormat tests Claude Code execution in text format
func TestLauncherTextFormat(t *testing.T) {
	t.SkipNow()

	// Skip if claude CLI is not available
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatText,
		Timeout:      30 * time.Second,
	}

	// Simple prompt that should return quickly
	prompt := "echo hello"

	result, err := launcher.Execute(ctx, prompt, opts)

	// Check result
	require.NoError(t, err, "execution should succeed")
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0")
	assert.Empty(t, result.Error, "error should be empty")
	assert.Equal(t, agentboot.OutputFormatText, result.Format)
	assert.Greater(t, result.Duration, 0, "duration should be positive")
	assert.NotEmpty(t, result.Output, "output should not be empty")
	assert.Contains(t, result.Output, "hello", "output should contain 'hello'")
}

// TestLauncherStreamJSONFormat tests Claude Code execution in stream-json format
func TestLauncherStreamJSONFormat(t *testing.T) {
	t.SkipNow()

	// Skip if claude CLI is not available
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Timeout:      300 * time.Second,
	}

	// Simple prompt that should return quickly
	prompt := "run bash ls"

	result, err := launcher.Execute(ctx, prompt, opts)
	for _, it := range result.Events {
		fmt.Printf("%s\n", it)
	}

	// Check result
	require.NoError(t, err, "execution should succeed")
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0")
	assert.Empty(t, result.Error, "error should be empty")
	assert.Equal(t, agentboot.OutputFormatStreamJSON, result.Format)
	assert.Greater(t, result.Duration, 0, "duration should be positive")

	// Check events
	assert.NotEmpty(t, result.Events, "should have events")

	// Verify we have expected event types (new format: assistant, result)
	hasAssistant := false
	hasResult := false
	for _, event := range result.Events {
		if event.Type == MessageTypeAssistant {
			hasAssistant = true
		}
		if event.Type == MessageTypeResult {
			hasResult = true
		}
	}
	assert.True(t, hasAssistant, "should have assistant events")
	assert.True(t, hasResult, "should have result events")

	// Check text output
	textOutput := result.TextOutput()
	assert.NotEmpty(t, textOutput, "text output should not be empty")
	assert.Contains(t, strings.ToLower(textOutput), "hello", "output should contain 'hello'")
}

// TestExecuteWithHandler tests execution with a message handler
func TestExecuteWithHandler(t *testing.T) {
	// Skip if claude CLI is not available
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Timeout:      30 * time.Second,
	}

	prompt := "say hello in one word"

	// Create a test handler
	handler := &TestMessageHandler{
		messages: make([]Message, 0),
	}

	err := launcher.ExecuteWithHandler(ctx, prompt, opts.Timeout, opts, handler)

	require.NoError(t, err, "execution should succeed")
	assert.NotEmpty(t, handler.messages, "handler should receive messages")

	// Check message types
	hasAssistant := false
	hasResult := false
	for _, msg := range handler.messages {
		if msg.GetType() == MessageTypeAssistant {
			hasAssistant = true
		}
		if msg.GetType() == MessageTypeResult {
			hasResult = true
		}
	}
	assert.True(t, hasAssistant, "should have assistant message")
	assert.True(t, hasResult, "should have result message")
	assert.True(t, handler.completed, "should be completed")
	assert.True(t, handler.success, "should be successful")
}

// TestExecuteStream tests streaming execution with channel-based handler
func TestExecuteStream(t *testing.T) {
	// Skip if claude CLI is not available
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Timeout:      30 * time.Second,
	}

	prompt := "say hi"

	streamHandler, err := launcher.ExecuteStream(ctx, prompt, opts.Timeout, opts)
	require.NoError(t, err, "stream execution should start")
	assert.NotNil(t, streamHandler)

	// Collect messages from the stream
	messages := make([]Message, 0)
	timeout := time.After(35 * time.Second)

loop:
	for {
		select {
		case msg, ok := <-streamHandler.Messages():
			if !ok {
				break loop
			}
			messages = append(messages, msg)
		case err, ok := <-streamHandler.Errors():
			if ok && err != nil {
				t.Errorf("unexpected error from stream: %v", err)
			}
		case <-timeout:
			t.Fatal("timed out waiting for messages")
		}
	}

	assert.NotEmpty(t, messages, "should receive messages")

	// Verify result
	result := streamHandler.GetResult()
	assert.NotNil(t, result, "should have result")
	assert.True(t, result.Success, "result should indicate success")

	// Check session ID
	sessionID := streamHandler.GetSessionID()
	assert.NotEmpty(t, sessionID, "should have session ID")
}

// TestLauncherWithProjectPath tests execution with a project path
func TestLauncherWithProjectPath(t *testing.T) {
	// Skip if claude CLI is not available
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	// Get current directory as project path
	projectPath, err := os.Getwd()
	require.NoError(t, err)

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatText,
		Timeout:      30 * time.Second,
		ProjectPath:  projectPath,
	}

	prompt := "what files are in this directory? list just the go files"

	result, err := launcher.Execute(ctx, prompt, opts)

	// Check result
	require.NoError(t, err, "execution should succeed")
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotEmpty(t, result.Output)
}

// TestLauncherTimeout tests execution timeout
func TestLauncherTimeout(t *testing.T) {
	// Skip if claude CLI is not available
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatText,
		Timeout:      1 * time.Nanosecond, // Very short timeout
	}

	prompt := "count to 100 slowly"

	result, err := launcher.Execute(ctx, prompt, opts)

	// Should timeout
	assert.Error(t, err, "execution should timeout")
	assert.NotNil(t, result)
	assert.Contains(t, result.Error, "timed out", "error should mention timeout")
}

// TestLauncherNotAvailable tests behavior when CLI is not available
func TestLauncherNotAvailable(t *testing.T) {
	launcher := NewLauncher(Config{})
	// Set an invalid CLI path
	launcher.SetCLIPath("nonexistent-cli-command-xyz123")

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatText,
		Timeout:      5 * time.Second,
	}

	result, err := launcher.Execute(ctx, "test", opts)

	// Should fail with exec.ErrNotFound or similar
	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Error)
}

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
	systemEvent := events.Event{
		Type:      MessageTypeSystem,
		Data:      map[string]interface{}{"subtype": "init", "session_id": "test-session-123"},
		Timestamp: time.Now(),
	}
	messages, _, _ := accumulator.AddEvent(systemEvent)
	assert.Len(t, messages, 1, "should have 1 message")
	assert.Equal(t, MessageTypeSystem, messages[0].GetType())
	assert.Equal(t, "test-session-123", accumulator.GetSessionID())

	// Test assistant message with text content
	assistantEvent := events.Event{
		Type: MessageTypeAssistant,
		Data: map[string]interface{}{
			"message": map[string]interface{}{
				"model": "claude-sonnet-4-6",
				"id":    "msg-123",
				"type":  "message",
				"role":  "assistant",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "Hello, world!"},
				},
				"stop_reason": "end_turn",
				"usage": map[string]interface{}{
					"input_tokens":  10,
					"output_tokens": 5,
				},
			},
			"session_id": "test-session-123",
			"uuid":       "msg-uuid-456",
		},
		Timestamp: time.Now(),
	}
	messages, _, _ = accumulator.AddEvent(assistantEvent)
	assert.Len(t, messages, 1, "should have 1 new message")
	assert.Equal(t, MessageTypeAssistant, messages[0].GetType())

	assistantMsg, ok := messages[0].(*AssistantMessage)
	require.True(t, ok, "should be AssistantMessage")
	assert.Equal(t, "claude-sonnet-4-6", assistantMsg.Message.Model)
	assert.Len(t, assistantMsg.Message.Content, 1)

	// Test result message
	resultEvent := events.Event{
		Type: MessageTypeResult,
		Data: map[string]interface{}{
			"subtype":        "success",
			"result":         "Done!",
			"total_cost_usd": 0.001,
			"duration_ms":    1000,
			"session_id":     "test-session-123",
		},
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
	assistantMessages := accumulator.GetMessagesByType(MessageTypeAssistant)
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

// TestStreamHandler tests the stream handler
func TestStreamHandler(t *testing.T) {
	handler := NewStreamHandler(10)
	defer handler.Close()

	// Test HandleEvent with system message
	systemEvent := events.Event{
		Type:      MessageTypeSystem,
		Data:      map[string]interface{}{"subtype": "init", "session_id": "test-123"},
		Timestamp: time.Now(),
	}
	err := handler.HandleEvent(systemEvent)
	assert.NoError(t, err)

	// Test HandleEvent with assistant message
	assistantEvent := events.Event{
		Type: MessageTypeAssistant,
		Data: map[string]interface{}{
			"message": map[string]interface{}{
				"model":       "claude-sonnet-4-6",
				"id":          "msg-123",
				"type":        "message",
				"role":        "assistant",
				"stop_reason": "end_turn",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "Test message"},
				},
			},
			"session_id": "test-123",
		},
		Timestamp: time.Now(),
	}
	err = handler.HandleEvent(assistantEvent)
	assert.NoError(t, err)

	// Test HandleEvent with result message
	resultEvent := events.Event{
		Type: MessageTypeResult,
		Data: map[string]interface{}{
			"subtype": "success",
			"result":  "Complete",
		},
		Timestamp: time.Now(),
	}
	err = handler.HandleEvent(resultEvent)
	assert.NoError(t, err)

	// Check messages from channel
	select {
	case msg := <-handler.Messages():
		assert.Equal(t, MessageTypeSystem, msg.GetType())
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for message")
	}

	select {
	case msg := <-handler.Messages():
		assert.Equal(t, MessageTypeAssistant, msg.GetType())
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for message")
	}

	select {
	case msg := <-handler.Messages():
		assert.Equal(t, MessageTypeResult, msg.GetType())
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for message")
	}

	// Check result
	result := handler.GetResult()
	assert.NotNil(t, result)
	assert.True(t, result.Success)

	// Check session ID
	sessionID := handler.GetSessionID()
	assert.Equal(t, "test-123", sessionID)

	// Test IsClosed
	assert.False(t, handler.IsClosed())

	// Close and check
	handler.Close()
	assert.True(t, handler.IsClosed())
}

// TestResultCollector tests the result collector
func TestResultCollector(t *testing.T) {
	collector := NewResultCollector()

	// Test OnMessage with assistant message
	assistantMsg := &AssistantMessage{
		Type: MessageTypeAssistant,
		Message: MessageData{
			Content: []ContentBlock{
				&TextBlock{Type: "text", Text: "Hello from assistant"},
			},
		},
	}
	err := collector.OnMessage(assistantMsg)
	assert.NoError(t, err)
	assert.Contains(t, collector.Result().Output, "Hello from assistant")

	// Test OnMessage with result message
	resultMsg := &ResultMessage{
		Type:    MessageTypeResult,
		SubType: "success",
		Result:  "Final result",
	}
	err = collector.OnMessage(resultMsg)
	assert.NoError(t, err)
	assert.True(t, collector.IsComplete())

	// Test Result
	result := collector.Result()
	assert.Equal(t, agentboot.OutputFormatStreamJSON, result.Format)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotEmpty(t, result.Events)

	// Test GetMessages
	messages := collector.GetMessages()
	assert.Len(t, messages, 2)

	// Test BuildTextOutput
	textOutput := collector.BuildTextOutput()
	assert.Contains(t, textOutput, "Hello from assistant")
}

// TestMultiHandler tests the multi handler
func TestMultiHandler(t *testing.T) {
	handler1 := &TestMessageHandler{messages: make([]Message, 0)}
	handler2 := &TestMessageHandler{messages: make([]Message, 0)}

	multi := NewMultiHandler(handler1, handler2)

	msg := &AssistantMessage{
		Type: MessageTypeAssistant,
		Message: MessageData{
			Content: []ContentBlock{
				&TextBlock{Type: "text", Text: "Test"},
			},
		},
	}

	err := multi.OnMessage(msg)
	assert.NoError(t, err)

	assert.Len(t, handler1.messages, 1)
	assert.Len(t, handler2.messages, 1)

	multi.OnComplete(&ResultCompletion{Success: true})

	assert.True(t, handler1.completed)
	assert.True(t, handler2.completed)
}

// TestCallbackHandler tests the callback handler
func TestCallbackHandler(t *testing.T) {
	var receivedMsg Message
	var completed bool

	handler := NewCallbackHandler(
		func(msg Message) error {
			receivedMsg = msg
			return nil
		},
		nil,
		func(completion *ResultCompletion) {
			completed = true
		},
	)

	msg := &AssistantMessage{
		Type: MessageTypeAssistant,
		Message: MessageData{
			Content: []ContentBlock{
				&TextBlock{Type: "text", Text: "Test"},
			},
		},
	}

	handler.OnMessage(msg)
	handler.OnComplete(&ResultCompletion{Success: true})

	assert.Same(t, msg, receivedMsg)
	assert.True(t, completed)
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

// TestMessageTypes tests message type methods
func TestMessageTypes(t *testing.T) {
	// Test SystemMessage
	sysMsg := &SystemMessage{
		Type:      MessageTypeSystem,
		SessionID: "session-123",
		Timestamp: time.Now(),
	}
	assert.Equal(t, MessageTypeSystem, sysMsg.GetType())
	assert.Equal(t, "session-123", sysMsg.SessionID)

	// Test AssistantMessage
	assistantMsg := &AssistantMessage{
		Type:      MessageTypeAssistant,
		SessionID: "session-123",
		UUID:      "uuid-456",
		Message: MessageData{
			Model: "claude-sonnet-4-6",
			Role:  "assistant",
			Content: []ContentBlock{
				&TextBlock{Type: "text", Text: "Hello"},
			},
		},
	}
	assert.Equal(t, MessageTypeAssistant, assistantMsg.GetType())
	assert.Equal(t, "assistant", assistantMsg.Message.Role)

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
		Type:      MessageTypeResult,
		SubType:   "success",
		IsError:   false,
		SessionID: "session-123",
	}
	assert.Equal(t, MessageTypeResult, resultMsg.GetType())
	assert.True(t, resultMsg.IsSuccess())
}

// TestListenerFunc tests the listener function adapter
func TestListenerFunc(t *testing.T) {
	var received Message

	listener := ListenerFunc(func(msg Message) {
		received = msg
	})

	msg := &AssistantMessage{
		Type: MessageTypeAssistant,
		Message: MessageData{
			Content: []ContentBlock{
				&TextBlock{Type: "text", Text: "Test"},
			},
		},
	}

	listener.OnMessage(msg)
	assert.Same(t, msg, received)
}

// TestMessageHandlerFunc tests the message handler function adapter
func TestMessageHandlerFunc(t *testing.T) {
	var received Message

	handler := MessageHandlerFunc(func(msg Message) error {
		received = msg
		return nil
	})

	msg := &AssistantMessage{
		Type: MessageTypeAssistant,
		Message: MessageData{
			Content: []ContentBlock{
				&TextBlock{Type: "text", Text: "Test"},
			},
		},
	}

	err := handler.OnMessage(msg)
	assert.NoError(t, err)
	assert.Same(t, msg, received)

	// Test OnError and OnComplete (should not panic)
	handler.OnError(assert.AnError)
	handler.OnComplete(&ResultCompletion{Success: true})
}

// Helper functions

// TestMessageHandler is a test implementation of MessageHandler
type TestMessageHandler struct {
	messages  []Message
	errors    []error
	completed bool
	success   bool
	mu        sync.Mutex
}

func (h *TestMessageHandler) OnMessage(msg Message) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages = append(h.messages, msg)
	fmt.Printf("TestMessageHandler OnMessage %v\n", msg)
	return nil
}

func (h *TestMessageHandler) OnError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errors = append(h.errors, err)
	fmt.Printf("TestMessageHandler OnError %v\n", err)
}

func (h *TestMessageHandler) OnComplete(result *ResultCompletion) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.completed = true
	if result != nil {
		h.success = result.Success
	}
}

func extractTextFromAssistant(msg *AssistantMessage) string {
	var text strings.Builder
	for _, block := range msg.Message.Content {
		if textBlock, ok := block.(*TextBlock); ok {
			text.WriteString(textBlock.Text)
		}
	}
	return text.String()
}
