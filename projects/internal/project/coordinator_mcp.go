package project

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

// coordinatorMCP registers the same tools used by the existing coordinator.
// The caller supplies the settings and trigger for the active turn; they must
// not be reused for another turn. The project ID is never accepted from Claude.
type coordinatorToolTurn struct {
	mu            sync.Mutex
	snapshot      *db.Project
	userTriggered bool
	quiet         atomic.Bool
}

func (t *coordinatorToolTurn) begin(p *db.Project, userTriggered bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	copy := *p
	copy.Repos = append([]db.Repo(nil), p.Repos...)
	t.snapshot, t.userTriggered = &copy, userTriggered
	t.quiet.Store(false)
}

func (t *coordinatorToolTurn) end() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snapshot = nil
}

func (c *Coordinator) coordinatorMCP(snapshot *db.Project, userTriggered bool) (*mcp.Server, error) {
	turn := &coordinatorToolTurn{}
	turn.begin(snapshot, userTriggered)
	return c.coordinatorMCPForTurn(turn)
}

func (c *Coordinator) coordinatorMCPForTurn(turn *coordinatorToolTurn) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "tingly-projects", Version: "1.0.0"}, nil)
	for _, tool := range coordinatorTools() {
		schema := map[string]any{"type": "object", "properties": tool.Properties}
		if len(tool.Required) != 0 {
			schema["required"] = tool.Required
		}
		data, err := json.Marshal(schema)
		if err != nil {
			return nil, err
		}
		var parsed jsonschema.Schema
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		resolved, err := parsed.Resolve(nil)
		if err != nil {
			return nil, fmt.Errorf("%s schema: %w", tool.Name, err)
		}
		name := tool.Name
		srv.AddTool(&mcp.Tool{Name: name, Description: tool.Description, InputSchema: schema}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			fail := func(message string) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}, nil
			}
			args := req.Params.Arguments
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			var input any
			if err := json.Unmarshal(args, &input); err != nil {
				return fail("invalid input: " + err.Error())
			}
			if err := resolved.Validate(input); err != nil {
				return fail("invalid input: " + err.Error())
			}
			// Serialize tool effects with turn completion and no_update. Claude
			// may submit multiple MCP calls concurrently.
			turn.mu.Lock()
			defer turn.mu.Unlock()
			snapshot, userTriggered := turn.snapshot, turn.userTriggered
			if snapshot == nil {
				return fail("No coordinator turn is active.")
			}
			if turn.quiet.Load() {
				return fail("This coordinator turn has already ended with no_update.")
			}
			current, err := c.store.GetProject(snapshot.ID)
			if err != nil {
				return fail(err.Error())
			}
			if current.Paused || current.ArchivedAt != nil {
				return fail("project is paused or archived")
			}
			if len(configurationChanges(snapshot, current)) > 0 {
				return fail("Project settings changed while this response was running. Re-evaluate using the current settings.")
			}
			if name == toolNoUpdate && userTriggered {
				return fail("A message from the user needs a reply. Answer it, or start or message a thread.")
			}
			out, isErr, started := c.runTool(ctx, current, llm.ToolCall{Name: name, Input: args})
			if name == toolNoUpdate && !isErr {
				turn.quiet.Store(true)
			}
			if len(started) > 0 {
				c.addEvent(current.ID, &db.ThreadEvent{Kind: db.EvThreadStarted, Payload: jsonString(map[string]any{"thread_ids": started})})
			}
			return &mcp.CallToolResult{IsError: isErr, Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil
		})
	}
	for _, item := range []struct {
		name, description string
		list              bool
	}{
		{"read_shared_file", "Read a UTF-8 text file or image in this project's library or memory. Accepts the absolute path shown for an attachment, or a path beginning library/ or memory/.", false},
		{"list_shared_files", "List a directory in this project's library or memory. Use library or memory as the root path.", true},
	} {
		item := item
		type pathArgs struct {
			Path string `json:"path" jsonschema:"Path in the project's library or memory"`
		}
		mcp.AddTool(srv, &mcp.Tool{Name: item.name, Description: item.description}, func(ctx context.Context, _ *mcp.CallToolRequest, args pathArgs) (*mcp.CallToolResult, any, error) {
			turn.mu.Lock()
			defer turn.mu.Unlock()
			if turn.snapshot == nil || turn.quiet.Load() {
				return nil, nil, fmt.Errorf("no coordinator turn is active")
			}
			current, err := c.store.GetProject(turn.snapshot.ID)
			if err != nil {
				return nil, nil, err
			}
			if current.Paused || current.ArchivedAt != nil || len(configurationChanges(turn.snapshot, current)) > 0 {
				return nil, nil, fmt.Errorf("project settings changed or the project is paused")
			}
			content, err := c.sharedFile(current.ID, args.Path, item.list)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: content}, nil, nil
		})
	}
	return srv, nil
}
