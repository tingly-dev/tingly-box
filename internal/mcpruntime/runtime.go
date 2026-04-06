package mcpruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

const normalizedPrefix = "mcp__"

type configProvider func() *typ.MCPRuntimeConfig

// Runtime handles MCP tool source discovery and tool execution.
type Runtime struct {
	getConfig configProvider
}

// NewRuntime creates a new MCP runtime.
func NewRuntime(getConfig configProvider) *Runtime {
	return &Runtime{getConfig: getConfig}
}

type mcpTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema,omitempty"`
	InputSchema2 json.RawMessage `json:"input_schema,omitempty"`
}

func (t mcpTool) schema() json.RawMessage {
	if len(t.InputSchema) > 0 {
		return t.InputSchema
	}
	if len(t.InputSchema2) > 0 {
		return t.InputSchema2
	}
	return nil
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpListToolsResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpCallToolResult struct {
	Content interface{} `json:"content,omitempty"`
}

// ListOpenAITools returns all MCP tools in normalized OpenAI function-tool format.
func (r *Runtime) ListOpenAITools(ctx context.Context) []openai.ChatCompletionToolUnionParam {
	cfg := r.getConfigOrDefault()
	if cfg == nil || len(cfg.Sources) == 0 {
		logrus.Debugf("mcp: ListOpenAITools - no config or no sources (cfg=%v, sources=%d)", cfg != nil, len(cfg.Sources))
		return nil
	}
	logrus.Debugf("mcp: ListOpenAITools - %d sources", len(cfg.Sources))

	out := make([]openai.ChatCompletionToolUnionParam, 0, 8)
	for _, source := range cfg.Sources {
		transport := strings.TrimSpace(source.Transport)
		if transport == "" {
			transport = "stdio"
		}
		var (
			tools []mcpTool
			err   error
		)
		switch transport {
		case "http":
			tools, err = r.listHTTPSourceTools(ctx, cfg, source)
		case "stdio":
			tools, err = r.listStdioSourceTools(ctx, cfg, source)
		default:
			continue
		}
		if err != nil {
			logrus.WithError(err).Warnf("mcp: list tools failed for source=%s", source.ID)
			continue
		}
		allowAll, allowSet := buildAllowList(source.Tools)
		for _, t := range tools {
			if strings.TrimSpace(t.Name) == "" {
				continue
			}
			if !allowAll && !allowSet[t.Name] {
				continue
			}
			normalized := NormalizeToolName(source.ID, t.Name)
			params := shared.FunctionParameters{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
			if raw := t.schema(); len(raw) > 0 {
				var schema map[string]interface{}
				if err := json.Unmarshal(raw, &schema); err == nil && len(schema) > 0 {
					params = schema
				}
			}
			def := shared.FunctionDefinitionParam{
				Name:       normalized,
				Parameters: params,
			}
			if t.Description != "" {
				def.Description = param.NewOpt(t.Description)
			}
			out = append(out, openai.ChatCompletionFunctionTool(def))
		}
	}
	return out
}

// IsMCPToolName checks whether a tool name is a normalized MCP tool.
func IsMCPToolName(name string) bool {
	return strings.HasPrefix(name, normalizedPrefix) && strings.Count(name, "__") >= 2
}

// NormalizeToolName converts source/tool pair to normalized tool name.
func NormalizeToolName(sourceID, toolName string) string {
	return normalizedPrefix + sourceID + "__" + toolName
}

// ParseNormalizedToolName parses normalized name and returns sourceID/toolName.
func ParseNormalizedToolName(name string) (string, string, bool) {
	if !strings.HasPrefix(name, normalizedPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(name, normalizedPrefix)
	parts := strings.SplitN(rest, "__", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// CallTool executes a normalized MCP tool call and returns serialized result.
func (r *Runtime) CallTool(ctx context.Context, normalizedName string, arguments string) (string, error) {
	sourceID, toolName, ok := ParseNormalizedToolName(normalizedName)
	if !ok {
		return "", fmt.Errorf("invalid normalized MCP tool name: %s", normalizedName)
	}

	cfg := r.getConfigOrDefault()
	if cfg == nil {
		return "", fmt.Errorf("mcp runtime config is not set")
	}

	var source *typ.MCPSourceConfig
	for i := range cfg.Sources {
		if cfg.Sources[i].ID == sourceID {
			source = &cfg.Sources[i]
			break
		}
	}
	if source == nil {
		return "", fmt.Errorf("mcp source %s not found", sourceID)
	}

	transport := strings.TrimSpace(source.Transport)
	if transport == "" {
		transport = "stdio"
	}
	switch transport {
	case "http":
	case "stdio":
	default:
		return "", fmt.Errorf("mcp transport %s is not implemented", transport)
	}

	var argsMap map[string]interface{}
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), &argsMap); err != nil {
			return "", fmt.Errorf("invalid tool arguments: %w", err)
		}
	}
	if argsMap == nil {
		argsMap = map[string]interface{}{}
	}

	var (
		callResult *mcpCallToolResult
		err        error
	)
	if transport == "http" {
		callResult, err = r.callHTTPTool(ctx, cfg, *source, toolName, argsMap)
	} else {
		callResult, err = r.callStdioTool(ctx, cfg, *source, toolName, argsMap)
	}
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(callResult)
	if err != nil {
		return "", fmt.Errorf("marshal mcp tool result failed: %w", err)
	}
	return string(b), nil
}

func (r *Runtime) getConfigOrDefault() *typ.MCPRuntimeConfig {
	if r == nil || r.getConfig == nil {
		return nil
	}
	cfg := r.getConfig()
	if cfg == nil {
		return nil
	}
	typ.ApplyMCPRuntimeDefaults(cfg)
	return cfg
}

func (r *Runtime) listHTTPSourceTools(ctx context.Context, cfg *typ.MCPRuntimeConfig, source typ.MCPSourceConfig) ([]mcpTool, error) {
	if err := r.initializeHTTPSource(ctx, cfg, source); err != nil {
		return nil, err
	}

	raw, err := callJSONRPC(ctx, cfg, source, "tools/list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var result mcpListToolsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode tools/list result failed: %w", err)
	}
	return result.Tools, nil
}

func (r *Runtime) listStdioSourceTools(ctx context.Context, cfg *typ.MCPRuntimeConfig, source typ.MCPSourceConfig) ([]mcpTool, error) {
	cli, cleanup, err := newStdioRPCClient(ctx, cfg, source)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if err := cli.initialize(ctx); err != nil {
		return nil, err
	}
	raw, err := cli.call(ctx, "tools/list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var result mcpListToolsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode tools/list result failed: %w", err)
	}
	return result.Tools, nil
}

func (r *Runtime) callHTTPTool(ctx context.Context, cfg *typ.MCPRuntimeConfig, source typ.MCPSourceConfig, toolName string, arguments map[string]interface{}) (*mcpCallToolResult, error) {
	if err := r.initializeHTTPSource(ctx, cfg, source); err != nil {
		return nil, err
	}
	raw, err := callJSONRPC(ctx, cfg, source, "tools/call", map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	})
	if err != nil {
		return nil, err
	}
	var result mcpCallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode tools/call result failed: %w", err)
	}
	return &result, nil
}

func (r *Runtime) callStdioTool(ctx context.Context, cfg *typ.MCPRuntimeConfig, source typ.MCPSourceConfig, toolName string, arguments map[string]interface{}) (*mcpCallToolResult, error) {
	cli, cleanup, err := newStdioRPCClient(ctx, cfg, source)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if err := cli.initialize(ctx); err != nil {
		return nil, err
	}
	raw, err := cli.call(ctx, "tools/call", map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	})
	if err != nil {
		return nil, err
	}
	var result mcpCallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode tools/call result failed: %w", err)
	}
	return &result, nil
}

func (r *Runtime) initializeHTTPSource(ctx context.Context, cfg *typ.MCPRuntimeConfig, source typ.MCPSourceConfig) error {
	_, err := callJSONRPC(ctx, cfg, source, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "tingly-box",
			"version": "dev",
		},
	})
	return err
}

func callJSONRPC(ctx context.Context, cfg *typ.MCPRuntimeConfig, source typ.MCPSourceConfig, method string, params interface{}) (json.RawMessage, error) {
	if strings.TrimSpace(source.Endpoint) == "" {
		return nil, fmt.Errorf("mcp source %s has empty endpoint", source.ID)
	}

	timeout := time.Duration(cfg.RequestTimeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}

	// Use custom proxy if configured
	if strings.TrimSpace(source.ProxyURL) != "" {
		proxyURL, err := url.Parse(source.ProxyURL)
		if err == nil {
			httpClient.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		}
	}

	payload := rpcRequest{
		JSONRPC: "2.0",
		ID:      fmt.Sprintf("tbe-%d", time.Now().UnixNano()),
		Method:  method,
		Params:  params,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, source.Endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("build rpc request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// streamable-http MCP servers require accepting both JSON and SSE
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range source.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc call failed: %w", err)
	}
	defer resp.Body.Close()

	// Some MCP HTTP servers (streamable-http) return SSE format:
	//   event: message
	//   data: {"jsonrpc":"2.0","id":"1","result":{...}}
	// Try to parse as plain JSON first; if that fails, try SSE format.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rpc response failed: %w", err)
	}

	var out rpcResponse
	if err := json.Unmarshal(body, &out); err != nil {
		// Try SSE format: find "data: " prefix and extract JSON after it
		lines := strings.Split(string(body), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				jsonPart := strings.TrimPrefix(line, "data: ")
				if err := json.Unmarshal([]byte(jsonPart), &out); err == nil {
					break
				}
			}
		}
		if out.Result == nil && out.Error == nil {
			return nil, fmt.Errorf("decode rpc response failed (tried JSON and SSE): %w / body: %s", err, string(body)[:min(200, len(body))])
		}
	}
	if out.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

type stdioRPCClient struct {
	in  io.WriteCloser
	out *bufio.Reader
	seq int64
}

func newStdioRPCClient(ctx context.Context, cfg *typ.MCPRuntimeConfig, source typ.MCPSourceConfig) (*stdioRPCClient, func(), error) {
	if strings.TrimSpace(source.Command) == "" {
		return nil, nil, fmt.Errorf("mcp stdio source %s has empty command", source.ID)
	}
	cmd := exec.CommandContext(ctx, source.Command, source.Args...)
	// Helper: search for the MCP script in a list of directories.
	// For each dir, checks if any relative arg resolves to an existing file.
	// Returns the directory containing the script, or "" if not found.
	findScriptInDirs := func(dirs []string) string {
		for _, d := range dirs {
			resolved := d
			if strings.HasPrefix(d, "~/") {
				if home, err := os.UserHomeDir(); err == nil {
					resolved = filepath.Join(home, d[2:])
				}
			}
			for _, arg := range source.Args {
				if filepath.IsAbs(arg) {
					continue
				}
				scriptPath := filepath.Join(resolved, arg)
				if _, err := os.Stat(scriptPath); err == nil {
					return resolved
				}
			}
		}
		return ""
	}

	cwd := strings.TrimSpace(source.Cwd)
	if cwd == "" {
		// No cwd configured: search for the script in likely locations.
		// os.Executable() returns the go-run temp binary when running via `go run`,
		// so we skip it when it contains "/go-build".
		searchDirs := []string{"~/.tingly-box/mcp"}
		if execPath, err := os.Executable(); err == nil {
			if !strings.Contains(execPath, "/go-build") {
				execDir := filepath.Dir(execPath)
				// Also check the "bundle" layout: binary is one level above scripts/
				// e.g. /usr/local/bin/tingly-box + /usr/local/bin/../scripts/mcp_web_tools.py
				parent := filepath.Dir(execDir)
				searchDirs = append(searchDirs, execDir, parent)
			}
		}
		if found := findScriptInDirs(searchDirs); found != "" {
			cwd = found
			logrus.Debugf("mcp: found script in cwd=%s", cwd)
		} else {
			logrus.Debugf("mcp: no cwd configured, script not found in search dirs: %v", searchDirs)
		}
	} else {
		// User configured a cwd: expand ~ and validate script exists.
		// If not found, search as fallback.
		if strings.HasPrefix(cwd, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				cwd = filepath.Join(home, cwd[2:])
			}
		}
		if findScriptInDirs([]string{cwd}) == "" {
			logrus.Warnf("mcp: script not found in configured cwd %s, searching...", source.Cwd)
			searchDirs := []string{"~/.tingly-box/mcp"}
			if execPath, err := os.Executable(); err == nil && !strings.Contains(execPath, "/go-build") {
				execDir := filepath.Dir(execPath)
				parent := filepath.Dir(execDir)
				searchDirs = append(searchDirs, execDir, parent)
			}
			if found := findScriptInDirs(searchDirs); found != "" {
				cwd = found
				logrus.Debugf("mcp: found script in fallback cwd=%s", cwd)
			} else {
				cwd = ""
			}
		}
	}
	if cwd != "" {
		cmd.Dir = cwd
	}

	// Also expand ~ in args (e.g. python3 ~/.tingly-box/mcp/scripts/...)
	for i, arg := range cmd.Args[1:] {
		if strings.HasPrefix(arg, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				cmd.Args[i+1] = filepath.Join(home, arg[2:])
			}
		}
	}

	env := os.Environ()
	envVarPattern := regexp.MustCompile(`\$\{([^}]+)\}`)
	for k, v := range source.Env {
		if strings.TrimSpace(k) != "" {
			// Expand ${VAR} syntax to actual environment variable value
			expandedValue := envVarPattern.ReplaceAllStringFunc(v, func(match string) string {
				varName := match[2 : len(match)-1] // Extract VAR from ${VAR}
				return os.Getenv(varName)
			})
			env = append(env, k+"="+expandedValue)
		}
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("open stdio stdin failed: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("open stdio stdout failed: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start stdio command failed: %w", err)
	}

	client := &stdioRPCClient{
		in:  stdin,
		out: bufio.NewReader(stdout),
	}
	cleanup := func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	_ = cfg
	return client, cleanup, nil
}

func (c *stdioRPCClient) initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "tingly-box",
			"version": "dev",
		},
	})
	return err
}

func (c *stdioRPCClient) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	_ = ctx
	c.seq++
	id := fmt.Sprintf("tbe-stdio-%d", c.seq)
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal stdio rpc request failed: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(b))
	if _, err := io.WriteString(c.in, header); err != nil {
		return nil, fmt.Errorf("write stdio header failed: %w", err)
	}
	if _, err := c.in.Write(b); err != nil {
		return nil, fmt.Errorf("write stdio body failed: %w", err)
	}
	return c.readResponseForID(id)
}

func (c *stdioRPCClient) readResponseForID(id string) (json.RawMessage, error) {
	for i := 0; i < 32; i++ {
		payload, err := readStdioFrame(c.out)
		if err != nil {
			return nil, err
		}
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		rawID, hasID := msg["id"]
		if !hasID {
			continue
		}
		var gotID string
		if err := json.Unmarshal(rawID, &gotID); err != nil || gotID != id {
			continue
		}
		if rawErr, ok := msg["error"]; ok && len(rawErr) > 0 && string(rawErr) != "null" {
			var e rpcError
			if err := json.Unmarshal(rawErr, &e); err == nil {
				return nil, fmt.Errorf("rpc error %d: %s", e.Code, e.Message)
			}
			return nil, fmt.Errorf("rpc error: %s", string(rawErr))
		}
		if rawResult, ok := msg["result"]; ok {
			return rawResult, nil
		}
		return nil, fmt.Errorf("rpc response missing result")
	}
	return nil, fmt.Errorf("rpc response id %s not received", id)
}

func readStdioFrame(r *bufio.Reader) ([]byte, error) {
	contentLen := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read stdio header failed: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			v := strings.TrimSpace(line[len("content-length:"):])
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid content-length %q: %w", v, err)
			}
			contentLen = n
		}
	}
	if contentLen <= 0 {
		return nil, fmt.Errorf("missing content-length in stdio frame")
	}
	body := make([]byte, contentLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read stdio body failed: %w", err)
	}
	return body, nil
}

func buildAllowList(names []string) (bool, map[string]bool) {
	if len(names) == 0 {
		return true, nil
	}
	out := make(map[string]bool, len(names))
	for _, n := range names {
		if n == "*" {
			return true, nil
		}
		n = strings.TrimSpace(n)
		if n != "" {
			out[n] = true
		}
	}
	return false, out
}
