package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type configProvider func() *typ.MCPRuntimeConfig

// Runtime handles MCP tool source discovery and tool execution.
type Runtime struct {
	getConfig         configProvider
	sc                *sessionCache
	toolSourceFactory *ToolSourceFactory
	activeSources     map[string]ToolSource // source ID -> ToolSource
	sourcesMu         sync.RWMutex
	virtualRegistry   *coretool.VirtualToolRegistry
	sessionStore      *SessionStore
	sweeper           *time.Ticker

	// Cache for enabled server tool names to avoid repeated full enumeration.
	enabledNamesCache   map[string]struct{}
	enabledNamesExpires time.Time
	enabledNamesMu      sync.RWMutex
}

// NewRuntime creates a new MCP runtime.
func NewRuntime(getConfig configProvider) *Runtime {
	// FIXME: it is useful but ugly, guard it in future
	cfg := getConfig()
	if cfg == nil {
		return nil
	}
	sc := newSessionCache()
	r := &Runtime{
		getConfig:         getConfig,
		sc:                sc,
		toolSourceFactory: NewToolSourceFactory(sc, nil),
		activeSources:     make(map[string]ToolSource),
		virtualRegistry:   coretool.NewVirtualToolRegistry(),
		sessionStore:      NewSessionStore(10 * time.Minute),
	}
	r.sweeper = r.sessionStore.StartSweeper(1 * time.Minute)
	return r
}

// SetClientPool injects the client pool into the runtime's tool source factory.
func (r *Runtime) SetClientPool(cp *client.ClientPool) {
	if r == nil {
		return
	}
	if r.toolSourceFactory != nil {
		r.toolSourceFactory.SetClientPool(cp)
	}
}

// Close releases all MCP sessions and tool source connections.
func (r *Runtime) Close() {
	if r == nil {
		return
	}

	// Close all active tool sources
	if r.activeSources != nil {
		if r.sweeper != nil {
			r.sweeper.Stop()
		}
		r.sourcesMu.Lock()
		defer r.sourcesMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for sourceID, source := range r.activeSources {
			if err := source.Disconnect(ctx); err != nil {
				logrus.WithField("source", sourceID).WithError(err).
					Warn("mcp: failed to disconnect source during close")
			}
		}
		r.activeSources = make(map[string]ToolSource)
	}

	// Close all sessions
	if r.sc != nil {
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

// ListServerToolsForInjection returns MCP tools that should be injected into upstream model requests.
// Injection is intentionally restricted to server-side virtual tools.
// Client-facing non-virtual tools must be exposed through MCP transport endpoints instead.
func (r *Runtime) ListServerToolsForInjection(ctx context.Context) []openai.ChatCompletionToolUnionParam {
	if r == nil {
		return nil
	}
	out := make([]openai.ChatCompletionToolUnionParam, 0, 8)
	if r.virtualRegistry != nil {
		virtualTools := r.virtualRegistry.ListVirtualTools()
		for _, vt := range virtualTools {
			if !r.isVirtualServerToolInjectable(vt) {
				continue
			}
			// Virtual tools use "builtin" as source ID for normalization
			normalized := NormalizeToolName("builtin", vt.Name)
			params := shared.FunctionParameters{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
			// Convert protocol-neutral input schema to shared.FunctionParameters via JSON.
			schemaBytes, _ := json.Marshal(vt.InputSchema)
			var schema map[string]interface{}
			if err := json.Unmarshal(schemaBytes, &schema); err == nil && len(schema) > 0 {
				params = shared.FunctionParameters(schema)
			}
			def := shared.FunctionDefinitionParam{
				Name:       normalized,
				Parameters: params,
			}
			if vt.Description != "" {
				def.Description = param.NewOpt(vt.Description)
			}
			out = append(out, openai.ChatCompletionFunctionTool(def))
		}
	}
	return out
}

func (r *Runtime) isVirtualServerToolInjectable(vt coretool.VirtualTool) bool {
	if strings.TrimSpace(vt.Name) == "" || !IsServerVisibleVirtualTool(vt) {
		return false
	}

	// Builtin adviser injection must follow source enablement and server-tool semantics.
	if vt.Name == mcptools.BuiltinAdvisorToolName {
		cfg := r.getConfigOrDefault()
		if cfg == nil {
			return false
		}
		for _, source := range cfg.Sources {
			if source.ID != mcptools.BuiltinAdvisorSourceID {
				continue
			}
			if !typ.IsMCPSourceEnabled(source) {
				return false
			}
			if !IsServerVisibleSource(source) {
				return false
			}
			allowAll, allowSet := buildAllowList(source.Tools)
			return allowAll || allowSet[vt.Name]
		}
		return false
	}

	return true
}

// IsMCPToolName checks whether a tool name is a normalized MCP tool.
func IsMCPToolName(name string) bool {
	return coretool.IsMCPToolName(name)
}

// NormalizeToolName converts source/tool pair to normalized tool name.
func NormalizeToolName(sourceID, toolName string) string {
	return coretool.NormalizeToolName(sourceID, toolName)
}

// ParseNormalizedToolName parses normalized name and returns sourceID/toolName.
func ParseNormalizedToolName(name string) (string, string, bool) {
	return coretool.ParseNormalizedToolName(name)
}

// CallTool executes a normalized MCP tool call and returns a structured ToolResult.
// Dispatches virtual tools first (kernel mode), then remote tools (user mode).
func (r *Runtime) CallTool(ctx context.Context, normalizedName string, arguments string) (coretool.ToolResult, error) {
	if r == nil {
		return coretool.ToolResult{}, fmt.Errorf("MCP runtime not initialized")
	}
	// 1. Check virtual registry first (kernel mode)
	sourceID, toolName, ok := ParseNormalizedToolName(normalizedName)
	if !ok {
		return coretool.ToolResult{}, &sessionError{sourceID: sourceID, msg: "invalid normalized MCP tool name: " + normalizedName}
	}

	if sourceID == "builtin" && r.virtualRegistry != nil {
		if tool, ok := r.virtualRegistry.Get(toolName); ok {
			return r.callVirtualTool(ctx, tool, arguments)
		}
	}

	// Backward compatibility: legacy adviser source ID maps to builtin virtual tool.
	if sourceID == mcptools.BuiltinAdvisorSourceID && r.virtualRegistry != nil {
		if tool, ok := r.virtualRegistry.Get(toolName); ok {
			return r.callVirtualTool(ctx, tool, arguments)
		}
	}

	// 2. Forward to remote tool source (user mode)
	source, err := r.getOrCreateSource(ctx, sourceID)
	if err != nil {
		return coretool.ToolResult{}, err
	}

	// Ensure source is connected
	if !source.IsConnected() {
		if err := source.Connect(ctx); err != nil {
			return coretool.ToolResult{}, &sessionError{sourceID: sourceID, msg: "failed to connect source: " + err.Error()}
		}

		// Enable health monitoring for persistent connections
		if transport := source.GetType(); transport == TransportHTTP || transport == TransportSSE {
			source.EnableHealthCheck(ctx, 30*time.Second)
		}
	}

	// Call the tool — remote sources still return string; wrap into ToolResult.
	result, err := source.CallTool(ctx, toolName, arguments)
	if err != nil {
		return coretool.ToolResult{}, err
	}

	return coretool.TextToolResult(result), nil
}

// callVirtualTool executes an in-process virtual tool with panic recovery.
func (r *Runtime) callVirtualTool(ctx context.Context, tool coretool.VirtualTool, arguments string) (out coretool.ToolResult, err error) {
	// Parse arguments
	var argMap map[string]any
	if arguments != "" {
		if jsonErr := json.Unmarshal([]byte(arguments), &argMap); jsonErr != nil {
			return coretool.ToolResult{}, fmt.Errorf("invalid arguments JSON: %w", jsonErr)
		}
	}

	// Execute with panic recovery — named returns allow the deferred func to set err.
	defer func() {
		if rec := recover(); rec != nil {
			logrus.WithField("panic", rec).Error("mcp: virtual tool panic")
			err = fmt.Errorf("virtual tool panic: %v", rec)
		}
	}()

	if tool.Handler == nil {
		return coretool.ToolResult{}, fmt.Errorf("virtual tool %q has no handler", tool.Name)
	}
	return tool.Handler(ctx, coretool.ToolCall{Name: tool.Name, Arguments: argMap})
}

// isSourceEnabled checks if a source is enabled in the current configuration
func (r *Runtime) isSourceEnabled(sourceID string) bool {
	cfg := r.getConfigOrDefault()
	if cfg == nil {
		return false
	}

	for i := range cfg.Sources {
		if cfg.Sources[i].ID == sourceID {
			return typ.IsMCPSourceEnabled(cfg.Sources[i])
		}
	}
	return false
}

// invalidateSource removes a source from the cache and disconnects it
func (r *Runtime) invalidateSource(ctx context.Context, sourceID string) {
	r.sourcesMu.Lock()
	defer r.sourcesMu.Unlock()

	if source, exists := r.activeSources[sourceID]; exists {
		// Disconnect the source
		if err := source.Disconnect(ctx); err != nil {
			logrus.WithField("source", sourceID).WithError(err).
				Warn("mcp: failed to disconnect source during invalidation")
		}
		delete(r.activeSources, sourceID)
		logrus.WithField("source", sourceID).Debug("mcp: invalidated cached source")
	}
}

// getOrCreateSource gets an existing tool source or creates a new one.
func (r *Runtime) getOrCreateSource(ctx context.Context, sourceID string) (ToolSource, error) {
	// Fast path: check cache
	r.sourcesMu.RLock()
	source := r.activeSources[sourceID]
	r.sourcesMu.RUnlock()

	if source != nil {
		// Check if source is still enabled in current config
		if !r.isSourceEnabled(sourceID) {
			// Source is disabled, remove from cache and return error
			r.invalidateSource(ctx, sourceID)
			return nil, &sessionError{sourceID: sourceID, msg: "mcp source " + sourceID + " is disabled"}
		}
		return source, nil
	}

	// Slow path: create new source
	r.sourcesMu.Lock()
	defer r.sourcesMu.Unlock()

	// Double-check
	source = r.activeSources[sourceID]
	if source != nil {
		// Check if source is still enabled in current config
		if !r.isSourceEnabled(sourceID) {
			// Source is disabled, remove from cache and return error
			if err := source.Disconnect(ctx); err != nil {
				logrus.WithField("source", sourceID).WithError(err).
					Warn("mcp: failed to disconnect source")
			}
			delete(r.activeSources, sourceID)
			return nil, &sessionError{sourceID: sourceID, msg: "mcp source " + sourceID + " is disabled"}
		}
		return source, nil
	}

	// Find source config
	cfg := r.getConfigOrDefault()
	if cfg == nil {
		return nil, &sessionError{sourceID: sourceID, msg: "mcp runtime config is not set"}
	}

	var sourceConfig *typ.MCPSourceConfig
	for i := range cfg.Sources {
		if cfg.Sources[i].ID == sourceID {
			sourceConfig = &cfg.Sources[i]
			break
		}
	}

	if sourceConfig == nil {
		return nil, &sessionError{sourceID: sourceID, msg: "mcp source " + sourceID + " not found"}
	}

	// Virtual sources (e.g. advisor) are handled in-process; they are never backed by a subprocess or remote connection.
	if sourceConfig.Advisor != nil {
		return nil, &sessionError{sourceID: sourceID, msg: "mcp source " + sourceID + " is a virtual tool (use virtual registry)"}
	}

	if !typ.IsMCPSourceEnabled(*sourceConfig) {
		return nil, &sessionError{sourceID: sourceID, msg: "mcp source " + sourceID + " is disabled"}
	}
	if missing := ValidateEnabledMCPSourceEnvRefs([]typ.MCPSourceConfig{*sourceConfig}); len(missing) > 0 {
		first := missing[0]
		return nil, &sessionError{
			sourceID: sourceID,
			msg:      "missing environment variable " + first.VarName + " for " + first.FieldPath,
		}
	}

	// Create tool source using factory
	newSource, err := r.toolSourceFactory.CreateToolSource(*sourceConfig)
	if err != nil {
		return nil, &sessionError{sourceID: sourceID, msg: "failed to create tool source: " + err.Error()}
	}

	// Cache the source
	r.activeSources[sourceID] = newSource

	logrus.WithField("source", sourceID).WithField("transport", newSource.GetType()).
		Debug("mcp: created tool source")

	return newSource, nil
}

// SourceTool represents a tool from a specific MCP source with its original name.
type SourceTool struct {
	SourceID    string
	SourceName  string
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ListSourceTools returns all MCP tools grouped by source with their original names.
// This is used by local mode to expose tools to external MCP clients.
func (r *Runtime) ListSourceTools(ctx context.Context) (map[string][]SourceTool, error) {
	cfg := r.getConfigOrDefault()
	if cfg == nil || len(cfg.Sources) == 0 {
		return nil, nil
	}

	result := make(map[string][]SourceTool)

	for _, source := range cfg.Sources {
		if !typ.IsMCPSourceEnabled(source) {
			continue
		}

		// Skip virtual-only sources (e.g., adviser) — they are not remote tool sources.
		if source.Advisor != nil {
			continue
		}

		// Get or create tool source
		toolSource, err := r.getOrCreateSource(ctx, source.ID)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"source": source.ID,
				"error":  err.Error(),
			}).Warn("mcp: failed to get tool source for ListSourceTools")
			continue
		}

		// Ensure source is connected
		if !toolSource.IsConnected() {
			if err := toolSource.Connect(ctx); err != nil {
				logrus.WithFields(logrus.Fields{
					"source": source.ID,
					"error":  err.Error(),
				}).Warn("mcp: connect failed for ListSourceTools")
				continue
			}
		}

		// List tools from source
		tools, err := toolSource.ListTools(ctx)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"source": source.ID,
				"error":  err.Error(),
			}).Warn("mcp: list tools failed for ListSourceTools")
			continue
		}

		// Apply allow list filtering
		allowAll, allowSet := buildAllowList(source.Tools)
		for _, t := range tools {
			if strings.TrimSpace(t.Name) == "" {
				continue
			}
			if !allowAll && !allowSet[t.Name] {
				continue
			}

			result[source.ID] = append(result[source.ID], SourceTool{
				SourceID:    source.ID,
				SourceName:  source.Name,
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	return result, nil
}

// ListClientSourceToolsForMCP returns source tools that are eligible for MCP client exposure.
// Rules:
//   - source must be enabled
//   - source must be client-visible
//   - tool must come from non-virtual source (ListSourceTools contract)
func (r *Runtime) ListClientSourceToolsForMCP(ctx context.Context) (map[string][]SourceTool, error) {
	sourceTools, err := r.ListSourceTools(ctx)
	if err != nil {
		return nil, err
	}
	if len(sourceTools) == 0 {
		return sourceTools, nil
	}

	cfg := r.getConfigOrDefault()
	if cfg == nil {
		return map[string][]SourceTool{}, nil
	}

	clientSources := make(map[string]bool)
	for _, source := range cfg.Sources {
		if !typ.IsMCPSourceEnabled(source) {
			continue
		}
		if IsClientVisibleSource(source) {
			clientSources[source.ID] = true
		}
	}

	filtered := make(map[string][]SourceTool)
	for sourceID, tools := range sourceTools {
		if !clientSources[sourceID] {
			continue
		}
		filtered[sourceID] = tools
	}
	return filtered, nil
}

func (r *Runtime) getConfigOrDefault() *typ.MCPRuntimeConfig {
	if r == nil || r.getConfig == nil {
		return nil
	}
	cfg := r.getConfig()
	if cfg == nil {
		return nil
	}
	var clone typ.MCPRuntimeConfig
	b, err := json.Marshal(cfg)
	if err != nil {
		logrus.WithError(err).Warn("mcp: failed to clone runtime config, using original")
		clone = *cfg
	} else if err := json.Unmarshal(b, &clone); err != nil {
		logrus.WithError(err).Warn("mcp: failed to unmarshal cloned runtime config, using original")
		clone = *cfg
	}

	typ.ApplyMCPRuntimeDefaults(&clone)
	missing := ExpandMCPRuntimeEnvRefs(&clone)
	for _, issue := range missing {
		logrus.WithFields(logrus.Fields{
			"source": issue.SourceID,
			"field":  issue.FieldPath,
			"var":    issue.VarName,
		}).Warn("mcp: unresolved environment reference in runtime config")
	}
	return &clone
}

// GetConfig returns the current MCP runtime configuration.
func (r *Runtime) GetConfig() *typ.MCPRuntimeConfig {
	return r.getConfigOrDefault()
}

// VirtualRegistry returns the runtime's virtual tool registry, or nil if the runtime is nil.
func (r *Runtime) VirtualRegistry() *coretool.VirtualToolRegistry {
	if r == nil {
		return nil
	}
	return r.virtualRegistry
}

// SessionStore returns the runtime's session store.
func (r *Runtime) SessionStore() *SessionStore {
	if r == nil {
		return nil
	}
	return r.sessionStore
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

// RegisterAdviser registers the adviser as a virtual tool in the runtime.
func (r *Runtime) RegisterAdviser(cfg typ.AdvisorConfig, cp *client.ClientPool) {
	if r == nil || r.virtualRegistry == nil {
		return
	}
	tool := NewAdvisorVirtualTool(cfg, cp, r.sessionStore)
	r.virtualRegistry.Register(tool)
}

// GetAdvisorMaxUses returns the MaxUsesPerRequest from the advisor source config.
// Returns 0 if no advisor is configured or the value is not positive.
func (r *Runtime) GetAdvisorMaxUses() int {
	if r == nil {
		return 0
	}
	cfg := r.GetConfig()
	if cfg == nil {
		return 0
	}
	for _, source := range cfg.Sources {
		if source.Advisor != nil && source.Advisor.MaxUsesPerRequest > 0 {
			return source.Advisor.MaxUsesPerRequest
		}
	}
	return 0
}

// HasServerTools returns true if there are any server tools configured
func (r *Runtime) HasServerTools() bool {
	cfg := r.getConfigOrDefault()
	if cfg == nil || len(cfg.Sources) == 0 {
		return false
	}
	for _, source := range cfg.Sources {
		if !typ.IsMCPSourceEnabled(source) {
			continue
		}
		if IsServerVisibleSource(source) {
			return true
		}
	}
	return false
}

const enabledNamesCacheTTL = 5 * time.Second

func (r *Runtime) ListClientVisibleMCPToolNames(ctx context.Context) map[string]struct{} {
	out := make(map[string]struct{})
	if r == nil {
		return out
	}
	if clientTools, err := r.ListClientSourceToolsForMCP(ctx); err == nil {
		for sourceID, tools := range clientTools {
			for _, t := range tools {
				if strings.TrimSpace(t.Name) == "" {
					continue
				}
				out[NormalizeToolName(sourceID, t.Name)] = struct{}{}
			}
		}
	}
	return out
}

func (r *Runtime) ListInjectableServerToolNames(ctx context.Context) map[string]struct{} {
	out := make(map[string]struct{})
	if r == nil {
		return out
	}
	for _, t := range r.ListServerToolsForInjection(ctx) {
		fn := t.GetFunction()
		if fn == nil || fn.Name == "" {
			continue
		}
		out[fn.Name] = struct{}{}
	}
	return out
}

func (r *Runtime) ListCallableServerToolNames(ctx context.Context) map[string]struct{} {
	out := r.ListInjectableServerToolNames(ctx)
	if r == nil {
		return out
	}
	cfg := r.getConfigOrDefault()
	if cfg != nil {
		for _, source := range cfg.Sources {
			if source.ID != mcptools.BuiltinAdvisorSourceID || !typ.IsMCPSourceEnabled(source) || !IsServerVisibleSource(source) {
				continue
			}
			allowAll, allowSet := buildAllowList(source.Tools)
			if allowAll || allowSet[mcptools.BuiltinAdvisorToolName] {
				out[NormalizeToolName("builtin", mcptools.BuiltinAdvisorToolName)] = struct{}{}
				out[NormalizeToolName(mcptools.BuiltinAdvisorSourceID, mcptools.BuiltinAdvisorToolName)] = struct{}{}
			}
		}
	}
	if r.virtualRegistry != nil {
		for _, vt := range r.virtualRegistry.ListVirtualTools() {
			if !IsServerVisibleVirtualTool(vt) || strings.TrimSpace(vt.Name) == "" {
				continue
			}
			if !r.isVirtualServerToolInjectable(vt) {
				continue
			}
			out[NormalizeToolName("builtin", vt.Name)] = struct{}{}
			if vt.Name == "advisor" {
				out[NormalizeToolName("advisor", vt.Name)] = struct{}{}
			}
		}
	}
	return out
}

// IsClientToolAvailable reports whether a client-visible tool on the given source is
// enabled and configured to handle requests.
//
// The check is two-layered:
//  1. Source-level: enabled + IsConfigured() on the ToolSource implementation.
//     Custom MCP servers (stdio/http/sse) use their transport-specific check
//     (command non-empty / endpoint non-empty / env refs resolved).
//  2. Tool-level: for builtin sources, a per-tool ToolReadinessChecker may impose
//     additional requirements (e.g. mcp_web_search requires SERPER_API_KEY).
func (r *Runtime) IsClientToolAvailable(sourceID string, toolName string) bool {
	if r == nil {
		return false
	}
	cfg := r.getConfigOrDefault()
	if cfg == nil {
		return false
	}
	for _, source := range cfg.Sources {
		if source.ID != sourceID {
			continue
		}
		if !typ.IsMCPSourceEnabled(source) {
			return false
		}
		// Check tool is listed for this source.
		found := false
		for _, t := range source.Tools {
			if t == toolName || t == "*" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
		// Source-level configuration check via ToolSource.IsConfigured().
		// Expand env refs first so IsConfigured sees resolved values.
		expandedSource := source
		expandedSource.Env = make(map[string]string, len(source.Env))
		for k, v := range source.Env {
			expandedSource.Env[k] = v
		}
		expandSourceEnvInPlace(expandedSource.Env)
		ts, err := r.toolSourceFactory.CreateToolSource(expandedSource)
		if err != nil || !ts.IsConfigured() {
			return false
		}
		// Tool-level readiness check (builtin sources only).
		checker, ok := mcptools.WebtoolReadinessCheckers[toolName]
		if !ok {
			return true
		}
		return checker.IsReady(expandedSource.Env)
	}
	return false
}

// expandSourceEnvInPlace expands ${VAR} references in an env map using os.Getenv as fallback.
func expandSourceEnvInPlace(env map[string]string) {
	for k, v := range env {
		expanded, _ := expandStringEnvRefs(v, env, true)
		env[k] = expanded
	}
}

// ListEnabledServerToolNames returns normalized MCP tool names that are callable by
// server-side MCP execution paths. This includes:
//   - injected server-side virtual tools (via ListServerToolsForInjection)
//   - backward-compatible aliases for server-only virtual tools (e.g. advisor source id)
//
// Results are cached with a short TTL to avoid repeated full source enumeration.
func (r *Runtime) ListEnabledServerToolNames(ctx context.Context) map[string]struct{} {
	if r == nil {
		return nil
	}
	r.enabledNamesMu.RLock()
	if r.enabledNamesCache != nil && time.Now().Before(r.enabledNamesExpires) {
		r.enabledNamesMu.RUnlock()
		return r.enabledNamesCache
	}
	r.enabledNamesMu.RUnlock()

	r.enabledNamesMu.Lock()
	defer r.enabledNamesMu.Unlock()
	if r.enabledNamesCache != nil && time.Now().Before(r.enabledNamesExpires) {
		return r.enabledNamesCache
	}
	out := r.ListClientVisibleMCPToolNames(ctx)
	for name := range r.ListCallableServerToolNames(ctx) {
		out[name] = struct{}{}
	}
	r.enabledNamesCache = out
	r.enabledNamesExpires = time.Now().Add(enabledNamesCacheTTL)
	return out
}
