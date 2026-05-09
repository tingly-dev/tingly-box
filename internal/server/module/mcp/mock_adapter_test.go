package mcp

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// MockFormatAdapter is a mock implementation of FormatAdapter for testing
type MockFormatAdapter struct {
	// Mock data to return
	ToolsToReturn      []Tool
	EventsToReturn     []any
	UsageToReturn      TokenUsage
	RequestType        any
	ResponseType       any

	// Call tracking
	ExtractToolsCalled    bool
	BuildToolMessageCalled bool
	AppendToolResultsCalled bool
	FilterVirtualToolsCalled bool
	SplitVirtualExternalCalled bool

	// Simulated behavior
	ShouldErrorExtract bool
	SuppressedEvents   map[int]bool // event index -> should suppress
}

func NewMockFormatAdapter() *MockFormatAdapter {
	return &MockFormatAdapter{
		ToolsToReturn:    []Tool{},
		EventsToReturn:   []any{},
		UsageToReturn:    TokenUsage{},
		SuppressedEvents: make(map[int]bool),
	}
}

// Request/Response types
func (m *MockFormatAdapter) NewRequest() any {
	return m.RequestType
}

func (m *MockFormatAdapter) NewResponse() any {
	if m.ResponseType != nil {
		return m.ResponseType
	}
	return &MockResponse{}
}

// Tool extraction
func (m *MockFormatAdapter) ExtractTools(response any) ([]Tool, error) {
	m.ExtractToolsCalled = true
	if m.ShouldErrorExtract {
		return nil, fmt.Errorf("mock extract error")
	}
	return m.ToolsToReturn, nil
}

func (m *MockFormatAdapter) IsVirtualTool(tool Tool, registry *coretool.VirtualToolRegistry) bool {
	if registry == nil {
		return false
	}
	_, toolName, _ := runtime.ParseNormalizedToolName(tool.Name())
	_, exists := registry.Get(toolName)
	return exists
}

func (m *MockFormatAdapter) SplitVirtualExternal(
	tools []Tool,
	registry *coretool.VirtualToolRegistry,
) (virtual, external []Tool, externalIDs []string) {
	m.SplitVirtualExternalCalled = true
	virtual = make([]Tool, 0)
	external = make([]Tool, 0)
	externalIDs = make([]string, 0)

	for _, tool := range tools {
		if m.IsVirtualTool(tool, registry) {
			virtual = append(virtual, tool)
		} else {
			external = append(external, tool)
			if tool.ID() != "" {
				externalIDs = append(externalIDs, tool.ID())
			}
		}
	}
	return
}

// Message building
func (m *MockFormatAdapter) BuildAssistantMessage(response any) (any, error) {
	return response, nil
}

func (m *MockFormatAdapter) BuildToolMessage(result ToolExecutionResult) any {
	m.BuildToolMessageCalled = true
	return &MockToolMessage{
		ToolUseID: result.ToolUseID,
		Content:   result.TextContent(),
	}
}

func (m *MockFormatAdapter) AppendToolResults(req, resp any, results []any) (any, error) {
	m.AppendToolResultsCalled = true
	return req, nil
}

func (m *MockFormatAdapter) FilterVirtualTools(response any, externalTools []Tool) (any, error) {
	m.FilterVirtualToolsCalled = true
	return response, nil
}

// Streaming setup
func (m *MockFormatAdapter) SetupSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
}

func (m *MockFormatAdapter) SendEvent(c *gin.Context, eventType string, payload []byte) error {
	c.SSEvent("", payload)
	c.Writer.Flush()
	return nil
}

func (m *MockFormatAdapter) SendKeepAlive(c *gin.Context) error {
	fmt.Fprint(c.Writer, ": keep-alive\n\n")
	c.Writer.Flush()
	return nil
}

func (m *MockFormatAdapter) SendFinalMessage(c *gin.Context) error {
	c.SSEvent("", "[DONE]")
	c.Writer.Flush()
	return nil
}

// Event processing
func (m *MockFormatAdapter) ClassifyEvent(event any) EventType {
	return EventText
}

func (m *MockFormatAdapter) ExtractToolFromEvent(event any) (Tool, bool) {
	return nil, false
}

func (m *MockFormatAdapter) ShouldSuppressEvent(event any, virtualRegistry *coretool.VirtualToolRegistry) bool {
	return false
}

func (m *MockFormatAdapter) RewriteEventIndex(event any, offset int) ([]byte, error) {
	return nil, nil
}

	// Usage extraction
func (m *MockFormatAdapter) ExtractUsage(response any) (TokenUsage, error) {
		return m.UsageToReturn, nil
	}

	// Mock types
type MockResponse struct {
	Tools []MockTool
}

type MockTool struct {
	IDVal       string
	NameVal     string
	ArgumentsVal string
}

func (t *MockTool) ID() string {
	return t.IDVal
}

func (t *MockTool) Name() string {
	return t.NameVal
}

func (t *MockTool) Arguments() string {
	return t.ArgumentsVal
}

type MockToolMessage struct {
	ToolUseID string
	Content   string
}

// MockStreamHandle is a mock StreamHandle for testing
type MockStreamHandle struct {
	Events     []any
	CurrentIdx int
	Closed     bool
	ErrorToReturn error
}

func NewMockStreamHandle(events []any) *MockStreamHandle {
	return &MockStreamHandle{
		Events:     events,
		CurrentIdx: -1,
		Closed:     false,
	}
}

func (m *MockStreamHandle) Next() bool {
	if m.Closed {
		return false
	}
	m.CurrentIdx++
	return m.CurrentIdx < len(m.Events)
}

func (m *MockStreamHandle) Current() any {
	if m.CurrentIdx < 0 || m.CurrentIdx >= len(m.Events) {
		return nil
	}
	return m.Events[m.CurrentIdx]
}

func (m *MockStreamHandle) Err() error {
	return m.ErrorToReturn
}

func (m *MockStreamHandle) Close() error {
	m.Closed = true
	return nil
}

// MockServerOps is a mock ServerOps for testing
type MockServerOps struct {
	ToolResultsToReturn map[string]string
	ToolErrorsToReturn  map[string]error
	Recorder            *MockProtocolRecorder
	UsageTracked        bool
	InputTokens         int
	OutputTokens        int
	CacheTokens         int
}

func NewMockServerOps() *MockServerOps {
	return &MockServerOps{
		ToolResultsToReturn: make(map[string]string),
		ToolErrorsToReturn:  make(map[string]error),
		Recorder:            NewMockProtocolRecorder(),
	}
}

func (m *MockServerOps) TrackUsage(c *gin.Context, input, output, cache int) {
	m.UsageTracked = true
	m.InputTokens = input
	m.OutputTokens = output
	m.CacheTokens = cache
}

func (m *MockServerOps) CallMCPTool(ctx context.Context, toolName, arguments string, messages []map[string]any) (string, error) {
	if err, exists := m.ToolErrorsToReturn[toolName]; exists {
		return "", err
	}
	if result, exists := m.ToolResultsToReturn[toolName]; exists {
		return result, nil
	}
	return fmt.Sprintf("Mock result for %s", toolName), nil
}

func (m *MockServerOps) GetRecorder() ProtocolRecorder {
	// Return nil for tests - most tests don't need the recorder
	return nil
}

// MockProtocolRecorder is a mock recorder for testing
type MockProtocolRecorder struct {
	ResponseSet       bool
	ResponseRecorded  bool
	ErrorRecorded     bool
	LastResponse      any
	LastError         error
}

func NewMockProtocolRecorder() *MockProtocolRecorder {
	return &MockProtocolRecorder{}
}

func (m *MockProtocolRecorder) RecordError(err error) {
	m.ErrorRecorded = true
	m.LastError = err
}

func (m *MockProtocolRecorder) SetAssembledResponse(resp any) {
	m.ResponseSet = true
	m.LastResponse = resp
}

func (m *MockProtocolRecorder) RecordResponse(provider any, model string) {
	m.ResponseRecorded = true
}
