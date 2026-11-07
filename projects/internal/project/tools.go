package project

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Scope says which sessions a tool is offered to. Each session gets its own
// MCP server, built from the tools whose scopes include its own.
type Scope uint8

const (
	// ScopeCoordinator is a project's main thread: it acts on the project and
	// on its threads, always inside its own project.
	ScopeCoordinator Scope = 1 << iota
	// ScopeWorker is a thread in a project. Its tools act on itself only.
	ScopeWorker
	// ScopeFolder is a thread in a working folder (no coordinator). Same
	// reach as a worker.
	ScopeFolder

	// ScopeAll is every session.
	ScopeAll = ScopeCoordinator | ScopeWorker | ScopeFolder
)

func (s Scope) has(o Scope) bool { return s&o != 0 }

// scopeOf is the scope of thread t, which belongs to project p.
func scopeOf(p *db.Project, t *db.Thread) Scope {
	switch {
	case p.Type == db.ProjectFolder:
		return ScopeFolder
	case p.MainThreadID == t.ID:
		return ScopeCoordinator
	}
	return ScopeWorker
}

// Risk is how much a tool can cost if the model is wrong.
type Risk uint8

const (
	// RiskRead changes nothing.
	RiskRead Risk = iota
	// RiskWrite changes something reversible, and leaves a note where the user
	// can see it.
	RiskWrite
	// RiskConfirm is irreversible, spends money or widens what a session may
	// do. The call must carry approved_by: ids of the user's own messages,
	// checked like cite. Only the coordinator scope may have such a tool.
	RiskConfirm
)

// Tool is one MCP tool: what the model sees, who may call it, and what it does.
type Tool struct {
	Name        string
	Description string
	// Properties is the JSON Schema "properties" object of the input.
	Properties map[string]any
	Required   []string
	Scopes     Scope
	Risk       Risk
	// Strict refuses input properties the schema does not list.
	Strict bool
	// Output is the JSON Schema of the structured result, for a tool whose
	// handler returns one (StructuredContent).
	Output map[string]any
	// EditsProject is set on a coordinator tool that changes the project's
	// settings: the turn adopts the result as its own (see coordinatorToolTurn.run).
	EditsProject bool
	// Handle runs the tool. Every tool has one.
	Handle func(ctx context.Context, call ToolCall) (*mcp.CallToolResult, error)
}

// ToolCall is what a handler gets. Identity comes from the capability URL the
// session called, never from the model's input.
type ToolCall struct {
	// ThreadID is the thread whose session made the call.
	ThreadID string
	// Project is the project as it is now. Set for the coordinator scope only.
	Project *db.Project
	// Input is the validated JSON arguments.
	Input json.RawMessage
}

// approvedBy is the property a RiskConfirm tool gets added to its input.
const approvedBy = "approved_by"

func textResult(text string, isErr bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: isErr, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
