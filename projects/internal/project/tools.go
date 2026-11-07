package project

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Tool is one MCP tool: what the model sees, who is offered it, and what it does.
type Tool struct {
	Name        string
	Description string
	// Properties is the JSON Schema "properties" object of the input.
	Properties map[string]any
	Required   []string
	// CoordinatorOnly offers the tool to a project's main thread only. Every
	// other thread gets the tools without it.
	CoordinatorOnly bool
	// Strict refuses input properties the schema does not list.
	Strict bool
	// Output is the JSON Schema of the structured result, for a tool whose
	// handler returns one (StructuredContent).
	Output map[string]any
	// EditsProject is set on a coordinator tool that changes the project's
	// settings: the turn adopts the result as its own (see coordinatorToolTurn.run).
	EditsProject bool
	// Handle runs the tool.
	Handle func(ctx context.Context, call ToolCall) (*mcp.CallToolResult, error)
}

// ToolCall is what a handler gets. Identity comes from the capability URL the
// session called, never from the model's input.
type ToolCall struct {
	// ThreadID is the thread whose session made the call.
	ThreadID string
	// Project is the project as it is now. Set for the coordinator only.
	Project *db.Project
	// Input is the validated JSON arguments.
	Input json.RawMessage
}

func textResult(text string, isErr bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: isErr, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// strProp is the schema of a string property.
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
