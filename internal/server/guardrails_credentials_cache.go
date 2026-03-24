package server

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	serverguardrails "github.com/tingly-dev/tingly-box/internal/server/guardrails"
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
	built := serverguardrails.BuildCredentialCache(guardrails.Config{}, credentials, s.getGuardrailsSupportedScenarios())
	next.byID = built.ByID
	next.byScenario = built.ByScenario

	if !s.guardrailsEnabled() {
		s.storeGuardrailsCredentialCache(next)
		return nil
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
	byID := s.guardrailsCredentialCache.byID
	s.guardrailsCredentialCacheMu.RUnlock()
	return serverguardrails.ResolveCredentialNames(byID, ids)
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
