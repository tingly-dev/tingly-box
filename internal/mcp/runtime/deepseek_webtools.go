package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/webtools/deepseek"
)

// SetDeepSeekWebtools is wired once during server initialization. Provider
// resolution itself is live, so no credential is stored in a source's Env.
func (r *Runtime) SetDeepSeekWebtools(service *deepseek.Service) {
	if r != nil {
		r.deepSeekWebtools = service
	}
}

func isWebtoolSource(id string) bool {
	return id == mcptools.BuiltinWebtoolsSourceID || id == mcptools.ServerWebtoolsSourceID
}

func (r *Runtime) webBackend(source typ.MCPSourceConfig, tool string) (string, error) {
	env := maps.Clone(source.Env)
	expandSourceEnvInPlace(env)
	return mcptools.WebBackend(env, tool, r.deepSeekService(env, tool).Available())
}

func (r *Runtime) deepSeekService(env map[string]string, tool string) *deepseek.Service {
	key := "WEB_SEARCH_PROVIDER_UUID"
	if tool == mcptools.BuiltinWebFetchToolName {
		key = "WEB_FETCH_PROVIDER_UUID"
	}
	return r.deepSeekWebtools.ForProvider(strings.TrimSpace(env[key]))
}

func (r *Runtime) callDeepSeekWebtool(ctx context.Context, sourceID, toolName, arguments string) (coretool.ToolResult, bool, error) {
	if !isWebtoolSource(sourceID) || (toolName != mcptools.BuiltinWebSearchToolName && toolName != mcptools.BuiltinWebFetchToolName) {
		return coretool.ToolResult{}, false, nil
	}
	cfg := r.GetConfig()
	if cfg == nil {
		return coretool.ToolResult{}, false, nil
	}
	for _, source := range cfg.Sources {
		if source.ID != sourceID {
			continue
		}
		// Keep source/tool opt-in and allow lists authoritative on both surfaces.
		if (sourceID == mcptools.ServerWebtoolsSourceID && !r.IsBuiltinServerToolAvailable(sourceID, toolName)) || (sourceID != mcptools.ServerWebtoolsSourceID && !r.IsClientToolAvailable(sourceID, toolName)) {
			return coretool.ToolResult{}, true, fmt.Errorf("web tool is disabled or not configured")
		}
		backend, err := r.webBackend(source, toolName)
		if err != nil {
			return coretool.ErrorToolResult(err.Error()), true, nil
		}
		if backend != "deepseek" {
			return coretool.ToolResult{}, false, nil
		}
		r.sourcesMu.RLock()
		closed := r.closed
		r.sourcesMu.RUnlock()
		if closed {
			return coretool.ToolResult{}, true, fmt.Errorf("MCP runtime is shutting down")
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return coretool.ErrorToolResult("invalid arguments JSON: " + err.Error()), true, nil
		}
		operation := "search"
		if toolName == mcptools.BuiltinWebFetchToolName {
			operation = "fetch"
		}
		env := maps.Clone(source.Env)
		expandSourceEnvInPlace(env)
		text, err := r.deepSeekService(env, toolName).Tool(ctx, operation, args)
		if err != nil {
			return coretool.ErrorToolResult(err.Error()), true, nil
		}
		return coretool.TextToolResult(text), true, nil
	}
	return coretool.ToolResult{}, false, nil
}

// DeepSeekServerService resolves the same enabled source and selected credential
// used by gateway-injected tools. Direct endpoints must not bypass these choices.
func (r *Runtime) DeepSeekServerService(toolName string) (*deepseek.Service, error) {
	if r == nil || !r.IsBuiltinServerToolAvailable(mcptools.ServerWebtoolsSourceID, toolName) {
		return nil, fmt.Errorf("Server Tool is disabled or not configured")
	}
	cfg := r.GetConfig()
	if cfg != nil {
		for _, source := range cfg.Sources {
			if source.ID != mcptools.ServerWebtoolsSourceID {
				continue
			}
			backend, err := r.webBackend(source, toolName)
			if err != nil || backend != "deepseek" {
				break
			}
			env := maps.Clone(source.Env)
			expandSourceEnvInPlace(env)
			service := r.deepSeekService(env, toolName)
			if service.Available() {
				return service, nil
			}
		}
	}
	return nil, fmt.Errorf("select an available DeepSeek credential for this Server Tool")
}
