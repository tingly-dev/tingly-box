package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const normalizedPrefix = "tingly_box_mcp__"

// advisorDepthKey is the context key for tracking adviser call depth.
type advisorDepthKey struct{}

// WithAdvisorDepth sets the adviser call depth in context.
func WithAdvisorDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, advisorDepthKey{}, depth)
}

// GetAdvisorDepth retrieves the current adviser call depth from context.
func GetAdvisorDepth(ctx context.Context) int {
	v, _ := ctx.Value(advisorDepthKey{}).(int)
	return v
}

type configProvider func() *typ.MCPRuntimeConfig

// Runtime handles MCP tool source discovery and tool execution.
type Runtime struct {
	getConfig         configProvider
	sc                *sessionCache
	toolSourceFactory *ToolSourceFactory
	activeSources     map[string]ToolSource // source ID -> ToolSource
	sourcesMu         sync.RWMutex
	virtualRegistry   *VirtualToolRegistry
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
		virtualRegistry:   NewVirtualToolRegistry(),
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
			// Convert mcp.ToolInputSchema to shared.FunctionParameters via JSON
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

func (r *Runtime) isVirtualServerToolInjectable(vt VirtualTool) bool {
	if strings.TrimSpace(vt.Name) == "" || vt.IsClientTool {
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
			if source.IsClientTool != nil && *source.IsClientTool {
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
// Dispatches virtual tools first (kernel mode), then remote tools (user mode).
func (r *Runtime) CallTool(ctx context.Context, normalizedName string, arguments string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("MCP runtime not initialized")
	}
	// 1. Check virtual registry first (kernel mode)
	sourceID, toolName, ok := ParseNormalizedToolName(normalizedName)
	if !ok {
		return "", &sessionError{sourceID: sourceID, msg: "invalid normalized MCP tool name: " + normalizedName}
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
		return "", err
	}

	// Ensure source is connected
	if !source.IsConnected() {
		if err := source.Connect(ctx); err != nil {
			return "", &sessionError{sourceID: sourceID, msg: "failed to connect source: " + err.Error()}
		}

		// Enable health monitoring for persistent connections
		if transport := source.GetType(); transport == TransportHTTP || transport == TransportSSE {
			source.EnableHealthCheck(ctx, 30*time.Second)
		}
	}

	// Call the tool
	result, err := source.CallTool(ctx, toolName, arguments)
	if err != nil {
		return "", err
	}

	return result, nil
}

// callVirtualTool executes an in-process virtual tool with panic recovery.
func (r *Runtime) callVirtualTool(ctx context.Context, tool VirtualTool, arguments string) (string, error) {
	// Parse arguments
	var argMap map[string]any
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &argMap); err != nil {
			return "", fmt.Errorf("invalid arguments JSON: %w", err)
		}
	}

	// Build mcp.CallToolRequest
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      tool.Name,
			Arguments: argMap,
		},
	}

	// Execute with panic recovery
	defer func() {
		if rec := recover(); rec != nil {
			logrus.WithField("panic", rec).Error("mcp: virtual tool panic")
		}
	}()

	result, err := tool.Handler(ctx, req)
	if err != nil {
		return "", err
	}

	// Serialize result
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to serialize tool result: %w", err)
	}

	// Extract text content from result
	if len(result.Content) > 0 {
		if textContent, ok := result.Content[0].(mcp.TextContent); ok {
			return string(textContent.Text), nil
		}
	}

	return string(resultBytes), nil
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
//   - source must be client tool (is_client_tool=true)
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
		if source.IsClientTool != nil && *source.IsClientTool {
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
func (r *Runtime) VirtualRegistry() *VirtualToolRegistry {
	if r == nil {
		return nil
	}
	return r.virtualRegistry
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
		// Check if this is a server tool (nil or false means server tool)
		if source.IsClientTool == nil || !*source.IsClientTool {
			return true
		}
	}
	return false
}

const enabledNamesCacheTTL = 5 * time.Second

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
	out := make(map[string]struct{})
	// Include enabled client-facing non-virtual MCP tools so strip-guard does not
	// incorrectly remove valid tools declared by MCP clients.
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

	// Include server-side injected virtual tools.
	for _, t := range r.ListServerToolsForInjection(ctx) {
		fn := t.GetFunction()
		if fn == nil || fn.Name == "" {
			continue
		}
		out[fn.Name] = struct{}{}
	}
	// Server-only virtual tools are intentionally hidden from ListServerToolsForInjection,
	// but still need to be executable by server-side MCP loops.
	if r.virtualRegistry != nil {
		for _, vt := range r.virtualRegistry.ListVirtualTools() {
			if vt.IsClientTool || strings.TrimSpace(vt.Name) == "" {
				continue
			}
			if !r.isVirtualServerToolInjectable(vt) {
				continue
			}
			out[NormalizeToolName("builtin", vt.Name)] = struct{}{}
			// Backward compatibility: older flows may still reference advisor source id.
			if vt.Name == "advisor" {
				out[NormalizeToolName("advisor", vt.Name)] = struct{}{}
			}
		}
	}
	r.enabledNamesCache = out
	r.enabledNamesExpires = time.Now().Add(enabledNamesCacheTTL)
	return out
}
