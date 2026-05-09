package runtime

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

type VirtualToolHandler = coretool.VirtualToolHandler
type VirtualTool = coretool.VirtualTool
type ToolCall = coretool.ToolCall

// VirtualToolRegistry holds registered in-process tools and exposes MCP adapter views.
type VirtualToolRegistry struct {
	inner *coretool.VirtualToolRegistry
}

func NewVirtualToolRegistry() *VirtualToolRegistry {
	return &VirtualToolRegistry{inner: coretool.NewVirtualToolRegistry()}
}

func (r *VirtualToolRegistry) Register(tool VirtualTool) {
	if r == nil || r.inner == nil {
		return
	}
	r.inner.Register(tool)
}

func (r *VirtualToolRegistry) Get(name string) (VirtualTool, bool) {
	if r == nil || r.inner == nil {
		return VirtualTool{}, false
	}
	return r.inner.Get(name)
}

func (r *VirtualToolRegistry) List() []mcp.Tool {
	if r == nil || r.inner == nil {
		return nil
	}
	virtualTools := r.inner.ListVirtualTools()
	out := make([]mcp.Tool, 0, len(virtualTools))
	for _, t := range virtualTools {
		out = append(out, mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: inputSchemaToMCP(t.InputSchema),
		})
	}
	return out
}

// ListVirtualTools returns the full VirtualTool list with visibility information.
// This is used by Runtime to filter tools based on client exposure.
func (r *VirtualToolRegistry) ListVirtualTools() []VirtualTool {
	if r == nil || r.inner == nil {
		return nil
	}
	return r.inner.ListVirtualTools()
}

func inputSchemaToMCP(schema any) mcp.ToolInputSchema {
	switch v := schema.(type) {
	case nil:
		return mcp.ToolInputSchema{}
	case mcp.ToolInputSchema:
		return v
	case mcp.ToolArgumentsSchema:
		return mcp.ToolInputSchema(v)
	case json.RawMessage:
		var out mcp.ToolInputSchema
		_ = json.Unmarshal(v, &out)
		return out
	case map[string]any:
		out := mcp.ToolInputSchema{}
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
			return mcp.ToolInputSchema{}
		}
		var out mcp.ToolInputSchema
		_ = json.Unmarshal(b, &out)
		return out
	}
}
