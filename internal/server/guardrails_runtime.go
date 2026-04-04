package server

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailspipeline "github.com/tingly-dev/tingly-box/internal/guardrails/pipeline"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

var guardrailsSupportedScenarios = []string{
	string(typ.ScenarioAnthropic),
	string(typ.ScenarioClaudeCode),
}

// guardrailsEnabledForScenario centralizes feature-flag checks so protocol handlers
// do not repeat scenario/global guardrails gating logic.
func (s *Server) guardrailsEnabledForScenario(scenario string) bool {
	if s.guardrailsRuntime == nil || s.guardrailsRuntime.Policy == nil || s.config == nil || !s.guardrailsRuntime.HasActivePolicies {
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
		if group.Enabled != nil && !*group.Enabled {
			continue
		}
		enabledGroups[group.ID] = struct{}{}
	}
	if len(enabledGroups) == 0 {
		return false
	}

	for _, policy := range cfg.Policies {
		if policy.Enabled != nil && !*policy.Enabled {
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
	if s.guardrailsRuntime == nil || s.config == nil || s.config.ConfigDir == "" {
		if s.guardrailsRuntime != nil {
			s.guardrailsRuntime.SetCredentialCache(guardrails.NewCredentialCache())
		}
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
	s.guardrailsRuntime.SetCredentialCache(built)
	return nil
}

func (s *Server) getCachedGuardrailsMaskCredentials(scenario string) []guardrailscore.ProtectedCredential {
	if s.guardrailsRuntime == nil {
		return nil
	}
	return s.guardrailsRuntime.CredentialMaskCredentials(scenario)
}

func (s *Server) getCachedGuardrailsCredentialNames(ids []string) []string {
	if s.guardrailsRuntime == nil || len(ids) == 0 {
		return nil
	}
	return s.guardrailsRuntime.CredentialNames(ids)
}

func (s *Server) refreshGuardrailsCredentialCacheOrWarn(context string) {
	if err := s.refreshGuardrailsCredentialCache(); err != nil {
		logrus.WithError(err).Warnf("Guardrails credential cache refresh failed after %s", context)
	}
}

func (s *Server) refreshGuardrailsActivationState() {
	if s.guardrailsRuntime == nil {
		return
	}
	s.guardrailsRuntime.HasActivePolicies = false
	s.guardrailsRuntime.Config = guardrailscore.Config{}
	if s.config == nil || s.config.ConfigDir == "" {
		return
	}

	cfgPath, err := FindGuardrailsConfig(s.config.ConfigDir)
	if err != nil {
		return
	}

	cfg, err := guardrails.LoadConfig(cfgPath)
	if err != nil {
		logrus.WithError(err).Debug("Guardrails activation state: failed to load config")
		return
	}

	s.guardrailsRuntime.Config = cfg
	s.guardrailsRuntime.HasActivePolicies = hasActiveGuardrailsPolicies(cfg)
}

func (s *Server) setGuardrailsRuntime(runtime *guardrails.Guardrails, context string) {
	if runtime != nil && s.guardrailsRuntime != nil {
		if runtime.History == nil {
			runtime.History = s.guardrailsRuntime.History
		}
		if len(runtime.CredentialCache.ByID) == 0 && len(runtime.CredentialCache.ByScenario) == 0 {
			runtime.CredentialCache = s.guardrailsRuntime.CredentialCache
		}
	}
	s.guardrailsRuntime = runtime
	if runtime != nil {
		s.refreshGuardrailsActivationState()
		s.refreshGuardrailsCredentialCacheOrWarn(context)
	}
}

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

// attachGuardrailsHooks wires the shared stream guardrails runtime into a protocol
// handle context. Provider-specific handlers only need to provide already-normalized
// message history.
func (s *Server) attachGuardrailsHooks(c *gin.Context, hc *protocol.HandleContext, actualModel string, provider *typ.Provider, messages []guardrailscore.Message) {
	_, _, _, _, scenario, _, _ := GetTrackingContext(c)
	if !s.guardrailsEnabledForScenario(scenario) {
		return
	}

	baseInput := s.buildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionResponse, messages)
	logrus.Debugf("Guardrails: attaching hook (scenario=%s model=%s)", baseInput.Scenario, baseInput.Model)

	onEvent, onComplete, onError := NewGuardrailsHooks(
		s.guardrailsRuntime,
		baseInput,
		WithGuardrailsContext(c.Request.Context()),
		WithGuardrailsOnBlock(func(result GuardrailsHookResult) {
			if result.BlockToolID == "" || result.BlockMessage == "" {
				return
			}
			s.recordGuardrailsHistory(baseInput, result.Result, "tool_use", result.BlockMessage)
			stream.RegisterGuardrailsBlock(c, result.BlockToolID, result.BlockIndex, result.BlockMessage)
		}),
		WithGuardrailsOnVerdict(func(result GuardrailsHookResult) {
			c.Set("guardrails_result", result.Result)
			if result.BlockMessage != "" {
				c.Set("guardrails_block_message", result.BlockMessage)
				c.Set("guardrails_block_index", result.BlockIndex)
				if result.BlockToolID != "" {
					c.Set("guardrails_block_tool_id", result.BlockToolID)
				}
				// Early tool_use blocks are already recorded in onBlock. Skip adding a
				// second near-identical response entry when the final verdict points at
				// the same blocked tool_use.
				if result.BlockToolID == "" {
					s.recordGuardrailsHistory(baseInput, result.Result, "response", result.BlockMessage)
				}
			}
			if result.Err != nil {
				c.Set("guardrails_error", result.Err.Error())
			}
		}),
	)
	if onEvent != nil {
		hc.WithOnStreamEvent(onEvent)
	}
	if onComplete != nil {
		hc.WithOnStreamComplete(onComplete)
	}
	if onError != nil {
		hc.WithOnStreamError(onError)
	}
}

// applyGuardrailsToAnthropicV1Request is the merged request-side entry for
// Anthropic v1 requests. It runs request tool_result filtering first and then
// request credential masking on the latest raw request state.
func (s *Server) applyGuardrailsToAnthropicV1Request(c *gin.Context, req *anthropic.MessageNewParams, actualModel string, provider *typ.Provider) {
	if req == nil {
		return
	}

	input := s.buildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionRequest, nil)
	input.Payload.Protocol = "anthropic_v1"
	input.Payload.Request = req

	_, err := guardrailspipeline.ProcessAnthropicV1Request(
		c.Request.Context(),
		s.guardrailsRuntime,
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
	input.Payload.Protocol = "anthropic_beta"
	input.Payload.Request = req

	_, err := guardrailspipeline.ProcessAnthropicBetaRequest(
		c.Request.Context(),
		s.guardrailsRuntime,
		input,
	)
	if err != nil {
		return
	}
}

func (s *Server) getGuardrailsCredentialMaskState(c *gin.Context) *guardrailscore.CredentialMaskState {
	if existing, ok := c.Get(guardrailscore.CredentialMaskStateContextKey); ok {
		if state, ok := existing.(*guardrailscore.CredentialMaskState); ok {
			return state
		}
	}
	state := guardrailscore.NewCredentialMaskState()
	c.Set(guardrailscore.CredentialMaskStateContextKey, state)
	return state
}

func (s *Server) restoreGuardrailsCredentialAliasesV1Response(c *gin.Context, resp *anthropic.Message) bool {
	if resp == nil {
		return false
	}
	return restoreAnthropicResponseBlocks(resp.Content, s.getGuardrailsCredentialMaskState(c))
}

func (s *Server) restoreGuardrailsCredentialAliasesV1BetaResponse(c *gin.Context, resp *anthropic.BetaMessage) bool {
	if resp == nil {
		return false
	}
	return restoreAnthropicBetaResponseBlocks(resp.Content, s.getGuardrailsCredentialMaskState(c))
}

func restoreAnthropicResponseBlocks(blocks []anthropic.ContentBlockUnion, state *guardrailscore.CredentialMaskState) bool {
	if state == nil || len(state.AliasToReal) == 0 {
		return false
	}
	changed := false
	for i := range blocks {
		block := &blocks[i]
		if guardrailscore.MayContainAliasToken(block.Text) {
			if text, ok := guardrailscore.RestoreText(block.Text, state); ok {
				block.Text = text
				changed = true
			}
		}
		if len(block.Input) == 0 || !guardrailscore.MayContainAliasToken(string(block.Input)) {
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal(block.Input, &parsed); err != nil {
			if restored, ok := guardrailscore.RestoreText(string(block.Input), state); ok {
				block.Input = json.RawMessage(restored)
				changed = true
			}
			continue
		}
		restored, ok := guardrailscore.RestoreStructuredValue(parsed, state)
		if !ok {
			continue
		}
		payload, err := json.Marshal(restored)
		if err != nil {
			continue
		}
		block.Input = payload
		changed = true
	}
	return changed
}

func restoreAnthropicBetaResponseBlocks(blocks []anthropic.BetaContentBlockUnion, state *guardrailscore.CredentialMaskState) bool {
	if state == nil || len(state.AliasToReal) == 0 {
		return false
	}
	changed := false
	for i := range blocks {
		block := &blocks[i]
		if guardrailscore.MayContainAliasToken(block.Text) {
			if text, ok := guardrailscore.RestoreText(block.Text, state); ok {
				block.Text = text
				changed = true
			}
		}
		if len(block.Input) == 0 || !guardrailscore.MayContainAliasToken(string(block.Input)) {
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal(block.Input, &parsed); err != nil {
			if restored, ok := guardrailscore.RestoreText(string(block.Input), state); ok {
				block.Input = json.RawMessage(restored)
				changed = true
			}
			continue
		}
		restored, ok := guardrailscore.RestoreStructuredValue(parsed, state)
		if !ok {
			continue
		}
		payload, err := json.Marshal(restored)
		if err != nil {
			continue
		}
		block.Input = payload
		changed = true
	}
	return changed
}
