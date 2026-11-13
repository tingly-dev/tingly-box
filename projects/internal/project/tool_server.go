package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// tools is every tool a session of thread t, in project p, could be offered;
// buildServer keeps the ones that session may have.
func (f *sessionToolsMCP) tools(t *db.Thread, p *db.Project) []Tool {
	var out []Tool
	if c := f.service.Coordinator; c != nil {
		out = c.coordinatorTools()
	}
	return append(out, Tool{Name: "send_file", Description: f.sendFileDescription(t, p), Strict: true,
		Properties: map[string]any{
			"path":    strProp("Path to the file to publish in this conversation"),
			"caption": strProp("Optional description shown with the file"),
		}, Required: []string{"path"},
		Output: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"message_id"},
			"properties": map[string]any{"message_id": map[string]any{"type": "string"}}},
		Handle: func(ctx context.Context, call ToolCall) (*mcp.CallToolResult, error) {
			var in struct{ Path, Caption string }
			if err := json.Unmarshal(call.Input, &in); err != nil {
				return nil, err
			}
			if in.Path == "" {
				return nil, errors.New("path is required")
			}
			e, err := f.service.PublishFile(ctx, call.ThreadID, in.Path, in.Caption)
			if err != nil {
				return nil, err
			}
			res := sendFileResult{MessageID: e.ID}
			b, _ := json.Marshal(res)
			out := textResult(string(b), false)
			out.StructuredContent = res
			return out, nil
		}})
}

// buildFor makes the MCP server for the session of thread threadID. A project's
// main thread gets the coordinator's tools and the turn its calls are gated on
// (nil for any other thread).
func (f *sessionToolsMCP) buildFor(threadID string) (*mcp.Server, *coordinatorToolTurn, error) {
	t, err := f.service.Store.GetThread(threadID)
	if err != nil {
		return nil, nil, err
	}
	p, err := f.service.Store.GetProject(t.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	var turn *coordinatorToolTurn
	if f.service.Coordinator != nil && p.MainThreadID == t.ID {
		turn = &coordinatorToolTurn{}
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "tingly-projects", Version: "1.0.0"}, nil)
	for _, tool := range f.tools(t, p) {
		if tool.CoordinatorOnly && turn == nil {
			continue
		}
		if err := f.register(srv, tool, t.ID, turn); err != nil {
			return nil, nil, err
		}
	}
	return srv, turn, nil
}

// inputSchema is the schema the model sees for tool, and its resolved form for
// validating a call.
func inputSchema(tool Tool) (map[string]any, *jsonschema.Resolved, error) {
	schema := map[string]any{"type": "object", "properties": tool.Properties}
	if len(tool.Required) != 0 {
		schema["required"] = tool.Required
	}
	if tool.Strict {
		schema["additionalProperties"] = false
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, nil, err
	}
	var parsed jsonschema.Schema
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, nil, err
	}
	resolved, err := parsed.Resolve(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("%s schema: %w", tool.Name, err)
	}
	return schema, resolved, nil
}

// register adds tool to srv. A call is validated against the schema, then run
// through the turn's gate when there is one (the coordinator).
func (f *sessionToolsMCP) register(srv *mcp.Server, tool Tool, threadID string, turn *coordinatorToolTurn) error {
	if tool.Handle == nil {
		return fmt.Errorf("tool %s has no handler", tool.Name)
	}
	schema, resolved, err := inputSchema(tool)
	if err != nil {
		return err
	}
	mt := &mcp.Tool{Name: tool.Name, Description: tool.Description, InputSchema: schema}
	if tool.Output != nil {
		mt.OutputSchema = tool.Output
	}
	srv.AddTool(mt, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		var input any
		if err := json.Unmarshal(args, &input); err != nil {
			return textResult("invalid input: "+err.Error(), true), nil
		}
		if err := resolved.Validate(input); err != nil {
			return textResult("invalid input: "+err.Error(), true), nil
		}
		call := ToolCall{ThreadID: threadID, Input: args}
		if turn == nil {
			return finish(tool.Handle(ctx, call))
		}
		return turn.run(f.service.Store, tool.Name, tool.EditsProject, func(current *db.Project) (*mcp.CallToolResult, error) {
			call.Project = current
			return finish(tool.Handle(ctx, call))
		})
	})
	return nil
}

// finish turns a handler's error into a failed tool result the model reads.
func finish(res *mcp.CallToolResult, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return textResult(err.Error(), true), nil
	}
	return res, nil
}
