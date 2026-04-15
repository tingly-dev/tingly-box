package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const normalizedPrefix = "tingly_box_mcp__"

type configProvider func() *typ.MCPRuntimeConfig

// Runtime handles MCP tool source discovery and tool execution.
type Runtime struct {
	getConfig         configProvider
	sc                *sessionCache
	toolSourceFactory *ToolSourceFactory
	activeSources     map[string]ToolSource // source ID -> ToolSource
	sourcesMu         sync.RWMutex

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
	return &Runtime{
		getConfig:         getConfig,
		sc:                sc,
		toolSourceFactory: NewToolSourceFactory(sc, nil),
		activeSources:     make(map[string]ToolSource),
	}
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
		// Skip client tools - they should not be injected into AI requests
		if source.IsClientTool != nil && *source.IsClientTool {
			logrus.Debugf("mcp: source=%s is a client tool; skip tool injection", source.ID)
			continue
		}

		// Get or create tool source
		toolSource, err := r.getOrCreateSource(ctx, source.ID)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"source": source.ID,
				"error":  err.Error(),
			}).Warn("mcp: failed to get tool source")
			continue
		}

		// Ensure source is connected
		if !toolSource.IsConnected() {
			if err := toolSource.Connect(ctx); err != nil {
				logrus.WithFields(logrus.Fields{
					"source":    source.ID,
					"transport": toolSource.GetType(),
					"error":     err.Error(),
				}).Warn("mcp: connect failed")
				continue
			}
		}

		// List tools from source
		tools, err := toolSource.ListTools(ctx)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"source": source.ID,
				"error":  err.Error(),
			}).Warn("mcp: list tools failed")
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

			normalized := NormalizeToolName(source.ID, t.Name)
			params := shared.FunctionParameters{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
			if raw := t.InputSchema; len(raw) > 0 {
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

	// Get or create tool source
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

	if !typ.IsMCPSourceEnabled(*sourceConfig) {
		return nil, &sessionError{sourceID: sourceID, msg: "mcp source " + sourceID + " is disabled"}
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

// GetConfig returns the current MCP runtime configuration.
func (r *Runtime) GetConfig() *typ.MCPRuntimeConfig {
	return r.getConfigOrDefault()
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

// ListEnabledServerToolNames returns normalized MCP tool names from enabled server-tool sources.
// Results are cached with a short TTL to avoid repeated full source enumeration.
func (r *Runtime) ListEnabledServerToolNames(ctx context.Context) map[string]struct{} {
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
	for _, t := range r.ListOpenAITools(ctx) {
		fn := t.GetFunction()
		if fn == nil || fn.Name == "" {
			continue
		}
		out[fn.Name] = struct{}{}
	}
	r.enabledNamesCache = out
	r.enabledNamesExpires = time.Now().Add(enabledNamesCacheTTL)
	return out
}
