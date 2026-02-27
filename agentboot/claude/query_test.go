package claude

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestQueryNext tests the Next() method of Query
func TestQueryNext(t *testing.T) {
	// Create mock stdin/stdout
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a process done channel that closes after writing
	processDone := make(chan struct{})

	query := NewQuery(ctx, nil, reader, QueryOptions{
		ProcessDone: processDone,
	})
	defer query.Close()

	// Write some messages to stdout
	go func() {
		messages := []map[string]interface{}{
			{"type": "system", "subtype": "init", "session_id": "test-123"},
			{"type": "assistant", "session_id": "test-123", "message": map[string]interface{}{"role": "assistant"}},
			{"type": "result", "subtype": "success", "result": "Done!"},
		}

		for _, msg := range messages {
			data, _ := json.Marshal(msg)
			data = append(data, '\n')
			writer.Write(data)
			time.Sleep(10 * time.Millisecond) // Small delay between messages
		}

		// Close writer to signal EOF
		writer.Close()
		close(processDone)
	}()

	// Read messages
	count := 0
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatalf("timeout reading messages, got %d messages", count)
		default:
			msg, ok := query.Next()
			if !ok {
				goto done
			}
			count++
			assert.NotEmpty(t, msg.Type)
		}
	}
done:
	assert.Equal(t, 3, count)
}

// TestQueryMessagesChannel tests the Messages() channel
func TestQueryMessagesChannel(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	query := NewQuery(ctx, nil, reader, QueryOptions{})

	go func() {
		msg := map[string]interface{}{"type": "test", "value": "hello"}
		data, _ := json.Marshal(msg)
		data = append(data, '\n')
		writer.Write(data)
		writer.Close()
	}()

	select {
	case msg := <-query.Messages():
		assert.Equal(t, "test", msg.Type)
		assert.Equal(t, "hello", msg.RawData["value"])
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for message")
	}

	query.Close()
}

// TestQueryControlResponse tests control response handling
func TestQueryControlResponse(t *testing.T) {
	reader, writer := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	defer stdinReader.Close()
	defer stdinWriter.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	query := NewQuery(ctx, stdinWriter, reader, QueryOptions{})

	// Simulate Claude sending a control response
	go func() {
		resp := map[string]interface{}{
			"type": "control_response",
			"response": map[string]interface{}{
				"subtype":    "success",
				"request_id": "req-123",
				"data":       "test data",
			},
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		writer.Write(data)
		writer.Close()
	}()

	// The query should handle this internally without exposing it
	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	query.Close()
}

// TestQueryControlRequest tests control request handling with canCallTool
func TestQueryControlRequest(t *testing.T) {
	reader, writer := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	defer stdinReader.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track responses written to stdin
	var writtenResponse map[string]interface{}
	go func() {
		decoder := json.NewDecoder(stdinReader)
		var resp map[string]interface{}
		if err := decoder.Decode(&resp); err == nil {
			writtenResponse = resp
		}
	}()

	// canCallTool callback that approves the request
	canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts CallToolOptions) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved": true,
		}, nil
	}

	query := NewQuery(ctx, stdinWriter, reader, QueryOptions{
		CanCallTool: canCallTool,
	})

	// Simulate Claude sending a control request
	go func() {
		req := map[string]interface{}{
			"type":       "control_request",
			"request_id": "req-456",
			"request": map[string]interface{}{
				"subtype":   "can_use_tool",
				"tool_name": "bash",
				"input":     map[string]interface{}{"command": "ls"},
			},
		}
		data, _ := json.Marshal(req)
		data = append(data, '\n')
		writer.Write(data)
	}()

	// Wait for response
	time.Sleep(200 * time.Millisecond)

	// Check that a response was written
	assert.NotNil(t, writtenResponse)
	assert.Equal(t, "control_response", writtenResponse["type"])

	response := writtenResponse["response"].(map[string]interface{})
	assert.Equal(t, "success", response["subtype"])
	assert.Equal(t, "req-456", response["request_id"])

	query.Close()
}

// TestQueryControlRequestDenial tests control request with denial
func TestQueryControlRequestDenial(t *testing.T) {
	reader, writer := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	defer stdinReader.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var writtenResponse map[string]interface{}
	go func() {
		decoder := json.NewDecoder(stdinReader)
		var resp map[string]interface{}
		if err := decoder.Decode(&resp); err == nil {
			writtenResponse = resp
		}
	}()

	// canCallTool callback that denies the request
	canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts CallToolOptions) (map[string]interface{}, error) {
		return nil, assert.AnError
	}

	query := NewQuery(ctx, stdinWriter, reader, QueryOptions{
		CanCallTool: canCallTool,
	})

	// Simulate Claude sending a control request
	go func() {
		req := map[string]interface{}{
			"type":       "control_request",
			"request_id": "req-deny",
			"request": map[string]interface{}{
				"subtype":   "can_use_tool",
				"tool_name": "bash",
			},
		}
		data, _ := json.Marshal(req)
		data = append(data, '\n')
		writer.Write(data)
	}()

	time.Sleep(200 * time.Millisecond)

	assert.NotNil(t, writtenResponse)
	response := writtenResponse["response"].(map[string]interface{})
	assert.Equal(t, "error", response["subtype"])

	query.Close()
}

// TestQueryInterrupt tests sending an interrupt request
func TestQueryInterrupt(t *testing.T) {
	reader, writer := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	defer stdinReader.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track requests written to stdin
	var writtenRequest map[string]interface{}
	go func() {
		decoder := json.NewDecoder(stdinReader)
		var req map[string]interface{}
		if err := decoder.Decode(&req); err == nil {
			writtenRequest = req
		}
	}()

	query := NewQuery(ctx, stdinWriter, reader, QueryOptions{})

	// Send interrupt
	err := query.Interrupt()
	assert.NoError(t, err)

	// Check that the request was written
	time.Sleep(100 * time.Millisecond)
	assert.NotNil(t, writtenRequest)
	assert.Equal(t, "control_request", writtenRequest["type"])

	request := writtenRequest["request"].(map[string]interface{})
	assert.Equal(t, "interrupt", request["subtype"])

	query.Close()
}

// TestQuerySetError tests error handling
func TestQuerySetError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	query := NewQuery(ctx, nil, nil, QueryOptions{})

	testError := assert.AnError
	query.SetError(testError)

	select {
	case err := <-query.Errors():
		assert.Equal(t, testError, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected error in errors channel")
	}

	query.Close()
}

// TestStreamPromptBuilder tests the stream prompt builder
func TestStreamPromptBuilder(t *testing.T) {
	builder := NewStreamPromptBuilder()

	// Add messages
	err := builder.AddText("Hello, world!")
	assert.NoError(t, err)

	err = builder.AddUserMessage("List files")
	assert.NoError(t, err)

	// Check not closed yet
	assert.False(t, builder.IsClosed())

	// Get messages channel
	messages := builder.Messages()
	assert.Equal(t, 2, len(messages))

	// Close the builder
	closed := builder.Close()
	assert.True(t, builder.IsClosed())
	assert.NotNil(t, closed)

	// Adding after close should fail
	err = builder.AddText("Should fail")
	assert.Error(t, err)
}

// TestStreamToStdin tests the StreamToStdin function
func TestStreamToStdin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader, writer := io.Pipe()
	defer reader.Close()

	messages := make(chan map[string]interface{}, 3)

	// Send messages
	go func() {
		messages <- map[string]interface{}{"type": "test", "value": "one"}
		messages <- map[string]interface{}{"type": "test", "value": "two"}
		close(messages)
	}()

	// Stream to stdin
	go func() {
		err := StreamToStdin(ctx, writer, messages)
		assert.NoError(t, err)
	}()

	// Read and verify
	decoder := json.NewDecoder(reader)

	var msg1 map[string]interface{}
	err := decoder.Decode(&msg1)
	assert.NoError(t, err)
	assert.Equal(t, "one", msg1["value"])

	var msg2 map[string]interface{}
	err = decoder.Decode(&msg2)
	assert.NoError(t, err)
	assert.Equal(t, "two", msg2["value"])

	// Should get EOF
	err = decoder.Decode(&map[string]interface{}{})
	assert.Equal(t, io.EOF, err)
}

// TestQueryLauncherBuildQueryArgs tests argument building
func TestQueryLauncherBuildQueryArgs(t *testing.T) {
	launcher := NewQueryLauncher(Config{})

	tests := []struct {
		name     string
		config   QueryConfig
		contains []string
	}{
		{
			name: "String prompt",
			config: QueryConfig{
				Prompt: "test prompt",
				Options: &QueryOptionsConfig{},
			},
			contains: []string{"--print", "test prompt"},
		},
		{
			name: "Stream prompt with canCallTool",
			config: QueryConfig{
				Prompt: StreamPrompt(nil),
				Options: &QueryOptionsConfig{
					CanCallTool: func(ctx context.Context, toolName string, input map[string]interface{}, opts CallToolOptions) (map[string]interface{}, error) {
						return nil, nil
					},
				},
			},
			contains: []string{"--input-format", "stream-json", "--permission-prompt-tool", "stdio"},
		},
		{
			name: "With model",
			config: QueryConfig{
				Prompt: "test",
				Options: &QueryOptionsConfig{
					Model: "claude-sonnet-4-6",
				},
			},
			contains: []string{"--model", "claude-sonnet-4-6"},
		},
		{
			name: "With fallback model",
			config: QueryConfig{
				Prompt: "test",
				Options: &QueryOptionsConfig{
					Model:         "claude-sonnet-4-6",
					FallbackModel: "claude-haiku-4-5",
				},
			},
			contains: []string{"--model", "claude-sonnet-4-6", "--fallback-model", "claude-haiku-4-5"},
		},
		{
			name: "Continue conversation",
			config: QueryConfig{
				Prompt: "test",
				Options: &QueryOptionsConfig{
					ContinueConversation: true,
				},
			},
			contains: []string{"--continue"},
		},
		{
			name: "With resume",
			config: QueryConfig{
				Prompt: "test",
				Options: &QueryOptionsConfig{
					Resume: "session-123",
				},
			},
			contains: []string{"--resume", "session-123"},
		},
		{
			name: "With custom system prompt",
			config: QueryConfig{
				Prompt: "test",
				Options: &QueryOptionsConfig{
					CustomSystemPrompt: "You are helpful",
				},
			},
			contains: []string{"--system-prompt", "You are helpful"},
		},
		{
			name: "With allowed tools",
			config: QueryConfig{
				Prompt: "test",
				Options: &QueryOptionsConfig{
					AllowedTools: []string{"bash", "editor"},
				},
			},
			contains: []string{"--allowedTools", "bash,editor"},
		},
		{
			name: "With max turns",
			config: QueryConfig{
				Prompt: "test",
				Options: &QueryOptionsConfig{
					MaxTurns: 5,
				},
			},
			contains: []string{"--max-turns", "5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := launcher.buildQueryArgs(tt.config)
			argsStr := strings.Join(args, " ")

			for _, substr := range tt.contains {
				assert.Contains(t, argsStr, substr)
			}
		})
	}
}

// TestQueryConfigValidation tests config validation
func TestQueryConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      QueryOptionsConfig
		prompt      QueryPrompt
		expectError bool
	}{
		{
			name: "Valid config",
			config: QueryOptionsConfig{
				Model: "claude-sonnet-4-6",
			},
			prompt:      "test",
			expectError: false,
		},
		{
			name: "Same fallback model",
			config: QueryOptionsConfig{
				Model:         "claude-sonnet-4-6",
				FallbackModel: "claude-sonnet-4-6",
			},
			prompt:      "test",
			expectError: true,
		},
		{
			name: "Stream prompt without canCallTool",
			config: QueryOptionsConfig{
				CanCallTool: nil,
			},
			prompt:      StreamPrompt(nil),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qc := QueryConfig{
				Prompt:  tt.prompt,
				Options: &tt.config,
			}
			err := ValidateQueryConfig(qc)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestQueryOptionFunctionalTests tests the functional options
func TestQueryOptionFunctionalTests(t *testing.T) {
	config := &QueryOptionsConfig{}

	// Apply options
	WithModel("claude-sonnet-4-6")(config)
	WithFallbackModel("claude-haiku-4-5")(config)
	WithCustomSystemPrompt("Custom prompt")(config)
	WithCWD("/tmp")(config)
	WithResume("session-123")(config)
	WithContinue()(config)
	WithAllowedTools("bash", "editor")(config)

	assert.Equal(t, "claude-sonnet-4-6", config.Model)
	assert.Equal(t, "claude-haiku-4-5", config.FallbackModel)
	assert.Equal(t, "Custom prompt", config.CustomSystemPrompt)
	assert.Equal(t, "/tmp", config.CWD)
	assert.Equal(t, "session-123", config.Resume)
	assert.True(t, config.ContinueConversation)
	assert.Equal(t, []string{"bash", "editor"}, config.AllowedTools)
}

// TestQueryConcurrent tests concurrent message consumption
func TestQueryConcurrent(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	query := NewQuery(ctx, nil, reader, QueryOptions{})

	// Write multiple messages
	go func() {
		for i := 0; i < 10; i++ {
			msg := map[string]interface{}{
				"type":  "test",
				"index": i,
			}
			data, _ := json.Marshal(msg)
			data = append(data, '\n')
			writer.Write(data)
		}
		writer.Close()
	}()

	// Concurrently read messages
	var wg sync.WaitGroup
	messageCount := 0
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				msg, ok := query.Next()
				if !ok {
					return
				}
				mu.Lock()
				messageCount++
				mu.Unlock()
				_ = msg
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, 10, messageCount)

	query.Close()
}

// TestQueryDoneChannel tests the Done channel
func TestQueryDoneChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	query := NewQuery(ctx, nil, nil, QueryOptions{})

	// Done channel should not be closed yet
	select {
	case <-query.Done():
		t.Fatal("Done channel should not be closed")
	default:
	}

	// Cancel context
	cancel()

	// Done channel should be closed
	select {
	case <-query.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Done channel should be closed after cancel")
	}

	query.Close()
}
