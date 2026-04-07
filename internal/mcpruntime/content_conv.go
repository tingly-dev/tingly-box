package mcpruntime

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// contentBlocksToResult converts SDK []Content to json.RawMessage
// matching the existing mcpCallToolResult format: {"content": [...]}.
func contentBlocksToResult(content []mcp.Content) (json.RawMessage, error) {
	result := struct {
		Content interface{} `json:"content,omitempty"`
	}{
		Content: content,
	}
	return json.Marshal(result)
}

// contentBlocksToTools converts SDK []*mcp.Tool to []mcpTool
// matching the existing internal tool format.
func contentBlocksToTools(sdkTools []*mcp.Tool) []mcpTool {
	tools := make([]mcpTool, 0, len(sdkTools))
	for _, t := range sdkTools {
		if t == nil {
			continue
		}
		tool := mcpTool{
			Name:        t.Name,
			Description: t.Description,
		}
		// InputSchema from SDK is any (map[string]any or json.RawMessage).
		// Convert to json.RawMessage for compatibility.
		if t.InputSchema != nil {
			if raw, ok := t.InputSchema.(json.RawMessage); ok {
				tool.InputSchema = raw
			} else {
				// map[string]any → json.RawMessage
				if b, err := json.Marshal(t.InputSchema); err == nil {
					tool.InputSchema = b
				}
			}
		}
		// Also check for input_schema (older spec version).
		// SDK uses inputSchema, so this covers the common case.
		tools = append(tools, tool)
	}
	return tools
}
