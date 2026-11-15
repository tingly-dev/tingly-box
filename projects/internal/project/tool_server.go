package project

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	toolhub "github.com/tingly-dev/tingly-box/mcphub/tool"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// tools is every tool a session of thread t, in project p, could be offered;
// buildServer keeps the ones that session may have.
func (f *sessionToolsMCP) tools(t *db.Thread, p *db.Project) []Tool {
	var out []Tool
	if c := f.service.Coordinator; c != nil {
		out = c.coordinatorTools()
	}
	if f.service.Links != nil {
		out = append(out, f.githubTools()...)
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
		if (tool.CoordinatorOnly && turn == nil) || (tool.ThreadOnly && turn != nil) {
			continue
		}
		if err := f.register(srv, tool, t.ID, turn); err != nil {
			return nil, nil, err
		}
	}
	return srv, turn, nil
}

// register adds tool to srv. A call is validated against its schema (the
// generic core in mcphub/tool), then run through the turn's gate when there is
// one (the coordinator).
func (f *sessionToolsMCP) register(srv *mcp.Server, tool Tool, threadID string, turn *coordinatorToolTurn) error {
	_, err := toolhub.Register(srv, toolhub.Definition{
		Name: tool.Name, Description: tool.Description,
		Properties: tool.Properties, Required: tool.Required, Strict: tool.Strict, Output: tool.Output,
		Handle: func(ctx context.Context, c toolhub.Call) (*mcp.CallToolResult, error) {
			call := ToolCall{ThreadID: threadID, Input: c.Input}
			if turn == nil {
				return tool.Handle(ctx, call)
			}
			return turn.run(f.service.Store, tool.Name, tool.EditsProject, func(current *db.Project) (*mcp.CallToolResult, error) {
				call.Project = current
				return finish(tool.Handle(ctx, call))
			})
		},
	})
	return err
}

// finish turns a handler's error into a failed tool result the model reads.
func finish(res *mcp.CallToolResult, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return textResult(err.Error(), true), nil
	}
	return res, nil
}
