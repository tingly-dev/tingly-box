package runtime

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// TransportType represents the type of transport for a tool source.
type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportHTTP  TransportType = "http"
	TransportSSE   TransportType = "sse"
)

// ConnectionState represents the current state of a tool source connection.
type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateError        ConnectionState = "error"
)

// ConnectionStatus represents the current status of a tool source connection.
type ConnectionStatus struct {
	State         ConnectionState `json:"state"`
	LastConnected time.Time       `json:"last_connected,omitempty"`
	LastError     error           `json:"last_error,omitempty"`
	RetryCount    int             `json:"retry_count,omitempty"`
}

// ToolDefinition represents a tool's metadata and schema.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// ToolSource is the unified interface for all MCP tool sources.
// All transport types (stdio, http, sse, builtin) implement this interface.
type ToolSource interface {
	// Connection management
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	IsConnected() bool
	IsConfigured() bool
	GetConnectionState() ConnectionState
	GetConnectionStatus() ConnectionStatus

	// Tool operations
	ListTools(ctx context.Context) ([]ToolDefinition, error)
	CallTool(ctx context.Context, toolName string, arguments string) (string, error)

	// Health monitoring (for persistent connections)
	HealthCheck(ctx context.Context) error
	EnableHealthCheck(ctx context.Context, interval time.Duration)
	DisableHealthCheck(ctx context.Context)

	// Metadata
	GetType() TransportType
	GetSourceID() string
	GetSourceConfig() interface{} // Returns source-specific config
}

// BaseToolSource provides common functionality for all tool source implementations.
type BaseToolSource struct {
	sourceID  string
	transport TransportType
	state     ConnectionState
	status    ConnectionStatus
	mu        sync.RWMutex
}

// NewBaseToolSource creates a new base tool source.
func NewBaseToolSource(sourceID string, transport TransportType) *BaseToolSource {
	return &BaseToolSource{
		sourceID:  sourceID,
		transport: transport,
		state:     StateDisconnected,
		status: ConnectionStatus{
			State: StateDisconnected,
		},
	}
}

// GetSourceID returns the source ID.
func (b *BaseToolSource) GetSourceID() string {
	return b.sourceID
}

// GetType returns the transport type.
func (b *BaseToolSource) GetType() TransportType {
	return b.transport
}

// GetConnectionState returns the current connection state.
func (b *BaseToolSource) GetConnectionState() ConnectionState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// GetConnectionStatus returns the current connection status.
func (b *BaseToolSource) GetConnectionStatus() ConnectionStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

// setState updates the connection state and status.
func (b *BaseToolSource) setState(state ConnectionState, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = state
	b.status.State = state
	if err != nil {
		b.status.LastError = err
	}
	if state == StateConnected {
		b.status.LastConnected = time.Now()
		b.status.RetryCount = 0
	}
}

// HealthCheck provides a default no-op health check for on-demand connections.
func (b *BaseToolSource) HealthCheck(ctx context.Context) error {
	// Default implementation: no-op for on-demand connections
	return nil
}

// EnableHealthCheck provides a default no-op implementation for on-demand connections.
func (b *BaseToolSource) EnableHealthCheck(ctx context.Context, interval time.Duration) {
	// Default implementation: no-op for on-demand connections
}

// DisableHealthCheck provides a default no-op implementation for on-demand connections.
func (b *BaseToolSource) DisableHealthCheck(ctx context.Context) {
	// Default implementation: no-op for on-demand connections
}

// UnsupportedTransportError indicates an unsupported transport type.
type UnsupportedTransportError struct {
	Transport string
}

func (e *UnsupportedTransportError) Error() string {
	return "unsupported transport: " + e.Transport
}

// ConnectionError represents a connection-related error.
type ConnectionError struct {
	Source string
	Reason string
	Err    error
}

func (e *ConnectionError) Error() string {
	if e.Err != nil {
		return e.Source + ": " + e.Reason + ": " + e.Err.Error()
	}
	return e.Source + ": " + e.Reason
}

// SourceNotFoundError indicates that a source was not found.
type SourceNotFoundError struct {
	ID string
}

func (e *SourceNotFoundError) Error() string {
	return "source not found: " + e.ID
}

// InvalidToolNameError indicates an invalid tool name.
type InvalidToolNameError struct {
	Name string
}

func (e *InvalidToolNameError) Error() string {
	return "invalid tool name: " + e.Name
}

// ToolExecutionError represents an error during tool execution.
type ToolExecutionError struct {
	ToolName string
	Message  string
}

func (e *ToolExecutionError) Error() string {
	return e.ToolName + ": " + e.Message
}
