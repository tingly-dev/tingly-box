package toolruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type mcpSource struct {
	id      string
	config  *typ.MCPToolSourceConfig
	mu      sync.Mutex
	session *mcp.ClientSession
	tools   map[string]*mcp.Tool
}

func newMCPSource(id string, config *typ.MCPToolSourceConfig) Source {
	return &mcpSource{
		id:     id,
		config: config,
		tools:  make(map[string]*mcp.Tool),
	}
}

func (s *mcpSource) ID() string { return s.id }

func (s *mcpSource) OwnsTool(name string) bool {
	_, sourceID, _, ok := parseMCPToolName(name)
	return ok && sourceID == s.id
}

func normalizeMCPToolName(sourceID, toolName string) string {
	return "mcp__" + sourceID + "__" + toolName
}

func parseMCPToolName(name string) (normalized string, sourceID string, rawName string, ok bool) {
	if !strings.HasPrefix(name, "mcp__") {
		return "", "", "", false
	}
	parts := strings.SplitN(name, "__", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return name, parts[1], parts[2], true
}

func (s *mcpSource) ensureSession(ctx context.Context) (*mcp.ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		return s.session, nil
	}
	if s.config == nil {
		return nil, fmt.Errorf("mcp source %s missing config", s.id)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "tingly-box", Version: "v1"}, nil)
	transport, err := s.newTransport(ctx)
	if err != nil {
		return nil, err
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	s.session = session
	return s.session, nil
}

func (s *mcpSource) newTransport(ctx context.Context) (mcp.Transport, error) {
	transport := s.config.Transport
	if transport == "" {
		transport = typ.MCPTransportStdio
		if s.config.Endpoint != "" {
			transport = typ.MCPTransportHTTP
		}
	}

	switch transport {
	case typ.MCPTransportStdio:
		if s.config.Command == "" {
			return nil, fmt.Errorf("mcp source %s missing command", s.id)
		}
		cmd := exec.CommandContext(ctx, s.config.Command, s.config.Args...)
		if s.config.Cwd != "" {
			cmd.Dir = s.config.Cwd
		}
		cmd.Env = os.Environ()
		for key, value := range s.config.Env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	case typ.MCPTransportHTTP:
		if s.config.Endpoint == "" {
			return nil, fmt.Errorf("mcp source %s missing endpoint", s.id)
		}
		return &mcp.StreamableClientTransport{
			Endpoint:   s.config.Endpoint,
			HTTPClient: newMCPHTTPClient(s.config.Headers),
		}, nil
	default:
		return nil, fmt.Errorf("mcp source %s unsupported transport: %s", s.id, transport)
	}
}

func newMCPHTTPClient(headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return http.DefaultClient
	}
	return &http.Client{
		Transport: mcpHeaderRoundTripper{
			base:    http.DefaultTransport,
			headers: headers,
		},
	}
}

type mcpHeaderRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (rt mcpHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	for key, value := range rt.headers {
		if cloned.Header.Get(key) == "" {
			cloned.Header.Set(key, value)
		}
	}
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}

func (s *mcpSource) ListTools(ctx context.Context) ([]ToolDescriptor, error) {
	session, err := s.ensureSession(ctx)
	if err != nil {
		return nil, err
	}
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = make(map[string]*mcp.Tool, len(result.Tools))
	descriptors := make([]ToolDescriptor, 0, len(result.Tools))
	for _, tool := range result.Tools {
		if tool == nil {
			continue
		}
		s.tools[tool.Name] = tool
		parameters := map[string]interface{}{}
		if tool.InputSchema != nil {
			bytes, err := json.Marshal(tool.InputSchema)
			if err == nil {
				_ = json.Unmarshal(bytes, &parameters)
			}
		}
		descriptors = append(descriptors, ToolDescriptor{
			Name:        normalizeMCPToolName(s.id, tool.Name),
			Description: tool.Description,
			Parameters:  parameters,
			SourceID:    s.id,
		})
	}
	return descriptors, nil
}

func (s *mcpSource) CallTool(ctx context.Context, name string, arguments string) ToolResult {
	_, sourceID, rawName, ok := parseMCPToolName(name)
	if !ok || sourceID != s.id {
		return ToolResult{IsError: true, Error: fmt.Sprintf("unknown mcp tool: %s", name)}
	}
	session, err := s.ensureSession(ctx)
	if err != nil {
		return ToolResult{IsError: true, Error: fmt.Sprintf("mcp connect failed: %v", err)}
	}

	args := map[string]interface{}{}
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return ToolResult{IsError: true, Error: fmt.Sprintf("invalid mcp tool arguments: %v", err)}
		}
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: rawName, Arguments: args})
	if err != nil {
		return ToolResult{IsError: true, Error: fmt.Sprintf("mcp call failed: %v", err)}
	}
	return ToolResult{
		Content: renderMCPResult(result),
		IsError: result.IsError,
		Error:   renderMCPError(result),
	}
}

func renderMCPResult(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	if result.StructuredContent != nil {
		if bytes, err := json.Marshal(result.StructuredContent); err == nil {
			return string(bytes)
		}
	}
	if len(result.Content) == 0 {
		return ""
	}
	parts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	if bytes, err := json.Marshal(result.Content); err == nil {
		return string(bytes)
	}
	return ""
}

func renderMCPError(result *mcp.CallToolResult) string {
	if result == nil || !result.IsError {
		return ""
	}
	if text := renderMCPResult(result); text != "" {
		return text
	}
	return "mcp tool returned error"
}

func (s *mcpSource) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		if err := s.session.Close(); err != nil {
			logrus.WithError(err).Debugf("failed closing mcp session %s", s.id)
		}
		s.session = nil
	}
}
