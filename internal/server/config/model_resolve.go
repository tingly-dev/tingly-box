package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ModelListSource identifies where a resolved model list came from. Its string
// values match provider.ModelCacheSource so the HTTP layer can map directly.
type ModelListSource string

const (
	ModelListSourceCache    ModelListSource = "db"       // served from the DB cache
	ModelListSourceAPI      ModelListSource = "api"      // freshly fetched from upstream
	ModelListSourceVModel   ModelListSource = "vmodel"   // virtual provider's static list
	ModelListSourceTemplate ModelListSource = "template" // compile-time embedded fallback
)

// ResolvedModels is the effective model list for a provider plus provenance.
type ResolvedModels struct {
	Models      []string
	Source      ModelListSource
	LastUpdated string // formatted timestamp of the cached record; empty if unknown
}

// ResolveProviderModels returns the effective model list for a provider,
// walking the full fallback chain in ONE place so every caller (HTTP read,
// HTTP refresh, CLI, OAuth completion) sees identical behavior:
//
//	cache (DB) → VModel (virtual) → upstream API → embedded template
//
// When forceRefresh is false, a fresh-enough DB cache short-circuits the
// chain; when true the cache is bypassed and upstream is re-queried (the
// manual "refresh models" action). The returned list is canonically sorted.
//
// Template results are never persisted, so an improved embedded list takes
// effect immediately and a template snapshot never pollutes a real cached
// list. This method always fetches on a cache miss — callers that must not
// touch the network (e.g. TUI render) should read the cache/template directly.
func (c *Config) ResolveProviderModels(forceRefresh, forceUpstream bool, uid string) (ResolvedModels, error) {
	provider, provErr := c.GetProviderByUUID(uid)

	finalize := func(models []string, src ModelListSource) ResolvedModels {
		if provErr == nil {
			SortProviderModels(provider, models)
		}
		lastUpdated := ""
		if _, updated, exists := c.modelManager.GetProviderInfo(uid); exists {
			lastUpdated = updated
		}
		return ResolvedModels{Models: models, Source: src, LastUpdated: lastUpdated}
	}

	// A forced upstream fetch re-queries the real upstream by definition, so it
	// bypasses the DB cache. (forceRefresh alone also bypasses it.)
	if !forceRefresh && !forceUpstream {
		if cached := c.modelManager.GetModels(uid); len(cached) > 0 {
			return finalize(cached, ModelListSourceCache), nil
		}
	}

	if provErr != nil {
		return ResolvedModels{}, fmt.Errorf("provider with UUID %s not found: %w", uid, provErr)
	}

	// Step 2: VModel static list. A forced upstream attempt skips this shortcut.
	if !forceUpstream && provider.IsVirtual() {
		var models []string
		if provider.VModelDetail != nil {
			models = provider.VModelDetail.Models
		}
		return finalize(models, ModelListSourceVModel), nil
	}

	// Step 3: Upstream API (persisted on success).
	if err := c.fetchAndSaveAPIModels(provider, forceUpstream); err == nil {
		if fresh := c.modelManager.GetModels(uid); len(fresh) > 0 {
			return finalize(fresh, ModelListSourceAPI), nil
		}
	}

	// Step 4: Embedded template fallback (live, never persisted). It is a
	// last resort only — never merged into a non-empty real list, since the
	// snapshot can still name models the upstream has retired.
	if c.templateManager != nil {
		if tmpl, err := c.templateManager.GetEmbeddedModelsForProvider(provider); err == nil && len(tmpl) > 0 {
			return finalize(tmpl, ModelListSourceTemplate), nil
		}
	}

	return finalize(nil, ModelListSourceTemplate), nil
}

// fetchAndSaveAPIModels queries the provider's upstream /models endpoint and,
// on success, persists the sorted list (ModelSourceAPI). It returns an error
// when the endpoint is unsupported or the call fails; the caller falls back to
// the template. It never persists template data.
func (c *Config) fetchAndSaveAPIModels(provider *typ.Provider, forceUpstream bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	lister, err := c.newModelLister(ctx, provider)
	if err != nil || lister == nil {
		logrus.Errorf("Failed to create client for provider %s: %v", provider.Name, err)
		return fmt.Errorf("failed to create client for provider %s: %w", provider.Name, err)
	}
	defer lister.Close()

	var result *client.ModelListResult
	var apiErr error
	if provider.IsClaudeCodeProvider() && !forceUpstream {
		apiErr = errors.New("model listing from Claude Code upstream is disabled")
	} else {
		result, apiErr = lister.ListModels(ctx)
	}

	if apiErr != nil {
		// Unsupported endpoints are expected but still useful during triage.
		if persistErr := c.modelManager.SaveFetchFailure(provider, apiErr.Error(), modelListRaw(result)); persistErr != nil {
			logrus.Warnf("Failed to persist model fetch failure for %s: %v", provider.Name, persistErr)
		}

		logrus.Errorf("Failed to fetch models from API: %v", apiErr)

		return apiErr
	}

	if result == nil || len(result.Models) == 0 {
		errMsg := fmt.Sprintf("provider %s returned no models", provider.Name)
		if persistErr := c.modelManager.SaveFetchFailure(provider, errMsg, modelListRaw(result)); persistErr != nil {
			logrus.Warnf("Failed to persist model fetch failure for %s: %v", provider.Name, persistErr)
		}
		return errors.New(errMsg)
	}

	// Persist the upstream list verbatim. It is authoritative for both
	// additions and removals, so the embedded template snapshot must not be
	// merged in here — that would resurrect retired models. Apply canonical
	// ordering before persisting; the same sort is reapplied at the serving
	// boundary so cached order is irrelevant. Raw is the genuine upstream
	// payload, marshalled for persistence/triage.
	SortProviderModels(provider, result.Models)
	return c.modelManager.SaveModelsWithRaw(provider, result.Models, db.ModelSourceAPI, marshalRaw(result.Raw))
}

func modelListRaw(result *client.ModelListResult) json.RawMessage {
	if result == nil {
		return nil
	}
	return marshalRaw(result.Raw)
}

// marshalRaw serializes a client's raw upstream payload (an SDK response
// struct, a json.RawMessage body, etc.) to bytes for persistence. nil and
// already-raw values pass through. A marshal error yields nil rather than
// failing the model save — the parsed Models list is still authoritative.
func marshalRaw(raw any) json.RawMessage {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case json.RawMessage:
		return v
	case []byte:
		return v
	case string:
		return []byte(v)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	return b
}

// FetchAndSaveProviderModels fetches a provider's models and persists API
// results, warming the DB cache. It is retained for callers that only need to
// trigger a fetch (e.g. TUI cache-warming); callers that need the effective
// list should use ResolveProviderModels instead.
//
// Contract: returns nil when a real API list was persisted OR when the API is
// unavailable but an embedded template exists to fall back on; returns an
// error only when neither a real nor a template list can be produced.
func (c *Config) FetchAndSaveProviderModels(uid string) error {
	provider, err := c.GetProviderByUUID(uid)
	if err != nil {
		return fmt.Errorf("provider with UUID %s not found: %w", uid, err)
	}

	// Vmodel providers store their model list on the provider record itself.
	if provider.IsVirtual() {
		var models []string
		if provider.VModelDetail != nil {
			models = provider.VModelDetail.Models
		}
		return c.modelManager.SaveModels(provider, models, db.ModelSourceAPI)
	}

	apiErr := c.fetchAndSaveAPIModels(provider, false)
	if apiErr == nil {
		return nil
	}

	// API failed or not supported — success only if a template can cover it.
	if c.templateManager != nil {
		if tmpl, tErr := c.templateManager.GetEmbeddedModelsForProvider(provider); tErr == nil && len(tmpl) > 0 {
			return nil
		}
	}
	return fmt.Errorf("failed to fetch models (API: %v, template fallback: not available)", apiErr)
}

// newModelLister builds the provider-specific client used to list models.
// ClientPool handles OAuth issuer dispatch; DeepSeek is the only endpoint
// override because its model list lives on the bare host rather than /v1.
//
//   - DeepSeek exposes its model list only on the bare host (no /v1 path), so
//     we override APIBase to https://api.deepseek.com before constructing the
//     OpenAI client.
func (c *Config) newModelLister(ctx context.Context, provider *typ.Provider) (client.ModelLister, error) {
	host, _ := ops.SplitProviderHostPath(provider.APIBase)
	switch host {
	case "api.deepseek.com":
		providerForModels := *provider
		providerForModels.APIBase = "https://api.deepseek.com"
		oClient, err := client.NewOpenAIClient(&providerForModels, "", typ.SessionID{})
		if err != nil {
			return nil, err
		}
		return oClient, nil
	}

	pool := client.NewClientPool()
	var lister client.ModelLister
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		lister = pool.GetAnthropicClient(ctx, provider, "")
	case protocol.APIStyleGoogle:
		lister = pool.GetGoogleClient(ctx, provider, "")
	case protocol.APIStyleOpenAI:
		fallthrough
	default:
		lister = pool.GetOpenAIClient(ctx, provider, "")
	}
	if lister == nil {
		return nil, fmt.Errorf("failed to create model-list client for provider %s", provider.Name)
	}
	return lister, nil
}

// SortProviderModels applies the canonical display ordering to a provider's
// model list in place. It is the single source of truth for model ordering
// served to clients — callers (fetch + serving boundaries) invoke it instead
// of relying on the frontend to sort.
//
// Default: alphabetical by model id. OpenRouter additionally tags free models
// with a ":free" suffix; those are promoted to the front, sorted alphabetically
// among themselves and ahead of the paid models (which are also alphabetical).
func SortProviderModels(provider *typ.Provider, models []string) {
	if provider == nil || len(models) < 2 {
		return
	}
	promoteFree := isOpenRouterProvider(provider)
	slices.SortStableFunc(models, func(a, b string) int {
		if promoteFree {
			aFree := strings.HasSuffix(a, ":free")
			bFree := strings.HasSuffix(b, ":free")
			if aFree != bFree {
				if aFree {
					return -1
				}
				return 1
			}
		}
		return strings.Compare(a, b)
	})
}

// isOpenRouterProvider reports whether the provider routes to OpenRouter.
// OpenRouter's canonical host is openrouter.ai; the base_url templates embed it
// as https://openrouter.ai/api/v1 (OpenAI) and https://openrouter.ai/api (Anthropic).
func isOpenRouterProvider(provider *typ.Provider) bool {
	for _, base := range []string{provider.APIBase, provider.APIBaseOpenAI, provider.APIBaseAnthropic} {
		host, _ := ops.SplitProviderHostPath(base)
		if host == "openrouter.ai" {
			return true
		}
	}
	return false
}

func (c *Config) GetModelManager() *catalog.ModelListManager {
	return c.modelManager
}

// SetTemplateManager sets the template manager for provider templates
func (c *Config) SetTemplateManager(tm *catalog.ProviderCatalogManager) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.templateManager = tm
}

// GetTemplateManager returns the template manager
func (c *Config) GetTemplateManager() *catalog.ProviderCatalogManager {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.templateManager
}
