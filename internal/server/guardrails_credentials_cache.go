package server

import (
	"os"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

type guardrailsCredentialCache struct {
	byScenario map[string][]guardrails.ProtectedCredential
	byID       map[string]guardrails.ProtectedCredential
}

func (s *Server) refreshGuardrailsCredentialCache() error {
	next := guardrailsCredentialCache{
		byScenario: make(map[string][]guardrails.ProtectedCredential),
		byID:       make(map[string]guardrails.ProtectedCredential),
	}
	if s.config == nil || s.config.ConfigDir == "" {
		s.storeGuardrailsCredentialCache(next)
		return nil
	}

	store, err := s.guardrailsCredentialStore()
	if err != nil {
		return err
	}
	credentials, err := store.List()
	if err != nil {
		return err
	}
	for _, credential := range credentials {
		next.byID[credential.ID] = credential
	}

	if !s.guardrailsEnabled() {
		s.storeGuardrailsCredentialCache(next)
		return nil
	}

	configPath, err := findGuardrailsConfig(s.config.ConfigDir)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no guardrails config") {
			s.storeGuardrailsCredentialCache(next)
			return nil
		}
		return err
	}
	cfg, err := guardrails.LoadConfig(configPath)
	if err != nil {
		return err
	}

	enabledCredentials := make(map[string]guardrails.ProtectedCredential)
	for _, credential := range credentials {
		if credential.Enabled {
			enabledCredentials[credential.ID] = credential
		}
	}

	for _, scenario := range s.getGuardrailsSupportedScenarios() {
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
		if len(resolved) > 0 {
			sort.Slice(resolved, func(i, j int) bool {
				return len(resolved[i].Secret) > len(resolved[j].Secret)
			})
			next.byScenario[scenario] = resolved
		}
	}

	s.storeGuardrailsCredentialCache(next)
	return nil
}

func (s *Server) storeGuardrailsCredentialCache(cache guardrailsCredentialCache) {
	s.guardrailsCredentialCacheMu.Lock()
	s.guardrailsCredentialCache = cache
	s.guardrailsCredentialCacheMu.Unlock()
}

func (s *Server) getCachedGuardrailsMaskCredentials(scenario string) []guardrails.ProtectedCredential {
	s.guardrailsCredentialCacheMu.RLock()
	cached := s.guardrailsCredentialCache.byScenario[scenario]
	s.guardrailsCredentialCacheMu.RUnlock()
	if len(cached) == 0 {
		return nil
	}
	out := make([]guardrails.ProtectedCredential, len(cached))
	copy(out, cached)
	return out
}

func (s *Server) getCachedGuardrailsCredentialNames(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	s.guardrailsCredentialCacheMu.RLock()
	defer s.guardrailsCredentialCacheMu.RUnlock()

	names := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		credential, ok := s.guardrailsCredentialCache.byID[id]
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

func (s *Server) refreshGuardrailsCredentialCacheOrWarn(context string) {
	if err := s.refreshGuardrailsCredentialCache(); err != nil {
		logrus.WithError(err).Warnf("Guardrails credential cache refresh failed after %s", context)
	}
}

func (s *Server) setGuardrailsEngine(engine guardrails.Guardrails, context string) {
	s.guardrailsEngine = engine
	s.refreshGuardrailsCredentialCacheOrWarn(context)
}
