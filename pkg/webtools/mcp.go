package webtools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// MCPServer is a single-use MCP tool server.
// It shuts down automatically after executing a tool.
type MCPServer struct {
	tools    []Tool
	httpAddr string
	server   *http.Server
	cancel   context.CancelFunc
}

// NewMCPServer creates a new MCP server instance.
func NewMCPServer(tools []Tool, port int) *MCPServer {
	return &MCPServer{
		tools:    tools,
		httpAddr: fmt.Sprintf(":%d", port),
	}
}

// ToolsetName is the name of the toolset.
const ToolsetName = "webtools"

// ServeHTTP handles MCP requests.
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

// listTools lists all available tools.
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

// executeTool executes a tool and shuts down the server.
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

	// Find tool
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

	// Execute tool
	result, err := tool.Execute(context.Background(), req.Params)

	// Shutdown after execution (single-use server)
	if m.cancel != nil {
		m.cancel()
	}

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

// Start starts the MCP server with the given context.
// The server runs until the context is cancelled or a tool is executed.
func (m *MCPServer) Start(ctx context.Context) error {
	ctx, m.cancel = context.WithCancel(ctx)
	m.server = &http.Server{
		Addr:    m.httpAddr,
		Handler: m,
	}
	go func() {
		<-ctx.Done()
		_ = m.server.Shutdown(context.Background())
	}()
	return m.server.ListenAndServe()
}

// Stop stops the MCP server.
func (m *MCPServer) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}
