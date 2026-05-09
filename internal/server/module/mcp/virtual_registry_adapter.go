package mcp

import (
	"encoding/json"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// ListAsMCPTools converts virtual tools to mcp.Tool slice for wire protocol use.
func ListAsMCPTools(r *coretool.VirtualToolRegistry) []mcpgo.Tool {
	if r == nil {
		return nil
	}
	virtualTools := r.ListVirtualTools()
	out := make([]mcpgo.Tool, 0, len(virtualTools))
	for _, t := range virtualTools {
		out = append(out, mcpgo.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: inputSchemaToMCP(t.InputSchema),
		})
	}
	return out
}

func inputSchemaToMCP(schema any) mcpgo.ToolInputSchema {
	switch v := schema.(type) {
	case nil:
		return mcpgo.ToolInputSchema{}
	case mcpgo.ToolInputSchema:
		return v
	case mcpgo.ToolArgumentsSchema:
		return mcpgo.ToolInputSchema(v)
	case json.RawMessage:
		var out mcpgo.ToolInputSchema
		_ = json.Unmarshal(v, &out)
		return out
	case map[string]any:
		out := mcpgo.ToolInputSchema{}
		if typ, _ := v["type"].(string); typ != "" {
			out.Type = typ
		}
		if props, _ := v["properties"].(map[string]any); props != nil {
			out.Properties = props
		}
		if req, ok := v["required"].([]string); ok {
			out.Required = req
		} else if rawReq, ok := v["required"].([]any); ok {
			for _, item := range rawReq {
				if s, _ := item.(string); s != "" {
					out.Required = append(out.Required, s)
				}
			}
		}
		out.AdditionalProperties = v["additionalProperties"]
		if defs, _ := v["$defs"].(map[string]any); defs != nil {
			out.Defs = defs
		}
		return out
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return mcpgo.ToolInputSchema{}
		}
		var out mcpgo.ToolInputSchema
		_ = json.Unmarshal(b, &out)
		return out
	}
}
