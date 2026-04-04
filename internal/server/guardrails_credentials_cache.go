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

// refreshGuardrailsCredentialCache rebuilds the in-memory credential view used by
// request masking and history rendering. It is intentionally cheap to read and is
// refreshed after config or credential mutations.
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

// storeGuardrailsCredentialCache swaps the cache atomically under the server lock.
func (s *Server) storeGuardrailsCredentialCache(cache guardrailsCredentialCache) {
	s.guardrailsCredentialCacheMu.Lock()
	s.guardrailsCredentialCache = cache
	s.guardrailsCredentialCacheMu.Unlock()
}

// getCachedGuardrailsMaskCredentials returns a copy of the active protected
// credentials for the given scenario so callers can read without holding locks.
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

// getCachedGuardrailsCredentialNames resolves ids to display names from the same
// cache used by request masking.
func (s *Server) getCachedGuardrailsCredentialNames(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	s.guardrailsCredentialCacheMu.RLock()
	byID := s.guardrailsCredentialCache.byID
	s.guardrailsCredentialCacheMu.RUnlock()
	return serverguardrails.ResolveCredentialNames(byID, ids)
}

// refreshGuardrailsCredentialCacheOrWarn keeps cache refresh failures out of the
// main request path while still surfacing them in logs.
func (s *Server) refreshGuardrailsCredentialCacheOrWarn(context string) {
	if err := s.refreshGuardrailsCredentialCache(); err != nil {
		logrus.WithError(err).Warnf("Guardrails credential cache refresh failed after %s", context)
	}
}

func (s *Server) refreshGuardrailsActivationState() {
	s.guardrailsHasActivePolicies = false
	if s.config == nil || s.config.ConfigDir == "" {
		return
	}

	cfgPath, err := serverguardrails.FindGuardrailsConfig(s.config.ConfigDir)
	if err != nil {
		return
	}

	cfg, err := guardrails.LoadConfig(cfgPath)
	if err != nil {
		logrus.WithError(err).Debug("Guardrails activation state: failed to load config")
		return
	}

	s.guardrailsHasActivePolicies = hasActiveGuardrailsPolicies(cfg)
}

func (s *Server) setGuardrailsEngine(engine guardrails.Guardrails, context string) {
	s.guardrailsEngine = engine
	if engine == nil {
		s.guardrailsHasActivePolicies = false
	} else {
		s.refreshGuardrailsActivationState()
	}
	s.refreshGuardrailsCredentialCacheOrWarn(context)
}
