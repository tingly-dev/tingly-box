package serverguardrails

import (
	"sort"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

type CredentialCache struct {
	ByScenario map[string][]guardrails.ProtectedCredential
	ByID       map[string]guardrails.ProtectedCredential
}

func NewCredentialCache() CredentialCache {
	return CredentialCache{
		ByScenario: make(map[string][]guardrails.ProtectedCredential),
		ByID:       make(map[string]guardrails.ProtectedCredential),
	}
}

func BuildCredentialCache(cfg guardrails.Config, credentials []guardrails.ProtectedCredential, scenarios []string) CredentialCache {
	cache := NewCredentialCache()
	for _, credential := range credentials {
		cache.ByID[credential.ID] = credential
	}

	enabledCredentials := make(map[string]guardrails.ProtectedCredential)
	for _, credential := range credentials {
		if credential.Enabled {
			enabledCredentials[credential.ID] = credential
		}
	}

	for _, scenario := range scenarios {
		refs := collectMaskCredentialRefs(cfg, scenario)
		if len(refs) == 0 {
			continue
		}
		resolved := make([]guardrails.ProtectedCredential, 0, len(refs))
		seen := make(map[string]struct{}, len(refs))
		for _, ref := range refs {
			credential, ok := enabledCredentials[ref]
			if !ok {
				continue
			}
			if _, ok := seen[credential.ID]; ok {
				continue
			}
			seen[credential.ID] = struct{}{}
			resolved = append(resolved, credential)
		}
		if len(resolved) == 0 {
			continue
		}

		// Longer secrets must be aliased first so a shorter secret cannot partially
		// consume the prefix of a longer one during request-side replacement.
		sort.Slice(resolved, func(i, j int) bool {
			return len(resolved[i].Secret) > len(resolved[j].Secret)
		})
		cache.ByScenario[scenario] = resolved
	}

	return cache
}

func ResolveCredentialNames(byID map[string]guardrails.ProtectedCredential, ids []string) []string {
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

func collectMaskCredentialRefs(cfg guardrails.Config, scenario string) []string {
	if len(cfg.Policies) == 0 {
		return nil
	}
	groupByID := make(map[string]guardrails.PolicyGroup, len(cfg.Groups))
	for _, group := range cfg.Groups {
		groupByID[group.ID] = group
	}
	refs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, policy := range cfg.Policies {
		if policy.Kind != guardrails.PolicyKindContent {
			continue
		}
		group, hasGroup := groupByID[policy.Group]
		if hasGroup && group.Enabled != nil && !*group.Enabled {
			continue
		}
		if policy.Enabled != nil && !*policy.Enabled {
			continue
		}
		verdict := policy.Verdict
		if verdict == "" && hasGroup {
			verdict = group.DefaultVerdict
		}
		if verdict != guardrails.VerdictMask {
			continue
		}
		scenarios := policy.Scope.Scenarios
		if len(scenarios) == 0 && hasGroup {
			scenarios = group.DefaultScope.Scenarios
		}
		if len(scenarios) > 0 && !containsString(scenarios, scenario) {
			continue
		}
		for _, ref := range policy.Match.CredentialRefs {
			if ref == "" {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			refs = append(refs, ref)
		}
	}
	return refs
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
