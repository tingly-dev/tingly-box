package server

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/server/config"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// This file is the single source of truth for the guardrails runtime pointer
// itself: the mutex-guarded swap/read primitives, and the admin-facing
// lifecycle operations (activation refresh, credential cache refresh) driven
// by config edits, hot-reload, and NewServer construction. It stays in root
// because both webui's admin handlers (via guardrails_runtime_adapter.go)
// and root's own lifecycle code (server.go, server_flags.go, server_options.go)
// need it.
//
// The gateway-facing half — building the evaluation envelope and applying
// guardrails during a live model request — lives in
// aimodel/guardrails_runtime.go, since every caller of that half is an
// ai-gateway file. Those functions take the current runtime snapshot as an
// explicit parameter (via currentGuardrailsRuntime() below) rather than
// reaching into shared state directly.

func (s *Server) currentGuardrailsRuntime() *guardrails.Guardrails {
	if s == nil {
		return nil
	}
	s.guardrailsRuntimeMu.RLock()
	runtime := s.guardrailsRuntime
	s.guardrailsRuntimeMu.RUnlock()
	return runtime
}

func (s *Server) setGuardrailsRuntimeRef(runtime *guardrails.Guardrails) {
	if s == nil {
		return
	}
	s.guardrailsRuntimeMu.Lock()
	s.guardrailsRuntime = runtime
	s.guardrailsRuntimeMu.Unlock()
}

func cloneGuardrailsRuntime(src *guardrails.Guardrails) *guardrails.Guardrails {
	if src == nil {
		return nil
	}
	cloned := &guardrails.Guardrails{}
	cloned.SetPolicyEngine(src.PolicyEngine())
	cloned.SetHistoryStore(src.HistoryStore())
	cloned.SetCredentialCache(src.CredentialCacheSnapshot())
	cloned.SetActivation(src.ConfigSnapshot(), src.IsActive())
	return cloned
}

// ----------------------------------------------------------------------
// Runtime Gate And Shared State
// ----------------------------------------------------------------------

// guardrailsEnabledForScenario centralizes feature-flag checks so protocol handlers
// do not repeat scenario/global guardrails gating logic.
func (s *Server) guardrailsEnabledForScenario(scenario string) bool {
	return GuardrailsEnabledForScenario(s.config, s.currentGuardrailsRuntime(), scenario)
}

func (s *Server) guardrailsSupportsScenario(scenario string) bool {
	return GuardrailsSupportsScenario(scenario)
}

func (s *Server) getGuardrailsSupportedScenarios() []string {
	out := make([]string, len(GuardrailsSupportedScenarios))
	copy(out, GuardrailsSupportedScenarios)
	return out
}

func hasActiveGuardrailsPolicies(cfg guardrailscore.Config) bool {
	if len(cfg.Policies) == 0 || len(cfg.Groups) == 0 {
		return false
	}

	enabledGroups := make(map[string]struct{}, len(cfg.Groups))
	for _, group := range cfg.Groups {
		if !group.Enabled {
			continue
		}
		enabledGroups[group.ID] = struct{}{}
	}
	if len(enabledGroups) == 0 {
		return false
	}

	for _, policy := range cfg.Policies {
		if !policy.Enabled {
			continue
		}
		for _, groupID := range policy.Groups {
			if _, ok := enabledGroups[groupID]; ok {
				return true
			}
		}
	}
	return false
}

// Credential cache and activation state live alongside the runtime gate because
// they are shared by request masking, history rendering, and runtime reloads.
func (s *Server) refreshGuardrailsCredentialCache() error {
	runtime := s.currentGuardrailsRuntime()
	if runtime == nil {
		return nil
	}
	if s.config == nil || s.config.ConfigDir == "" {
		next := cloneGuardrailsRuntime(runtime)
		next.SetCredentialCache(guardrails.NewCredentialCache())
		s.setGuardrailsRuntimeRef(next)
		return nil
	}

	store, err := config.CredentialStore(s.config.ConfigDir)
	if err != nil {
		return err
	}
	credentials, err := store.List()
	if err != nil {
		return err
	}
	built := guardrails.BuildCredentialCache(credentials, s.getGuardrailsSupportedScenarios())
	next := cloneGuardrailsRuntime(runtime)
	next.SetCredentialCache(built)
	s.setGuardrailsRuntimeRef(next)
	return nil
}

func (s *Server) refreshGuardrailsCredentialCacheOrWarn(context string) {
	if err := s.refreshGuardrailsCredentialCache(); err != nil {
		logrus.WithError(err).Warnf("Guardrails credential cache refresh failed after %s", context)
	}
}

func (s *Server) refreshGuardrailsActivationState() {
	runtime := s.currentGuardrailsRuntime()
	if runtime == nil {
		return
	}

	nextCfg := guardrailscore.Config{}
	nextActive := false
	if s.config == nil || s.config.ConfigDir == "" {
		next := cloneGuardrailsRuntime(runtime)
		next.SetActivation(nextCfg, nextActive)
		s.setGuardrailsRuntimeRef(next)
		return
	}

	cfgPath, err := config.FindConfig(s.config.ConfigDir)
	if err != nil {
		return
	}

	cfg, err := guardrails.LoadConfig(cfgPath)
	if err != nil {
		logrus.WithError(err).Debug("Guardrails activation state: failed to load config")
		next := cloneGuardrailsRuntime(runtime)
		next.SetActivation(nextCfg, nextActive)
		s.setGuardrailsRuntimeRef(next)
		return
	}
	nextCfg = cfg
	nextActive = hasActiveGuardrailsPolicies(cfg)
	next := cloneGuardrailsRuntime(runtime)
	next.SetActivation(nextCfg, nextActive)
	s.setGuardrailsRuntimeRef(next)
}

func (s *Server) setGuardrailsRuntime(runtime *guardrails.Guardrails, context string) {
	prev := s.currentGuardrailsRuntime()
	if runtime != nil && prev != nil {
		if runtime.HistoryStore() == nil {
			runtime.SetHistoryStore(prev.HistoryStore())
		}
		cache := runtime.CredentialCacheSnapshot()
		if len(cache.ByID) == 0 && len(cache.ByScenario) == 0 {
			runtime.SetCredentialCache(prev.CredentialCacheSnapshot())
		}
	}
	s.setGuardrailsRuntimeRef(runtime)
	if runtime != nil {
		s.refreshGuardrailsActivationState()
		s.refreshGuardrailsCredentialCacheOrWarn(context)
	}
}
