package mcpruntime

import (
	"context"
	"encoding/json"
	"fmt"
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
	sc        *sessionCache
}

// NewRuntime creates a new MCP runtime.
func NewRuntime(getConfig configProvider) *Runtime {
	// FIXME: it is useful but ugly, guard it in future
	cfg := getConfig()
	if cfg == nil {
		return nil
	}
	return &Runtime{getConfig: getConfig, sc: newSessionCache()}
}

// Close releases all MCP sessions.
func (r *Runtime) Close() {
	if r != nil && r.sc != nil {
		r.sc.closeAll()
	}
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

// ListOpenAITools returns all MCP tools in normalized OpenAI function-tool format.
func (r *Runtime) ListOpenAITools(ctx context.Context) []openai.ChatCompletionToolUnionParam {
	cfg := r.getConfigOrDefault()
	if cfg == nil || len(cfg.Sources) == 0 {
		sourceCount := 0
		if cfg != nil {
			sourceCount = len(cfg.Sources)
		}
		logrus.Debugf("mcp: ListOpenAITools - no config or no sources (cfg=%v, sources=%d)", cfg != nil, sourceCount)
		return nil
	}
	logrus.Debugf("mcp: ListOpenAITools - %d sources", len(cfg.Sources))

	out := make([]openai.ChatCompletionToolUnionParam, 0, 8)
	for _, source := range cfg.Sources {
		if !typ.IsMCPSourceEnabled(source) {
			logrus.Debugf("mcp: source=%s is disabled; skip tool listing", source.ID)
			continue
		}
		transport := strings.TrimSpace(source.Transport)
		if transport == "" {
			transport = "stdio"
		}

		ss, _, err := r.sc.getOrCreate(ctx, source, time.Duration(cfg.RequestTimeout)*time.Second)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"source":    source.ID,
				"transport": transport,
				"error":     err.Error(),
				"err_type":  fmt.Sprintf("%T", err),
			}).Warn("mcp: connect failed")
			continue
		}

		tools, err := func() ([]mcpTool, error) {
			// Hold a read lock for the session; the SDK is safe for concurrent calls.
			ss.mu.RLock()
			defer ss.mu.RUnlock()
			return ss.listTools(ctx)
		}()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"source":    source.ID,
				"transport": transport,
				"error":     err.Error(),
				"err_type":  fmt.Sprintf("%T", err),
			}).Warn("mcp: list tools failed")
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
		return "", &sessionError{sourceID: sourceID, msg: "invalid normalized MCP tool name: " + normalizedName}
	}

	cfg := r.getConfigOrDefault()
	if cfg == nil {
		return "", &sessionError{sourceID: sourceID, msg: "mcp runtime config is not set"}
	}

	var source *typ.MCPSourceConfig
	for i := range cfg.Sources {
		if cfg.Sources[i].ID == sourceID {
			source = &cfg.Sources[i]
			break
		}
	}
	if source == nil {
		return "", &sessionError{sourceID: sourceID, msg: "mcp source " + sourceID + " not found"}
	}
	if !typ.IsMCPSourceEnabled(*source) {
		return "", &sessionError{sourceID: sourceID, msg: "mcp source " + sourceID + " is disabled"}
	}

	transport := strings.TrimSpace(source.Transport)
	if transport == "" {
		transport = "stdio"
	}
	switch transport {
	case "http", "stdio":
	default:
		return "", &sessionError{sourceID: sourceID, msg: "mcp transport " + transport + " is not implemented"}
	}

	var argsMap map[string]interface{}
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), &argsMap); err != nil {
			return "", &sessionError{sourceID: sourceID, msg: "invalid tool arguments: " + err.Error()}
		}
	}
	if argsMap == nil {
		argsMap = map[string]interface{}{}
	}

	ss, _, err := r.sc.getOrCreate(ctx, *source, time.Duration(cfg.RequestTimeout)*time.Second)
	if err != nil {
		return "", err
	}

	result, err := func() (string, error) {
		ss.mu.RLock()
		defer ss.mu.RUnlock()
		return ss.callTool(ctx, toolName, argsMap)
	}()
	if err != nil {
		return "", err
	}
	return result, nil
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
