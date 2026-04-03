package mcpruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestRuntimeHTTPListAndCall(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		method, _ := req["method"].(string)
		id, _ := req["id"].(string)

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
		}

		switch method {
		case "initialize":
			resp["result"] = map[string]interface{}{"ok": true}
		case "tools/list":
			resp["result"] = map[string]interface{}{
				"tools": []map[string]interface{}{
					{
						"name":        "web_search",
						"description": "search",
						"inputSchema": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}},
						},
					},
				},
			}
		case "tools/call":
			resp["result"] = map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "ok"},
				},
			}
		default:
			resp["error"] = map[string]interface{}{"code": -32601, "message": "method not found"}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	cfg := &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			{
				ID:        "search",
				Transport: "http",
				Endpoint:  srv.URL,
			},
		},
	}
	rt := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	ctx := context.Background()

	tools := rt.ListOpenAITools(ctx)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	fn := tools[0].GetFunction()
	if fn == nil || fn.Name != "mcp__search__web_search" {
		t.Fatalf("unexpected tool name: %#v", fn)
	}

	out, err := rt.CallTool(ctx, "mcp__search__web_search", `{"query":"hi"}`)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if out == "" {
		t.Fatalf("expected non-empty call result")
	}
}
