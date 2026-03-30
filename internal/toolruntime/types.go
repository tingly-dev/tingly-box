package toolruntime

import "context"

// ToolDescriptor is the normalized runtime representation of a tool.
type ToolDescriptor struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	SourceID    string
}

// ToolResult is the normalized runtime result of a tool call.
type ToolResult struct {
	ToolCallID string
	Content    string
	Error      string
	IsError    bool
}

// CallRequest identifies the runtime tool call to execute.
type CallRequest struct {
	Name      string
	Arguments string
}

// Source is a tool source used by the runtime.
type Source interface {
	ID() string
	ListTools(ctx context.Context) ([]ToolDescriptor, error)
	CallTool(ctx context.Context, name string, arguments string) ToolResult
	OwnsTool(name string) bool
}
