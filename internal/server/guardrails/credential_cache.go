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

func BuildCredentialCache(_ guardrails.Config, credentials []guardrails.ProtectedCredential, scenarios []string) CredentialCache {
	cache := NewCredentialCache()
	for _, credential := range credentials {
		cache.ByID[credential.ID] = credential
	}

	enabledCredentials := make([]guardrails.ProtectedCredential, 0, len(credentials))
	for _, credential := range credentials {
		if credential.Enabled {
			enabledCredentials = append(enabledCredentials, credential)
		}
	}

	if len(enabledCredentials) == 0 {
		return cache
	}

	// Longer secrets must be aliased first so a shorter secret cannot partially
	// consume the prefix of a longer one during request-side replacement.
	sort.Slice(enabledCredentials, func(i, j int) bool {
		return len(enabledCredentials[i].Secret) > len(enabledCredentials[j].Secret)
	})
	for _, scenario := range scenarios {
		resolved := make([]guardrails.ProtectedCredential, len(enabledCredentials))
		copy(resolved, enabledCredentials)
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
