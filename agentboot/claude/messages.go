package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// MessageType constants for Claude Code stream JSON
const (
	MessageTypeText           = "text"
	MessageTypeSystem         = "system"
	MessageTypeAssistant      = "assistant"
	MessageTypeUser           = "user"
	MessageTypeToolUse        = "tool_use"
	MessageTypeToolResult     = "tool_result"
	MessageTypeResult         = "result"
	MessageTypeStreamEvent    = "stream_event"
	MessageTypeControlRequest = "control_request"
)

// Message is the interface for all Claude message types
type Message interface {
	GetType() string
	GetTimestamp() time.Time
	GetRawData() map[string]interface{}
}

// SystemMessage represents system/init messages
type SystemMessage struct {
	Type      string    `json:"type"`
	SubType   string    `json:"subtype,omitempty"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
}

// GetType implements Message
func (m *SystemMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *SystemMessage) GetTimestamp() time.Time {
	return m.Timestamp
}

// GetRawData implements Message
func (m *SystemMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// AssistantMessage represents assistant messages with content blocks
type AssistantMessage struct {
	Type            string            `json:"type"`
	Message         anthropic.Message `json:"message"`
	ParentToolUseID *string           `json:"parent_tool_use_id,omitempty"`
	SessionID       string            `json:"session_id"`
	UUID            string            `json:"uuid"`
	Timestamp       time.Time         `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *AssistantMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *AssistantMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *AssistantMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// UsageInfo contains token usage statistics
type UsageInfo struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens"`
}

// ContentBlock types
type ContentBlock interface {
	GetContentType() string
}

// TextBlock represents text content
type TextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// GetContentType implements ContentBlock
func (b *TextBlock) GetContentType() string {
	return b.Type
}

// ToolUseBlock represents a tool use invocation
type ToolUseBlock struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// GetContentType implements ContentBlock
func (b *ToolUseBlock) GetContentType() string {
	return b.Type
}

// ThinkingBlock represents reasoning/thinking content
type ThinkingBlock struct {
	Type     string `json:"type"`
	Thinking string `json:"thinking"`
}

// GetContentType implements ContentBlock
func (b *ThinkingBlock) GetContentType() string {
	return b.Type
}

// ToolResultContentBlock represents tool result content (within message content array)
type ToolResultContentBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// GetContentType implements ContentBlock
func (b *ToolResultContentBlock) GetContentType() string {
	return b.Type
}

// UnmarshalContentBlock unmarshals a content block from JSON
func UnmarshalContentBlock(data []byte) (ContentBlock, error) {
	var typeDetect struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeDetect); err != nil {
		return nil, err
	}

	switch typeDetect.Type {
	case "text":
		var block TextBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case "tool_use":
		var block ToolUseBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case "thinking":
		var block ThinkingBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case "tool_result":
		var block ToolResultContentBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	default:
		// Return unknown block type
		var block map[string]interface{}
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &UnknownBlock{Data: block}, nil
	}
}

// UnknownBlock represents an unrecognized content block
type UnknownBlock struct {
	Data map[string]interface{}
}

// GetContentType implements ContentBlock
func (b *UnknownBlock) GetContentType() string {
	if t, ok := b.Data["type"].(string); ok {
		return t
	}
	return "unknown"
}

// UserMessage represents user messages
type UserMessage struct {
	Type            string    `json:"type"`
	Message         string    `json:"message"`
	ParentToolUseID *string   `json:"parent_tool_use_id,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	Timestamp       time.Time `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *UserMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *UserMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *UserMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// ToolUseMessage represents a standalone tool use message (from stream)
type ToolUseMessage struct {
	Type      string                 `json:"type"`
	Name      string                 `json:"name"`
	Input     map[string]interface{} `json:"input"`
	ToolUseID string                 `json:"tool_use_id"`
	SessionID string                 `json:"session_id,omitempty"`
	Timestamp time.Time              `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *ToolUseMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *ToolUseMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *ToolUseMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// ToolResultMessage represents a tool result message
type ToolResultMessage struct {
	Type      string         `json:"type"`
	Output    string         `json:"output,omitempty"`  // String output
	Content   []ContentBlock `json:"content,omitempty"` // Or structured content
	ToolUseID string         `json:"tool_use_id"`
	IsError   bool           `json:"is_error,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Timestamp time.Time      `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *ToolResultMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *ToolResultMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *ToolResultMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// ResultMessage represents the final result message
type ResultMessage struct {
	Type              string             `json:"type"`
	SubType           string             `json:"subtype,omitempty"`
	Result            string             `json:"result,omitempty"`
	TotalCostUSD      float64            `json:"total_cost_usd,omitempty"`
	IsError           bool               `json:"is_error,omitempty"`
	DurationMS        int64              `json:"duration_ms,omitempty"`
	DurationAPIMS     int64              `json:"duration_api_ms,omitempty"`
	NumTurns          int                `json:"num_turns,omitempty"`
	Usage             UsageInfo          `json:"usage,omitempty"`
	SessionID         string             `json:"session_id,omitempty"`
	PermissionDenials []PermissionDenial `json:"permission_denials,omitempty"`
	Timestamp         time.Time          `json:"timestamp,omitempty"`
}

// PermissionDenial represents a denied permission request
type PermissionDenial struct {
	RequestID string `json:"request_id"`
	Reason    string `json:"reason"`
}

// GetType implements Message
func (m *ResultMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *ResultMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *ResultMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// IsSuccess returns true if the result indicates success
func (m *ResultMessage) IsSuccess() bool {
	return m.SubType == "success" || !m.IsError
}

// StreamEventMessage represents real-time streaming delta events
type StreamEventMessage struct {
	Type      string      `json:"type"`
	Event     StreamEvent `json:"event"`
	SessionID string      `json:"session_id,omitempty"`
	Timestamp time.Time   `json:"timestamp,omitempty"`
}

// GetType implements Message
func (m *StreamEventMessage) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *StreamEventMessage) GetTimestamp() time.Time {
	if !m.Timestamp.IsZero() {
		return m.Timestamp
	}
	return time.Now()
}

// GetRawData implements Message
func (m *StreamEventMessage) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// StreamEvent represents a streaming event
type StreamEvent struct {
	Type  string      `json:"type"` // content_block_delta, content_block_start, etc.
	Index int         `json:"index,omitempty"`
	Delta interface{} `json:"delta,omitempty"` // TextDelta, InputJSONDelta, etc.
}

// TextDelta represents incremental text content
type TextDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// InputJSONDelta represents incremental tool input JSON
type InputJSONDelta struct {
	Type        string `json:"type"`
	PartialJSON string `json:"partial_json"`
}

// MessageDelta represents message-level updates
type MessageDelta struct {
	Type       string     `json:"type"`
	StopReason string     `json:"stop_reason,omitempty"`
	Usage      *UsageInfo `json:"usage,omitempty"`
}

// ControlRequest represents a control request sent to Claude
type ControlRequest struct {
	RequestID string                 `json:"request_id"`
	Type      string                 `json:"type"` // e.g., "permission", "cancel"
	Request   map[string]interface{} `json:"request"`
}

// GetType implements Message
func (m *ControlRequest) GetType() string {
	return m.Type
}

// GetTimestamp implements Message
func (m *ControlRequest) GetTimestamp() time.Time {
	return time.Now()
}

// GetRawData implements Message
func (m *ControlRequest) GetRawData() map[string]interface{} {
	data, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// ControlResponse represents a control response received from Claude
type ControlResponse struct {
	RequestID string                 `json:"request_id"`
	Type      string                 `json:"type"` // e.g., "permission_response", "cancel_response"
	Response  map[string]interface{} `json:"response"`
}

// ControlManager manages control request/response lifecycle
type ControlManager struct {
	mu                sync.RWMutex
	pendingResponses  map[string]chan ControlResponse
	cancelControllers map[string]context.CancelFunc
	requestTimeout    time.Duration
	closed            bool
}

// NewControlManager creates a new control manager
func NewControlManager() *ControlManager {
	return &ControlManager{
		pendingResponses:  make(map[string]chan ControlResponse),
		cancelControllers: make(map[string]context.CancelFunc),
		requestTimeout:    30 * time.Second,
		closed:            false,
	}
}

// SetRequestTimeout sets the timeout for control requests
func (m *ControlManager) SetRequestTimeout(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestTimeout = d
}

// SendRequest sends a control request and waits for a response
func (m *ControlManager) SendRequest(ctx context.Context, req ControlRequest, stdin io.Writer) (ControlResponse, error) {
	if req.RequestID == "" {
		req.RequestID = generateRequestID()
	}

	respChan := make(chan ControlResponse, 1)

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ControlResponse{}, fmt.Errorf("control manager is closed")
	}
	m.pendingResponses[req.RequestID] = respChan
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pendingResponses, req.RequestID)
		m.mu.Unlock()
	}()

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, m.requestTimeout)
	defer cancel()

	// Write request to stdin
	if err := m.writeRequest(stdin, req); err != nil {
		return ControlResponse{}, fmt.Errorf("write control request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-respChan:
		return resp, nil
	case <-timeoutCtx.Done():
		return ControlResponse{}, fmt.Errorf("control request timeout after %s", m.requestTimeout)
	case <-ctx.Done():
		return ControlResponse{}, ctx.Err()
	}
}

// SendRequestAsync sends a control request without waiting for response
func (m *ControlManager) SendRequestAsync(req ControlRequest, stdin io.Writer) error {
	if req.RequestID == "" {
		req.RequestID = generateRequestID()
	}

	return m.writeRequest(stdin, req)
}

// HandleResponse processes a control response from Claude
func (m *ControlManager) HandleResponse(resp ControlResponse) {
	m.mu.RLock()
	ch, ok := m.pendingResponses[resp.RequestID]
	m.mu.RUnlock()

	if ok {
		select {
		case ch <- resp:
		default:
			logrus.Warnf("Control response channel full for request %s", resp.RequestID)
		}
	}
}

// HandleCancel handles a cancel notification from Claude
func (m *ControlManager) HandleCancel(cancelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, ok := m.cancelControllers[cancelID]; ok {
		cancel()
		delete(m.cancelControllers, cancelID)
	}
}

// RegisterCancelController registers a cancel controller for an operation
func (m *ControlManager) RegisterCancelController(id string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelControllers[id] = cancel
}

// UnregisterCancelController removes a cancel controller
func (m *ControlManager) UnregisterCancelController(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.cancelControllers, id)
}

// HandleControlMessage processes a raw control message from the event stream
func (m *ControlManager) HandleControlMessage(data map[string]interface{}) error {
	msgType, ok := data["type"].(string)
	if !ok {
		return fmt.Errorf("control message missing type")
	}

	switch {
	case msgType == "control_response":
		return m.handleControlResponse(data)

	case msgType == "cancel_notification":
		return m.handleCancelNotification(data)

	default:
		return fmt.Errorf("unknown control message type: %s", msgType)
	}
}

// handleControlResponse processes a control_response message
func (m *ControlManager) handleControlResponse(data map[string]interface{}) error {
	requestID, ok := data["request_id"].(string)
	if !ok {
		return fmt.Errorf("control_response missing request_id")
	}

	response, _ := data["response"].(map[string]interface{})

	resp := ControlResponse{
		RequestID: requestID,
		Type:      "control_response",
		Response:  response,
	}

	m.HandleResponse(resp)
	return nil
}

// handleCancelNotification processes a cancel_notification message
func (m *ControlManager) handleCancelNotification(data map[string]interface{}) error {
	cancelID, ok := data["cancel_id"].(string)
	if !ok {
		return fmt.Errorf("cancel_notification missing cancel_id")
	}

	m.HandleCancel(cancelID)
	return nil
}

// writeRequest writes a control request to stdin
func (m *ControlManager) writeRequest(stdin io.Writer, req ControlRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal control request: %w", err)
	}

	// Write with newline for proper protocol
	if _, err := stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write to stdin: %w", err)
	}

	return nil
}

// Close closes the control manager and cleans up resources
func (m *ControlManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true

	// Close all pending response channels
	for id, ch := range m.pendingResponses {
		close(ch)
		delete(m.pendingResponses, id)
	}

	// Cancel all active controllers
	for id, cancel := range m.cancelControllers {
		cancel()
		delete(m.cancelControllers, id)
	}

	return nil
}

// IsClosed returns true if the manager is closed
func (m *ControlManager) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return uuid.New().String()
}

// PermissionRequestBuilder builds permission control requests
type PermissionRequestBuilder struct {
	requestID string
	toolName  string
	input     map[string]interface{}
}

// NewPermissionRequestBuilder creates a new permission request builder
func NewPermissionRequestBuilder() *PermissionRequestBuilder {
	return &PermissionRequestBuilder{
		requestID: generateRequestID(),
		input:     make(map[string]interface{}),
	}
}

// WithRequestID sets the request ID
func (b *PermissionRequestBuilder) WithRequestID(id string) *PermissionRequestBuilder {
	b.requestID = id
	return b
}

// WithTool sets the tool name and input
func (b *PermissionRequestBuilder) WithTool(name string, input map[string]interface{}) *PermissionRequestBuilder {
	b.toolName = name
	b.input = input
	return b
}

// Build creates the control request
func (b *PermissionRequestBuilder) Build() ControlRequest {
	return ControlRequest{
		RequestID: b.requestID,
		Type:      "permission",
		Request: map[string]interface{}{
			"tool_name": b.toolName,
			"input":     b.input,
		},
	}
}

// CancelRequestBuilder builds cancel control requests
type CancelRequestBuilder struct {
	requestID string
	cancelID  string
	reason    string
}

// NewCancelRequestBuilder creates a new cancel request builder
func NewCancelRequestBuilder() *CancelRequestBuilder {
	return &CancelRequestBuilder{
		requestID: generateRequestID(),
	}
}

// WithRequestID sets the request ID
func (b *CancelRequestBuilder) WithRequestID(id string) *CancelRequestBuilder {
	b.requestID = id
	return b
}

// WithCancel sets the cancel ID
func (b *CancelRequestBuilder) WithCancel(id string) *CancelRequestBuilder {
	b.cancelID = id
	return b
}

// WithReason sets the cancellation reason
func (b *CancelRequestBuilder) WithReason(reason string) *CancelRequestBuilder {
	b.reason = reason
	return b
}

// Build creates the control request
func (b *CancelRequestBuilder) Build() ControlRequest {
	request := map[string]interface{}{
		"cancel_id": b.cancelID,
	}

	if b.reason != "" {
		request["reason"] = b.reason
	}

	return ControlRequest{
		RequestID: b.requestID,
		Type:      "cancel",
		Request:   request,
	}
}
