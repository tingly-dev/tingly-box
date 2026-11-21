package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func connectionKey(source typ.MCPSourceConfig) string {
	// Policy changes do not restart upstream connections.
	source.Enabled = nil
	source.Name = ""
	source.Tools = nil
	source.Visibility = ""
	source.Usage = nil
	source.ToolPolicies = nil
	b, _ := json.Marshal(source)
	return string(b)
}

func (r *Runtime) sourceLock(id string) *sync.Mutex {
	value, _ := r.sourceLocks.LoadOrStore(id, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func (r *Runtime) invalidateSource(ctx context.Context, id string) {
	lock := r.sourceLock(id)
	lock.Lock()
	defer lock.Unlock()
	r.detachSource(ctx, id)
}

// detachSource is called with the per-source lock, never the global map lock.
func (r *Runtime) detachSource(ctx context.Context, id string) {
	r.sourcesMu.Lock()
	source := r.activeSources[id]
	delete(r.activeSources, id)
	r.sourcesMu.Unlock()
	if source != nil {
		if retiring, ok := source.(interface{ Retire() }); ok {
			retiring.Retire()
		}
		_ = source.Disconnect(ctx)
	}
	r.sc.remove(id)
	r.enabledNamesMu.Lock()
	r.enabledNamesCache = nil
	r.enabledNamesMu.Unlock()
}

func (r *Runtime) getOrCreateSource(ctx context.Context, id string) (ToolSource, error) {
	lock := r.sourceLock(id)
	lock.Lock()
	defer lock.Unlock()
	cfg := r.getConfigOrDefault()
	if cfg == nil {
		return nil, fmt.Errorf("MCP runtime config is not set")
	}
	var wanted *typ.MCPSourceConfig
	for i := range cfg.Sources {
		if cfg.Sources[i].ID == id {
			wanted = &cfg.Sources[i]
			break
		}
	}
	if wanted == nil || !typ.IsMCPSourceEnabled(*wanted) {
		r.detachSource(ctx, id)
		return nil, fmt.Errorf("MCP source %s is missing or disabled", id)
	}
	if wanted.Advisor != nil {
		return nil, fmt.Errorf("MCP source %s is virtual", id)
	}
	if issues := ValidateEnabledMCPSourceEnvRefs([]typ.MCPSourceConfig{*wanted}); len(issues) > 0 {
		return nil, fmt.Errorf("source %s: missing environment variable %s", id, issues[0].VarName)
	}
	expanded := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{*wanted}}
	ExpandMCPRuntimeEnvRefs(expanded)
	wanted = &expanded.Sources[0]
	r.sourcesMu.RLock()
	source := r.activeSources[id]
	closed := r.closed
	r.sourcesMu.RUnlock()
	if closed {
		return nil, fmt.Errorf("MCP runtime is shutting down")
	}
	if source != nil {
		if existing, ok := source.GetSourceConfig().(typ.MCPSourceConfig); ok && connectionKey(existing) == connectionKey(*wanted) {
			return source, nil
		}
		r.detachSource(ctx, id)
	}
	source, err := r.toolSourceFactory.CreateToolSource(*wanted)
	if err != nil {
		return nil, err
	}
	r.sourcesMu.Lock()
	defer r.sourcesMu.Unlock()
	if r.closed {
		return nil, fmt.Errorf("MCP runtime is shutting down")
	}
	r.activeSources[id] = source
	return source, nil
}

// Reconcile closes removed/changed sources and invalidates tool-name caches.
// Sources reconnect lazily; a policy-only edit keeps the current connection.
func (r *Runtime) Reconcile(ctx context.Context) {
	if r == nil {
		return
	}
	r.sourcesMu.RLock()
	ids := make([]string, 0, len(r.activeSources))
	for id := range r.activeSources {
		ids = append(ids, id)
	}
	r.sourcesMu.RUnlock()
	for _, id := range ids {
		_, _ = r.getOrCreateSource(ctx, id)
	}
	r.enabledNamesMu.Lock()
	r.enabledNamesCache = nil
	r.enabledNamesMu.Unlock()
}

func (r *Runtime) ReconnectSource(ctx context.Context, id string) (SourceStatus, error) {
	r.invalidateSource(ctx, id)
	return r.DiscoverSource(ctx, id)
}
