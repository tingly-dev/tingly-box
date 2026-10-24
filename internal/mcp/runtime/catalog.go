package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

type CatalogTool struct {
	SourceID       string                 `json:"source_id"`
	Name           string                 `json:"name"`
	NormalizedName string                 `json:"normalized_name"`
	Description    string                 `json:"description,omitempty"`
	InputSchema    json.RawMessage        `json:"input_schema,omitempty"`
	OutputSchema   json.RawMessage        `json:"output_schema,omitempty"`
	Annotations    json.RawMessage        `json:"annotations,omitempty"`
	Enabled        bool                   `json:"enabled"`
	Usage          typ.MCPToolUsage       `json:"usage"`
	Implementation typ.ToolImplementation `json:"implementation"`
}

type SourceStatus struct {
	SourceID string        `json:"source_id"`
	State    string        `json:"state"`
	Error    string        `json:"error,omitempty"`
	Tools    []CatalogTool `json:"tools"`
}

func (r *Runtime) DiscoverSource(ctx context.Context, id string) (SourceStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	status := SourceStatus{SourceID: id, State: "disconnected", Tools: []CatalogTool{}}
	cfg := r.GetConfig()
	if cfg == nil {
		return status, fmt.Errorf("MCP runtime not initialized")
	}
	var source *typ.MCPSourceConfig
	for i := range cfg.Sources {
		if cfg.Sources[i].ID == id {
			source = &cfg.Sources[i]
			break
		}
	}
	if source == nil {
		return status, &SourceNotFoundError{ID: id}
	}
	if !typ.IsMCPSourceEnabled(*source) {
		status.State = "disabled"
		return status, nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.RequestTimeout)*time.Second)
	defer cancel()
	var definitions []ToolDefinition
	if SourceImplementation(*source) == typ.ToolImplementationVirtual {
		if r.virtualRegistry != nil {
			for _, vt := range r.virtualRegistry.ListVirtualTools() {
				if source.ID == "advisor" && vt.Name == "advisor" {
					schema, _ := json.Marshal(vt.InputSchema)
					definitions = append(definitions, ToolDefinition{Name: vt.Name, Description: vt.Description, InputSchema: schema})
				}
			}
		}
		if len(definitions) == 0 {
			status.State = "unconfigured"
			return status, nil
		}
	} else {
		ts, err := r.getOrCreateSource(ctx, id)
		if err == nil && !ts.IsConfigured() {
			status.State = "unconfigured"
			return status, nil
		}
		if err == nil && !ts.IsConnected() {
			err = ts.Connect(ctx)
		}
		if err == nil {
			definitions, err = ts.ListTools(ctx)
		}
		if err != nil {
			status.State = "error"
			status.Error = err.Error()
			return status, err
		}
	}
	status.State = "connected"
	for _, def := range definitions {
		if def.Name == "" {
			continue
		}
		enabled, usage := EffectiveToolPolicy(*source, def.Name)
		normalized := NormalizeToolName(id, def.Name)
		if SourceImplementation(*source) == typ.ToolImplementationVirtual {
			normalized = NormalizeToolName("builtin", def.Name)
		}
		status.Tools = append(status.Tools, CatalogTool{SourceID: id, Name: def.Name, NormalizedName: normalized, Description: def.Description,
			InputSchema: def.InputSchema, OutputSchema: def.OutputSchema, Annotations: def.Annotations, Enabled: enabled, Usage: usage, Implementation: SourceImplementation(*source)})
	}
	sort.Slice(status.Tools, func(i, j int) bool { return status.Tools[i].Name < status.Tools[j].Name })
	return status, nil
}

// Catalog discovers each source independently with bounded concurrency. Errors
// stay attached to the source; one failed server never turns the catalog empty.
func (r *Runtime) Catalog(ctx context.Context, usage string) []SourceStatus {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := r.GetConfig()
	if cfg == nil {
		return []SourceStatus{}
	}
	var sources []typ.MCPSourceConfig
	for _, source := range cfg.Sources {
		if usage != "" {
			eligible := SourceUsage(source)
			for _, policy := range source.ToolPolicies {
				if policy.Usage != nil {
					eligible.Client = eligible.Client || policy.Usage.Client
					eligible.Gateway = eligible.Gateway || policy.Usage.Gateway
				}
			}
			if usage == "client" && !eligible.Client || usage == "gateway" && !eligible.Gateway {
				continue
			}
		}
		sources = append(sources, source)
	}
	out := make([]SourceStatus, len(sources))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, source := range sources {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				out[i] = SourceStatus{SourceID: id, State: "error", Error: ctx.Err().Error(), Tools: []CatalogTool{}}
				return
			}
			status, err := r.DiscoverSource(ctx, id)
			if err != nil && status.Error == "" {
				status.State = "error"
				status.Error = err.Error()
			}
			out[i] = status
		}(i, source.ID)
	}
	wg.Wait()
	return out
}
