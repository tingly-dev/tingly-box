package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// MCPClientProbeRequest traverses the production MCP HTTP endpoint. An empty
// tool name checks initialize + tools/list only; a named tool also performs call.
type MCPClientProbeRequest struct {
	ToolName  string         `json:"tool_name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}
type MCPClientProbeResponse struct {
	Success       bool                 `json:"success"`
	Endpoint      string               `json:"endpoint"`
	Tools         []string             `json:"tools"`
	Result        *coretool.ToolResult `json:"result,omitempty"`
	ExecutionTime int64                `json:"execution_time"`
	Error         string               `json:"error,omitempty"`
}
type probeAuthTransport struct {
	base  http.RoundTripper
	token string
}

func (t probeAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}
func (h *Handler) ProbeClient(c *gin.Context) {
	if h.cfg == nil || h.runtime == nil {
		c.JSON(503, MCPClientProbeResponse{Error: "MCP runtime unavailable"})
		return
	}
	if !h.IsMCPEnabled() {
		c.JSON(403, MCPClientProbeResponse{Error: "MCP execution is disabled"})
		return
	}
	id := c.Param("profile_id")
	if !h.runtime.ClientExists(id) {
		c.JSON(404, MCPClientProbeResponse{Error: "unknown MCP client profile"})
		return
	}
	var request MCPClientProbeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, MCPClientProbeResponse{Error: err.Error()})
		return
	}
	endpoint := "/api/v1/mcp/" + url.PathEscape(id)
	response := MCPClientProbeResponse{Endpoint: endpoint, Tools: []string{}}
	start := time.Now()
	timeout := 30 * time.Second
	if cfg := h.runtime.GetConfig(); cfg != nil && cfg.RequestTimeout > 0 {
		timeout = time.Duration(cfg.RequestTimeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "tingly-route-check", Version: "1"}, nil)
	// The destination is the configured local gateway, never a request-provided
	// Host or URL. The management API's user authentication is reused internally.
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:   fmt.Sprintf("http://127.0.0.1:%d%s", h.cfg.GetServerPort(), endpoint),
		HTTPClient: &http.Client{Transport: probeAuthTransport{base: http.DefaultTransport, token: h.cfg.GetUserToken()}},
	}, nil)
	if err == nil {
		defer session.Close()
		params := &sdk.ListToolsParams{}
		for {
			var listed *sdk.ListToolsResult
			listed, err = session.ListTools(ctx, params)
			if err != nil {
				break
			}
			for _, tool := range listed.Tools {
				response.Tools = append(response.Tools, tool.Name)
			}
			if listed.NextCursor == "" {
				break
			}
			params.Cursor = listed.NextCursor
		}
		if err == nil && request.ToolName != "" {
			allowed := false
			for _, name := range response.Tools {
				if name == request.ToolName {
					allowed = true
					break
				}
			}
			if !allowed {
				err = fmt.Errorf("tool is not available to this client")
			} else {
				var result *sdk.CallToolResult
				result, err = session.CallTool(ctx, &sdk.CallToolParams{Name: request.ToolName, Arguments: request.Arguments})
				if err == nil {
					raw, marshalErr := json.Marshal(result)
					err = marshalErr
					if err == nil {
						response.Result = &coretool.ToolResult{}
						err = json.Unmarshal(raw, response.Result)
					}
				}
			}
		}
	}
	response.Success = err == nil
	response.ExecutionTime = time.Since(start).Milliseconds()
	if err != nil {
		response.Error = err.Error()
	}
	c.JSON(200, response)
}
