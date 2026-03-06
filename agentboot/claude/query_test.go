package claude

import (
	"context"
	"encoding/json"
	"io"
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
