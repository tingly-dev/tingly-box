package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
	guardrailspipeline "github.com/tingly-dev/tingly-box/internal/guardrails/pipeline"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

var guardrailsSupportedScenarios = []string{
	string(typ.ScenarioAnthropic),
	string(typ.ScenarioClaudeCode),
}

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
	runtime := s.currentGuardrailsRuntime()
	if runtime == nil || runtime.PolicyEngine() == nil || s.config == nil || !runtime.IsActive() {
		return false
	}
	if !s.guardrailsSupportsScenario(scenario) {
		return false
	}
	return s.config.GetScenarioFlag(typ.RuleScenario(scenario), "guardrails") ||
		s.config.GetScenarioFlag(typ.ScenarioGlobal, "guardrails")
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

func (s *Server) guardrailsSupportsScenario(scenario string) bool {
	for _, supported := range guardrailsSupportedScenarios {
		if scenario == supported {
			return true
		}
	}
	return false
}

func (s *Server) getGuardrailsSupportedScenarios() []string {
	out := make([]string, len(guardrailsSupportedScenarios))
	copy(out, guardrailsSupportedScenarios)
	return out
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

	store, err := s.guardrailsCredentialStore()
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

	cfgPath, err := FindGuardrailsConfig(s.config.ConfigDir)
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

func ensureGuardrailsCredentialMaskState(c *gin.Context) *guardrailscore.CredentialMaskState {
	if c == nil {
		return nil
	}
	if existing, ok := c.Get(guardrailscore.CredentialMaskStateContextKey); ok {
		if state, ok := existing.(*guardrailscore.CredentialMaskState); ok && state != nil {
			return state
		}
	}
	state := guardrailscore.NewCredentialMaskState()
	c.Set(guardrailscore.CredentialMaskStateContextKey, state)
	return state
}

// ----------------------------------------------------------------------
// Shared Input Construction
// ----------------------------------------------------------------------

// buildGuardrailsBaseInput creates the shared evaluation envelope; adapters can then
// add request/response-specific content without rebuilding metadata each time.
func (s *Server) buildGuardrailsBaseInput(c *gin.Context, actualModel string, provider *typ.Provider, direction guardrailscore.Direction, messages []guardrailscore.Message) guardrailscore.Input {
	_, _, _, requestModel, scenario, _, _ := GetTrackingContext(c)
	providerName := ""
	if provider != nil {
		providerName = provider.Name
	}
	return guardrailscore.Input{
		Scenario:  scenario,
		Model:     actualModel,
		Direction: direction,
		Content: guardrailscore.Content{
			Messages: messages,
		},
		Metadata: map[string]interface{}{
			"provider":      providerName,
			"request_model": requestModel,
		},
		Runtime: guardrailscore.InputRuntime{
			Context: c,
		},
	}
}

// ----------------------------------------------------------------------
// Stream Response Guardrails
// ----------------------------------------------------------------------

// attachGuardrailsHooks wires the shared stream guardrails runtime into a protocol
// handle context. Provider-specific handlers only need to provide already-normalized
// message history.
func (s *Server) attachGuardrailsHooks(c *gin.Context, hc *protocol.HandleContext, actualModel string, provider *typ.Provider, messages []guardrailscore.Message) {
	guardrailsState := hc.EnsureGuardrails()
	baseInput := s.buildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionResponse, messages)
	maskState := ensureGuardrailsCredentialMaskState(c)
	baseInput.State.CredentialMask = maskState
	guardrailsState.Enabled = true
	guardrailsState.CredentialMask = baseInput.State.CredentialMask
	streamState := hc.EnsureGuardrailsStream()
	logrus.Debugf("Guardrails: attaching hook (scenario=%s model=%s)", baseInput.Scenario, baseInput.Model)

	onEvent, onError := guardrailspipeline.NewGuardrailsHooks(
		c.Request.Context(),
		s.currentGuardrailsRuntime(),
		baseInput,
		streamState,
	)
	if onEvent != nil {
		hc.WithOnStreamEvent(onEvent)
	}
	if onError != nil {
		hc.WithOnStreamError(onError)
	}
}

// ----------------------------------------------------------------------
// Request Guardrails
// ----------------------------------------------------------------------

// applyGuardrailsToAnthropicV1Request is the merged request-side entry for
// Anthropic v1 requests. It runs request tool_result filtering first and then
// request credential masking on the latest raw request state.
func (s *Server) applyGuardrailsToAnthropicV1Request(c *gin.Context, req *anthropic.MessageNewParams, actualModel string, provider *typ.Provider) {
	if req == nil {
		return
	}

	input := s.buildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionRequest, nil)
	input.State.CredentialMask = ensureGuardrailsCredentialMaskState(c)
	input.Payload.Protocol = "anthropic_v1"
	input.Payload.Request = req

	err := guardrailspipeline.ProcessAnthropicV1Request(
		c.Request.Context(),
		s.currentGuardrailsRuntime(),
		input,
	)
	if err != nil {
		return
	}
}

// applyGuardrailsToAnthropicV1BetaRequest is the merged request-side entry for
// Anthropic beta requests. It runs request tool_result filtering first and then
// request credential masking on the latest raw request state.
func (s *Server) applyGuardrailsToAnthropicV1BetaRequest(c *gin.Context, req *anthropic.BetaMessageNewParams, actualModel string, provider *typ.Provider) {
	if req == nil {
		return
	}

	input := s.buildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionRequest, nil)
	input.State.CredentialMask = ensureGuardrailsCredentialMaskState(c)
	input.Payload.Protocol = "anthropic_beta"
	input.Payload.Request = req

	err := guardrailspipeline.ProcessAnthropicBetaRequest(
		c.Request.Context(),
		s.currentGuardrailsRuntime(),
		input,
	)
	if err != nil {
		return
	}
}

// ----------------------------------------------------------------------
// Non-Stream Response Guardrails
// ----------------------------------------------------------------------

// applyGuardrailsToAnthropicV1NonStreamResponse evaluates a fully assembled
// Anthropic v1 response and rewrites it when guardrails block it.
func (s *Server) applyGuardrailsToAnthropicV1NonStreamResponse(c *gin.Context, req *anthropic.MessageNewParams, actualModel string, provider *typ.Provider, resp *anthropic.Message) bool {
	if req == nil || resp == nil {
		return false
	}

	maskState := ensureGuardrailsCredentialMaskState(c)
	messageHistory := guardrailsadapter.AdaptMessagesFromAnthropicV1(req.System, req.Messages)
	input := s.buildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionResponse, messageHistory)
	input.State.CredentialMask = maskState
	input.Payload.Protocol = "anthropic_v1"
	input.Payload.Response = resp

	mutation, err := guardrailspipeline.ProcessAnthropicV1NonStreamResponse(c.Request.Context(), s.currentGuardrailsRuntime(), input, resp)
	if err != nil {
		return false
	}
	if !mutation.Changed {
		guardrailsmutate.RestoreAnthropicV1ResponseCredentials(maskState, resp)
	}
	return mutation.Changed
}

// applyGuardrailsToAnthropicV1BetaNonStreamResponse is the beta equivalent of
// applyGuardrailsToAnthropicV1NonStreamResponse.
func (s *Server) applyGuardrailsToAnthropicV1BetaNonStreamResponse(c *gin.Context, req *anthropic.BetaMessageNewParams, actualModel string, provider *typ.Provider, resp *anthropic.BetaMessage) bool {
	if req == nil || resp == nil {
		return false
	}

	maskState := ensureGuardrailsCredentialMaskState(c)
	messageHistory := guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages)
	input := s.buildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionResponse, messageHistory)
	input.State.CredentialMask = maskState
	input.Payload.Protocol = "anthropic_beta"
	input.Payload.Response = resp

	mutation, err := guardrailspipeline.ProcessAnthropicV1BetaNonStreamResponse(c.Request.Context(), s.currentGuardrailsRuntime(), input, resp)
	if err != nil {
		return false
	}
	if !mutation.Changed {
		guardrailsmutate.RestoreAnthropicV1BetaResponseCredentials(maskState, resp)
	}
	return mutation.Changed
}
