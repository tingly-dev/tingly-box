// Package tool is the generic core of an MCP tool server: building and
// validating a tool's input schema, registering it on a go-sdk server, and
// the shared result helpers. Host concerns (who is calling, permissions)
// stay with the embedder: a Definition's handler gets only the validated
// input, and captures whatever host context it needs at registration time.
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Definition is one MCP tool: what the model sees and what a call does.
type Definition struct {
	Name        string
	Description string
	// Properties is the JSON Schema "properties" object of the input.
	Properties map[string]any
	Required   []string
	// Strict refuses input properties the schema does not list.
	Strict bool
	// Output is the JSON Schema of the structured result, for a tool whose
	// handler returns one (StructuredContent).
	Output map[string]any
	// Handle runs the tool.
	Handle func(ctx context.Context, call Call) (*mcp.CallToolResult, error)
}

// Call is what a handler gets: the validated JSON arguments.
type Call struct {
	// Input is the validated JSON arguments.
	Input json.RawMessage
}

// Register adds the tool to srv. A call is validated against the schema, then
// handed to Handle; a handler error becomes a failed tool result the model
// reads. It returns the schema the model sees.
func Register(srv *mcp.Server, d Definition) (map[string]any, error) {
	if d.Handle == nil {
		return nil, fmt.Errorf("tool %s has no handler", d.Name)
	}
	schema, resolved, err := inputSchema(d.Name, d.Properties, d.Required, d.Strict)
	if err != nil {
		return nil, err
	}
	mt := &mcp.Tool{Name: d.Name, Description: d.Description, InputSchema: schema}
	if d.Output != nil {
		mt.OutputSchema = d.Output
	}
	srv.AddTool(mt, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		var input any
		if err := json.Unmarshal(args, &input); err != nil {
			return TextResult("invalid input: "+err.Error(), true), nil
		}
		if err := resolved.Validate(input); err != nil {
			return TextResult("invalid input: "+err.Error(), true), nil
		}
		return finish(d.Handle(ctx, Call{Input: args}))
	})
	return schema, nil
}

// inputSchema is the schema the model sees, and its resolved form for
// validating a call.
func inputSchema(name string, properties map[string]any, required []string, strict bool) (map[string]any, *jsonschema.Resolved, error) {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) != 0 {
		schema["required"] = required
	}
	if strict {
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
		return nil, nil, fmt.Errorf("%s schema: %w", name, err)
	}
	return schema, resolved, nil
}

// TextResult is a tool result that is plain text.
func TextResult(text string, isErr bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: isErr, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// StrProp is the schema of a string property.
func StrProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// Typed adapts a typed handler to a Call handler: it unmarshals the input and
// answers with text (an error text is not redacted here; wrap Handle if the
// output may carry credentials).
func Typed[T any](fn func(ctx context.Context, in T) (string, error)) func(context.Context, Call) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, call Call) (*mcp.CallToolResult, error) {
		var in T
		if err := json.Unmarshal(call.Input, &in); err != nil {
			return TextResult("invalid input: "+err.Error(), true), nil
		}
		out, err := fn(ctx, in)
		if err != nil {
			return TextResult(err.Error(), true), nil
		}
		return TextResult(out, false), nil
	}
}

// finish turns a handler's error into a failed tool result the model reads.
func finish(res *mcp.CallToolResult, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return TextResult(err.Error(), true), nil
	}
	return res, nil
}
