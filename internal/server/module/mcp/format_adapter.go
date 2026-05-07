package mcp

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

// ServerOps abstracts server operations needed by interceptors/processors
type ServerOps interface {
	TrackUsage(c *gin.Context, input, output, cache int)
	CallMCPTool(ctx context.Context, toolName, arguments string, messages []map[string]any) (string, error)
	GetRecorder() ProtocolRecorder
}

// ProtocolRecorder abstracts protocol recording operations
type ProtocolRecorder interface {
	RecordError(err error)
	SetAssembledResponse(resp any)
	RecordResponse(provider any, model string)
}

// EventType represents the type of streaming event
type EventType int

const (
	EventText EventType = iota
	EventToolStart
	EventToolDelta
	EventToolStop
	MessageDelta
	MessageStop
	EventUnknown
)

// Tool represents a tool call in a format-agnostic way
type Tool interface {
	ID() string
	Name() string
	Arguments() string
}

// ToolExecutionResult represents the result of executing a tool
type ToolExecutionResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

// TokenUsage represents token usage information
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	CacheTokens  int
}

// FormatAdapter abstracts differences between API formats (Anthropic/OpenAI)
type FormatAdapter interface {
	// Request/Response types
	NewRequest() any
	NewResponse() any

	// Tool extraction and classification
	ExtractTools(response any) ([]Tool, error)
	IsVirtualTool(tool Tool, registry *runtime.VirtualToolRegistry) bool
	SplitVirtualExternal(tools []Tool, registry *runtime.VirtualToolRegistry) (virtual, external []Tool, externalIDs []string)

	// Message building
	BuildAssistantMessage(response any) (any, error)
	BuildToolMessage(result ToolExecutionResult) any
	AppendToolResults(req any, resp any, results []any) (any, error)
	BuildContinuationSegment(resp any, results []ToolExecutionResult) (any, error)
	ApplyContinuation(req any, segment any) (any, error)
	FilterVirtualTools(response any, externalTools []Tool) (any, error)

	// Streaming setup
	SetupSSEHeaders(c *gin.Context)
	SendEvent(c *gin.Context, eventType string, payload []byte) error
	SendKeepAlive(c *gin.Context) error
	SendFinalMessage(c *gin.Context) error

	// Event processing (streaming only)
	ClassifyEvent(event any) EventType
	ExtractToolFromEvent(event any) (Tool, bool)
	// ShouldSuppressEvent is called for delta and stop events after ExtractToolFromEvent.
	//
	// Design note: suppress logic is format-dependent. Anthropic adapters suppress
	// virtual tool events at the start stage (bufferToolEvent in handleToolStartEvent),
	// so delta/stop events never reach the client and ShouldSuppressEvent returns false.
	// OpenAI adapters must suppress at delta stage because function names appear in
	// delta.ToolCalls, not at start; hence OpenAI's implementation checks the name here.
	ShouldSuppressEvent(event any, virtualRegistry *runtime.VirtualToolRegistry) bool
	RewriteEventIndex(event any, offset int) ([]byte, error)

	// Usage extraction
	ExtractUsage(response any) (TokenUsage, error)
}

// StreamHandle represents a generic streaming response
type StreamHandle interface {
	Next() bool
	Current() any
	Err() error
	Close() error
}

// Forwarder handles forwarding requests to upstream providers
type Forwarder interface {
	ForwardStream(ctx context.Context, provider any, model string, req any) (StreamHandle, error)
	ForwardNonStream(ctx context.Context, provider any, model string, req any) (any, error)
}

// ToolExecutor handles execution of virtual tools
type ToolExecutor interface {
	ExecuteToolWithContext(ctx context.Context, tool Tool, messages []map[string]any) (context.Context, ToolExecutionResult, error)
	ExecuteTool(ctx context.Context, tool Tool, messages []map[string]any) (ToolExecutionResult, error)
	ExecuteTools(ctx context.Context, tools []Tool, messages []map[string]any) ([]ToolExecutionResult, error)
}

// InterceptorConfig configures the generic interceptors
type InterceptorConfig struct {
	MaxRounds        int
	EnableGuardrails bool
	DisableUsage     bool
	ResponseModel    string
	// OnBeforeRound is called at the start of each round after round 0.
	// Use this to reset per-round state that lives outside the interceptor
	// (e.g. guardrails stream accumulator). The interceptor does not know
	// what this callback does — it only calls it.
	OnBeforeRound func(round int) error
}

// ResponseDecision represents the decision after classifying a response
type ResponseDecision int

const (
	DecisionNoTools ResponseDecision = iota
	DecisionPureVirtual
	DecisionPureExternal
	DecisionMixed
)
