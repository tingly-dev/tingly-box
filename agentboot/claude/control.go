package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ControlRequest represents a control request sent to Claude
type ControlRequest struct {
	RequestID string                 `json:"request_id"`
	Type      string                 `json:"type"` // e.g., "permission", "cancel"
	Request   map[string]interface{} `json:"request"`
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
		return ControlResponse{}, fmt.Errorf("control request timeout after %v", m.requestTimeout)
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
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
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
