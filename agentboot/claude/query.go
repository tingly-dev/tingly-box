package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/sirupsen/logrus"
)

const (
	// maxScannerLineSize is the maximum size of a line that can be read from stdout
	// Default bufio.Scanner buffer is 64KB which may not be enough for large JSON outputs
	maxScannerLineSize = 1024 * 1024 // 1MB
)

// SDKMessage represents a message from Claude SDK via stdout
type SDKMessage struct {
	Type      string                 `json:"type"`
	RequestID string                 `json:"request_id,omitempty"`
	Request   map[string]interface{} `json:"request,omitempty"`
	Response  map[string]interface{} `json:"response,omitempty"`
	// Message fields for regular messages
	SessionID string                 `json:"session_id,omitempty"`
	Message   map[string]interface{} `json:"message,omitempty"`
	SubType   string                 `json:"subtype,omitempty"`
	Result    string                 `json:"result,omitempty"`
	UUID      string                 `json:"uuid,omitempty"`
	Timestamp string                 `json:"timestamp,omitempty"`
	RawData   map[string]interface{} `json:"-"`
}

// Callback is called when Claude requests permission to use a tool
//
// Response format according to Claude CLI Agent Protocol:
// - Allow: {"behavior": "allow", "updatedInput": {...}}
// - Deny:  {"behavior": "deny", "message": "reason"}
//
// The updatedInput field is REQUIRED when allowing - it must contain the original
// or modified input that will be passed to the tool.
//
// Returning an error will also deny the tool with the error message as the reason.
type CanCallToolCallback func(ctx context.Context, toolName string, input map[string]interface{}, opts CallToolOptions) (map[string]interface{}, error)

// CallToolOptions contains options for tool calls
type CallToolOptions struct {
	Signal <-chan struct{} // Cancel signal for aborting the request
}

// Query manages the interaction with Claude process in stdio mode
// It implements an iterator-like pattern for consuming messages
type Query struct {
	ctx         context.Context
	cancel      context.CancelFunc
	childStdin  io.WriteCloser
	childStdout io.Reader
	processDone <-chan struct{}
	messages    chan SDKMessage
	errors      chan error
	canCallTool CanCallToolCallback

	pendingControlResponses map[string]chan map[string]interface{}
	cancelControllers       map[string]context.CancelFunc
	mu                      sync.RWMutex
	closed                  bool
	closeOnce               sync.Once
}

// QueryOptions contains options for creating a Query
type QueryOptions struct {
	CanCallTool CanCallToolCallback
	AbortSignal <-chan struct{}
	ProcessDone <-chan struct{}
}

// NewQuery creates a new Query instance for stdio-based communication
func NewQuery(
	ctx context.Context,
	childStdin io.WriteCloser,
	childStdout io.Reader,
	options QueryOptions,
) *Query {
	ctx, cancel := context.WithCancel(ctx)

	q := &Query{
		ctx:                     ctx,
		cancel:                  cancel,
		childStdin:              childStdin,
		childStdout:             childStdout,
		processDone:             options.ProcessDone,
		messages:                make(chan SDKMessage, 100),
		errors:                  make(chan error, 10),
		canCallTool:             options.CanCallTool,
		pendingControlResponses: make(map[string]chan map[string]interface{}),
		cancelControllers:       make(map[string]context.CancelFunc),
		closed:                  false,
	}

	// Start reading messages in background
	go q.readMessages()

	// Handle abort signal
	if options.AbortSignal != nil {
		go func() {
			select {
			case <-options.AbortSignal:
				q.SetError(fmt.Errorf("query aborted"))
				q.Close()
			case <-ctx.Done():
			}
		}()
	}

	return q
}

// Next returns the next message from the stream
// Returns (message, true) when a message is available
// Returns (nil, false) when the stream is complete
// Returns error via Errors() channel if an error occurs
func (q *Query) Next() (SDKMessage, bool) {
	select {
	case <-q.ctx.Done():
		return SDKMessage{}, false
	case msg, ok := <-q.messages:
		if !ok {
			return SDKMessage{}, false
		}
		return msg, true
	}
}

// Messages returns the read-only channel of messages
func (q *Query) Messages() <-chan SDKMessage {
	return q.messages
}

// Errors returns the read-only channel of errors
func (q *Query) Errors() <-chan error {
	return q.errors
}

// Done returns a channel that's closed when the query is complete
func (q *Query) Done() <-chan struct{} {
	return q.ctx.Done()
}

// Close closes the query and cleans up resources
func (q *Query) Close() error {
	q.closeOnce.Do(func() {
		q.closed = true
		q.cancel()

		// Close stdin
		if q.childStdin != nil {
			q.childStdin.Close()
		}

		// Close channels
		close(q.messages)
		close(q.errors)

		// Cleanup pending control responses
		q.mu.Lock()
		for id, ch := range q.pendingControlResponses {
			close(ch)
			delete(q.pendingControlResponses, id)
		}
		// Abort all cancel controllers
		for id, cancel := range q.cancelControllers {
			cancel()
			delete(q.cancelControllers, id)
		}
		q.mu.Unlock()
	})
	return nil
}

// SetError sets an error on the query stream
func (q *Query) SetError(err error) {
	if !q.closed {
		select {
		case q.errors <- err:
		case <-q.ctx.Done():
		}
	}
}

// Interrupt sends an interrupt request to Claude
func (q *Query) Interrupt() error {
	if q.childStdin == nil {
		return fmt.Errorf("interrupt requires stdin to be available")
	}

	request := map[string]interface{}{
		"subtype": "interrupt",
	}

	return q.request(request)
}

// request sends a control request to Claude and waits for response
func (q *Query) request(request map[string]interface{}) error {
	requestID := generateRequestID()

	sdkRequest := map[string]interface{}{
		"request_id": requestID,
		"type":       "control_request",
		"request":    request,
	}

	responseCh := make(chan map[string]interface{}, 1)

	q.mu.Lock()
	q.pendingControlResponses[requestID] = responseCh
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		delete(q.pendingControlResponses, requestID)
		q.mu.Unlock()
		close(responseCh)
	}()

	// Write request to stdin
	data, err := json.Marshal(sdkRequest)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	data = append(data, '\n')
	if _, err := q.childStdin.Write(data); err != nil {
		return fmt.Errorf("write to stdin: %w", err)
	}

	// Wait for response
	select {
	case response := <-responseCh:
		if subtype, _ := response["subtype"].(string); subtype == "error" {
			return fmt.Errorf("control request failed: %v", response["error"])
		}
		return nil
	case <-q.ctx.Done():
		return q.ctx.Err()
	}
}

// readMessages reads line-delimited JSON from stdout
// This is the main message loop that handles all message types
func (q *Query) readMessages() {
	defer q.Close()

	scanner := bufio.NewScanner(q.childStdout)
	// Increase buffer size to handle large JSON lines (e.g., tool outputs)
	buf := make([]byte, 0, maxScannerLineSize)
	scanner.Buffer(buf, maxScannerLineSize)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		// Parse the line as JSON
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			logrus.Debugf("Failed to parse JSON line: %s, error: %v", line, err)
			continue
		}

		msgType, _ := raw["type"].(string)

		// Handle different message types
		switch msgType {
		case "control_response":
			q.handleControlResponse(raw)
		case "control_request":
			q.handleControlRequest(raw)
		case "control_cancel_request":
			q.handleControlCancelRequest(raw)
		default:
			// Regular SDK message
			q.enqueueMessage(raw)
		}
	}

	// Check for scan errors
	if err := scanner.Err(); err != nil {
		q.SetError(fmt.Errorf("scanner error: %w", err))
	}

	// Wait for process to complete
	if q.processDone != nil {
		<-q.processDone
	}
}

// handleControlResponse handles a control response from Claude
func (q *Query) handleControlResponse(raw map[string]interface{}) {
	response, _ := raw["response"].(map[string]interface{})
	requestID, _ := response["request_id"].(string)

	q.mu.RLock()
	ch, ok := q.pendingControlResponses[requestID]
	q.mu.RUnlock()

	if ok {
		select {
		case ch <- response:
		case <-q.ctx.Done():
		}
	}
}

// handleControlRequest handles a control request from Claude (e.g., can_use_tool)
func (q *Query) handleControlRequest(raw map[string]interface{}) {
	requestID, _ := raw["request_id"].(string)
	request, _ := raw["request"].(map[string]interface{})
	subtype, _ := request["subtype"].(string)

	// Create cancel controller for this request
	ctx, cancel := context.WithCancel(q.ctx)
	q.mu.Lock()
	q.cancelControllers[requestID] = cancel
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		delete(q.cancelControllers, requestID)
		q.mu.Unlock()
	}()

	// Process the request based on subtype
	var response map[string]interface{}
	var err error

	switch subtype {
	case "can_use_tool":
		response, err = q.processCanUseTool(ctx, request, requestID)
	default:
		err = fmt.Errorf("unsupported control request subtype: %s", subtype)
	}

	// Send response back to Claude
	controlResponse := map[string]interface{}{
		"type": "control_response",
		"response": map[string]interface{}{
			"request_id": requestID,
		},
	}

	if err != nil {
		controlResponse["response"].(map[string]interface{})["subtype"] = "error"
		controlResponse["response"].(map[string]interface{})["error"] = err.Error()
	} else {
		controlResponse["response"].(map[string]interface{})["subtype"] = "success"
		if response != nil {
			controlResponse["response"].(map[string]interface{})["response"] = response
		}
	}

	// Write response to stdin
	data, err := json.Marshal(controlResponse)
	if err != nil {
		logrus.Errorf("Failed to marshal control response: %v", err)
		return
	}
	data = append(data, '\n')
	if _, err := q.childStdin.Write(data); err != nil {
		logrus.Errorf("Failed to write control response to stdin: %v", err)
		// If write fails, the communication is broken - close the query
		q.Close()
	}
}

// handleControlCancelRequest handles a cancel notification from Claude
func (q *Query) handleControlCancelRequest(raw map[string]interface{}) {
	requestID, _ := raw["request_id"].(string)

	q.mu.Lock()
	cancel, ok := q.cancelControllers[requestID]
	if ok {
		cancel()
		delete(q.cancelControllers, requestID)
	}
	q.mu.Unlock()
}

// processCanUseTool processes a can_use_tool control request
func (q *Query) processCanUseTool(ctx context.Context, request map[string]interface{}, requestID string) (map[string]interface{}, error) {
	if q.canCallTool == nil {
		return nil, fmt.Errorf("canCallTool callback not provided")
	}

	toolName, _ := request["tool_name"].(string)
	input, _ := request["input"].(map[string]interface{})

	// Create cancel signal channel
	signal := make(chan struct{})
	cancelled := false

	go func() {
		<-ctx.Done()
		cancelled = true
		close(signal)
	}()

	// Call the callback
	response, err := q.canCallTool(q.ctx, toolName, input, CallToolOptions{
		Signal: signal,
	})

	if cancelled {
		return nil, fmt.Errorf("request cancelled")
	}

	if err != nil {
		return nil, err
	}

	return response, nil
}

// enqueueMessage enqueues a regular SDK message
func (q *Query) enqueueMessage(raw map[string]interface{}) {
	msg := SDKMessage{
		RawData: raw,
	}

	// Populate common fields
	msg.Type, _ = raw["type"].(string)
	msg.SessionID, _ = raw["session_id"].(string)
	msg.SubType, _ = raw["subtype"].(string)
	msg.Result, _ = raw["result"].(string)
	msg.UUID, _ = raw["uuid"].(string)
	msg.Timestamp, _ = raw["timestamp"].(string)

	// Parse message field if present
	if message, ok := raw["message"].(map[string]interface{}); ok {
		msg.Message = message
	}

	// Parse request/response for control messages
	if request, ok := raw["request"].(map[string]interface{}); ok {
		msg.Request = request
	}
	if response, ok := raw["response"].(map[string]interface{}); ok {
		msg.Response = response
	}

	// Try to send to messages channel, block if full
	// This ensures we don't lose messages when the consumer is slow
	select {
	case q.messages <- msg:
		// Message enqueued successfully
	case <-q.ctx.Done():
		// Context cancelled, message will be dropped
	}
}

// IsClosed returns true if the query is closed
func (q *Query) IsClosed() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.closed
}

// QueryOutputFormat is the input/output format for Query mode
type QueryOutputFormat string

const (
	QueryOutputFormatStreamJSON QueryOutputFormat = "stream-json"
	QueryInputFormatStreamJSON  QueryInputFormat  = "stream-json"
)

// QueryInputFormat is the input format for prompts
type QueryInputFormat string

// QueryPrompt represents either a string prompt or a channel of messages
type QueryPrompt interface{}

// StringPrompt is a string prompt
type StringPrompt string

// StreamPrompt is a channel-based prompt for streaming
type StreamPrompt <-chan map[string]interface{}

// QueryConfig is the configuration for creating a Query
type QueryConfig struct {
	// Prompt can be either a string or a channel of messages
	Prompt QueryPrompt

	// Options for the query
	Options *QueryOptionsConfig
}

// QueryOptionsConfig contains options for Query execution
type QueryOptionsConfig struct {
	// Working directory
	CWD string

	// Path to Claude executable
	ClaudePath string

	// Model selection
	Model         string
	FallbackModel string

	// System prompts
	CustomSystemPrompt string
	AppendSystemPrompt string

	// Conversation control
	ContinueConversation bool
	Resume               string

	// Tool filtering
	AllowedTools    []string
	DisallowedTools []string

	// Permission mode
	PermissionMode string

	// MCP servers
	MCPServers      map[string]interface{}
	StrictMcpConfig bool

	// Settings path
	SettingsPath string

	// Maximum turns
	MaxTurns int

	// Callback for tool permission requests
	CanCallTool CanCallToolCallback

	// Abort signal
	AbortSignal <-chan struct{}

	// Custom environment
	CustomEnv []string
}

// Validate validates the query configuration
func (c *QueryOptionsConfig) Validate() error {
	if c.FallbackModel != "" && c.Model == c.FallbackModel {
		return fmt.Errorf("fallback model cannot be the same as the main model")
	}

	return nil
}

// ValidateQueryConfig validates the full QueryConfig including prompt
func ValidateQueryConfig(qc QueryConfig) error {
	if err := qc.Options.Validate(); err != nil {
		return err
	}

	// Validate prompt matches options
	// Check if prompt is a channel (stream prompt) - if so, canCallTool is required
	if qc.Prompt != nil {
		// Use type switch for proper channel type detection
		switch prompt := qc.Prompt.(type) {
		case string:
			// String prompt is valid
		case StreamPrompt, chan map[string]interface{}:
			// Stream prompt requires canCallTool callback
			if qc.Options.CanCallTool == nil {
				return fmt.Errorf("stream prompt requires canCallTool callback")
			}
		case nil:
			// nil prompt is valid
		default:
			return fmt.Errorf("unsupported prompt type: %T", prompt)
		}
	}

	return nil
}
