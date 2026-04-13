package webtools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// MCPServer MCP 工具服务器
// 参考: libs/go-genai/examples/mcptoolbox/mcp_toolbox.go
type MCPServer struct {
	tools    []Tool
	httpAddr string
	server   *http.Server
}

// NewMCPServer 创建 MCP 服务器
func NewMCPServer(tools []Tool, port int) *MCPServer {
	return &MCPServer{
		tools:    tools,
		httpAddr: fmt.Sprintf(":%d", port),
	}
}

// Toolset MCP 工具集名称
const ToolsetName = "webtools"

// ServeHTTP 处理 MCP 请求
func (m *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/tools":
		m.listTools(w, r)
	case "/execute":
		m.executeTool(w, r)
	case "/health":
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	default:
		http.NotFound(w, r)
	}
}

// listTools 列出所有工具
func (m *MCPServer) listTools(w http.ResponseWriter, r *http.Request) {
	tools := make([]map[string]interface{}, len(m.tools))
	for i, tool := range m.tools {
		params := make(map[string]map[string]interface{})
		for k, v := range tool.Parameters() {
			params[k] = map[string]interface{}{
				"type":        v.Type,
				"description": v.Description,
				"required":    v.Required,
			}
			if v.Default != nil {
				params[k]["default"] = v.Default
			}
		}

		tools[i] = map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters": map[string]interface{}{
				"type":       "object",
				"properties": params,
			},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"toolset":  ToolsetName,
		"tools":    tools,
		"version":  "1.0.0",
	})
}

// executeTool 执行工具
func (m *MCPServer) executeTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Tool   string                 `json:"tool"`
		Params map[string]interface{} `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 查找工具
	var tool Tool
	for _, t := range m.tools {
		if t.Name() == req.Tool {
			tool = t
			break
		}
	}

	if tool == nil {
		http.Error(w, fmt.Sprintf("tool not found: %s", req.Tool), http.StatusNotFound)
		return
	}

	// 执行工具
	result, err := tool.Execute(context.Background(), req.Params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"result":  result,
	})
}

// Start 启动 MCP 服务器
func (m *MCPServer) Start() error {
	m.server = &http.Server{
		Addr: m.httpAddr,
		Handler: m,
	}
	return m.server.ListenAndServe()
}

// Stop 停止 MCP 服务器
func (m *MCPServer) Stop(ctx context.Context) error {
	return m.server.Shutdown(ctx)
}
