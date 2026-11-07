package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// tools is every tool a session of thread threadID could be offered; buildServer
// keeps the ones its scope allows.
func (f *sessionToolsMCP) tools(threadID string) []Tool {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	var out []Tool
	if c := f.service.Coordinator; c != nil {
		out = append(out, c.coordinatorTools()...)
		for _, item := range []struct {
			name, description string
			list              bool
		}{
			{"read_shared_file", "Read a UTF-8 text file or image in this project's library or memory. Accepts the absolute path shown for an attachment, or a path beginning library/ or memory/.", false},
			{"list_shared_files", "List a directory in this project's library or memory. Use library or memory as the root path.", true},
		} {
			item := item
			out = append(out, Tool{Name: item.name, Description: item.description, Scopes: ScopeCoordinator, Risk: RiskRead,
				Properties: map[string]any{"path": str("Path in the project's library or memory")}, Required: []string{"path"},
				Handle: func(_ context.Context, call ToolCall) (*mcp.CallToolResult, error) {
					var in struct{ Path string }
					if err := json.Unmarshal(call.Input, &in); err != nil {
						return nil, err
					}
					content, err := c.sharedFile(call.Project.ID, in.Path, item.list)
					if err != nil {
						return nil, err
					}
					return &mcp.CallToolResult{Content: content}, nil
				}})
		}
	}
	out = append(out, Tool{Name: "send_file", Description: f.sendFileDescription(threadID), Scopes: ScopeAll, Risk: RiskWrite,
		Properties: map[string]any{
			"path":    str("Path to the file to publish in this conversation"),
			"caption": str("Optional description shown with the file"),
		}, Required: []string{"path"},
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
			b, _ := json.Marshal(sendFileResult{MessageID: e.ID})
			return textResult(string(b), false), nil
		}})
	return out
}

// buildServer makes the MCP server for the session of thread threadID: the
// given tools whose scopes include scope. A coordinator-scope server needs the
// turn its calls are gated on.
func (f *sessionToolsMCP) buildServer(threadID string, scope Scope, turn *coordinatorToolTurn, tools []Tool) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "tingly-projects", Version: "1.0.0"}, nil)
	for _, tool := range tools {
		if !tool.Scopes.has(scope) {
			continue
		}
		if err := f.register(srv, tool, threadID, scope, turn); err != nil {
			return nil, err
		}
	}
	return srv, nil
}

func (f *sessionToolsMCP) register(srv *mcp.Server, tool Tool, threadID string, scope Scope, turn *coordinatorToolTurn) error {
	if tool.Handle == nil {
		return fmt.Errorf("tool %s has no handler", tool.Name)
	}
	props, required := tool.Properties, tool.Required
	if tool.Risk == RiskConfirm {
		if tool.Scopes != ScopeCoordinator {
			return fmt.Errorf("tool %s needs the user's approval, so only the coordinator may have it", tool.Name)
		}
		props = map[string]any{}
		for k, v := range tool.Properties {
			props[k] = v
		}
		props[approvedBy] = map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"},
			"description": "Ids (the msg attribute of the user's event) of the user's own messages that approve this action. Without them it is refused."}
		required = append(append([]string(nil), required...), approvedBy)
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) != 0 {
		schema["required"] = required
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	var parsed jsonschema.Schema
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	resolved, err := parsed.Resolve(nil)
	if err != nil {
		return fmt.Errorf("%s schema: %w", tool.Name, err)
	}
	srv.AddTool(&mcp.Tool{Name: tool.Name, Description: tool.Description, InputSchema: schema}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		if scope != ScopeCoordinator {
			return finish(tool.Handle(ctx, call))
		}
		return turn.run(f.service.Store, tool.Name, tool.EditsProject, func(current *db.Project) (*mcp.CallToolResult, error) {
			call.Project = current
			var quote string
			if tool.Risk == RiskConfirm {
				var ids struct {
					ApprovedBy []string `json:"approved_by"`
				}
				_ = json.Unmarshal(args, &ids)
				if quote, err = f.service.Coordinator.quoteUser(current, ids.ApprovedBy); err != nil {
					return textResult(tool.Name+" needs the user's approval: "+err.Error(), true), nil
				}
			}
			res, err := finish(tool.Handle(ctx, call))
			if quote != "" && err == nil && !res.IsError {
				f.service.Coordinator.addEvent(current.ID, &db.ThreadEvent{Kind: db.EvSystem,
					Content: strings.TrimSpace(tool.Name + " ran on the user's word." + quote)})
			}
			return res, err
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
