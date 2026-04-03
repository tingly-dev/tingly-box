package guardrails

import (
	"context"
	"errors"
	"sort"
	"sync"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
)

var ErrNoPolicyEngine = errors.New("guardrails runtime has no policy engine")

// PolicyRunner is the runtime-owned evaluation surface used by Guardrails.
type PolicyRunner interface {
	Evaluate(ctx context.Context, input guardrailscore.Input) (guardrailscore.Result, error)
}

type CredentialCache struct {
	ByScenario map[string][]guardrailscore.ProtectedCredential
	ByID       map[string]guardrailscore.ProtectedCredential
}

func NewCredentialCache() CredentialCache {
	return CredentialCache{
		ByScenario: make(map[string][]guardrailscore.ProtectedCredential),
		ByID:       make(map[string]guardrailscore.ProtectedCredential),
	}
}

func BuildCredentialCache(credentials []guardrailscore.ProtectedCredential, scenarios []string) CredentialCache {
	cache := NewCredentialCache()
	for _, credential := range credentials {
		cache.ByID[credential.ID] = credential
	}

	enabledCredentials := make([]guardrailscore.ProtectedCredential, 0, len(credentials))
	for _, credential := range credentials {
		if credential.Enabled {
			enabledCredentials = append(enabledCredentials, credential)
		}
	}
	if len(enabledCredentials) == 0 {
		return cache
	}

	sort.Slice(enabledCredentials, func(i, j int) bool {
		return len(enabledCredentials[i].Secret) > len(enabledCredentials[j].Secret)
	})
	for _, scenario := range scenarios {
		resolved := make([]guardrailscore.ProtectedCredential, len(enabledCredentials))
		copy(resolved, enabledCredentials)
		cache.ByScenario[scenario] = resolved
	}
	return cache
}

func ResolveCredentialNames(byID map[string]guardrailscore.ProtectedCredential, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}

	names := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		credential, ok := byID[id]
		if !ok || credential.Name == "" {
			continue
		}
		if _, ok := seen[credential.Name]; ok {
			continue
		}
		seen[credential.Name] = struct{}{}
		names = append(names, credential.Name)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return names
}

// Guardrails is the shared runtime entry point. It can grow to hold more
// runtime-owned state, while the Policy field remains the evaluation-only component.
type Guardrails struct {
	Policy            PolicyRunner
	HasActivePolicies bool
	CredentialCache   CredentialCache
	History           *guardrailsutils.Store
	Config            guardrailscore.Config

	mu sync.RWMutex
}

// Evaluate delegates to the configured policy engine when present.
func (g *Guardrails) Evaluate(ctx context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
	if g == nil {
		return guardrailscore.Result{}, ErrNoPolicyEngine
	}
	g.mu.RLock()
	policy := g.Policy
	g.mu.RUnlock()
	if policy == nil {
		return guardrailscore.Result{}, ErrNoPolicyEngine
	}
	return policy.Evaluate(ctx, input)
}

func (g *Guardrails) SetCredentialCache(cache CredentialCache) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.CredentialCache = cache
	g.mu.Unlock()
}

func (g *Guardrails) CredentialMaskCredentials(scenario string) []guardrailscore.ProtectedCredential {
	if g == nil || scenario == "" {
		return nil
	}
	g.mu.RLock()
	cached := g.CredentialCache.ByScenario[scenario]
	g.mu.RUnlock()
	if len(cached) == 0 {
		return nil
	}
	out := make([]guardrailscore.ProtectedCredential, len(cached))
	copy(out, cached)
	return out
}

func (g *Guardrails) CredentialNames(ids []string) []string {
	if g == nil || len(ids) == 0 {
		return nil
	}
	g.mu.RLock()
	byID := g.CredentialCache.ByID
	g.mu.RUnlock()
	return ResolveCredentialNames(byID, ids)
}
