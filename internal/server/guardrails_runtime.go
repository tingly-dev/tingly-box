package server

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// guardrailsSession carries the request-scoped metadata needed to evaluate guardrails.
// Keep this small and protocol-agnostic so provider handlers only need to adapt their
// request/response payloads into guardrails.Content and can reuse the same runtime flow.
type guardrailsSession struct {
	Scenario     string
	Model        string
	RequestModel string
	ProviderName string
}

var guardrailsSupportedScenarios = []string{
	string(typ.ScenarioAnthropic),
	string(typ.ScenarioClaudeCode),
}

func (s *Server) guardrailsSessionFromContext(c *gin.Context, actualModel string, provider *typ.Provider) guardrailsSession {
	_, _, _, requestModel, scenario, _, _ := GetTrackingContext(c)
	providerName := ""
	if provider != nil {
		providerName = provider.Name
	}
	return guardrailsSession{
		Scenario:     scenario,
		Model:        actualModel,
		RequestModel: requestModel,
		ProviderName: providerName,
	}
}

// guardrailsEnabledForSession centralizes feature-flag checks so protocol handlers
// do not repeat scenario/global guardrails gating logic.
func (s *Server) guardrailsEnabledForSession(session guardrailsSession) bool {
	if s.guardrailsEngine == nil || s.config == nil {
		return false
	}
	if !s.guardrailsSupportsScenario(session.Scenario) {
		return false
	}
	return s.config.GetScenarioFlag(typ.RuleScenario(session.Scenario), "guardrails") ||
		s.config.GetScenarioFlag(typ.ScenarioGlobal, "guardrails")
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

// buildGuardrailsBaseInput creates the shared evaluation envelope; adapters can then
// add request/response-specific content without rebuilding metadata each time.
func (s *Server) buildGuardrailsBaseInput(session guardrailsSession, direction guardrails.Direction, messages []guardrails.Message) guardrails.Input {
	return guardrails.Input{
		Scenario:  session.Scenario,
		Model:     session.Model,
		Direction: direction,
		Content: guardrails.Content{
			Messages: messages,
		},
		Metadata: map[string]interface{}{
			"provider":      session.ProviderName,
			"request_model": session.RequestModel,
		},
	}
}

// attachGuardrailsHooks wires the shared stream guardrails runtime into a protocol
// handle context. Provider-specific handlers only need to provide already-normalized
// message history.
func (s *Server) attachGuardrailsHooks(c *gin.Context, hc *protocol.HandleContext, session guardrailsSession, messages []guardrails.Message) {
	if !s.guardrailsEnabledForSession(session) {
		return
	}

	logrus.Debugf("Guardrails: attaching hook (scenario=%s model=%s)", session.Scenario, session.Model)
	baseInput := s.buildGuardrailsBaseInput(session, guardrails.DirectionResponse, messages)

	onEvent, onComplete, onError := NewGuardrailsHooks(
		s.guardrailsEngine,
		baseInput,
		WithGuardrailsContext(c.Request.Context()),
		WithGuardrailsOnBlock(func(result GuardrailsHookResult) {
			if result.BlockToolID == "" || result.BlockMessage == "" {
				return
			}
			s.recordGuardrailsHistory(c, session, baseInput, result.Result, "tool_use", result.BlockMessage)
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
					s.recordGuardrailsHistory(c, session, baseInput, result.Result, "response", result.BlockMessage)
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

// evaluateGuardrailsToolResult centralizes request-side tool_result filtering so
// v1 and beta handlers only deal with protocol-specific extract/replace code.
func (s *Server) evaluateGuardrailsToolResult(c *gin.Context, session guardrailsSession, toolResultText string, history []guardrails.Message) (guardrails.Result, bool) {
	if !s.guardrailsEnabledForSession(session) {
		return guardrails.Result{}, false
	}
	if toolResultText == "" {
		return guardrails.Result{}, false
	}

	input := s.buildGuardrailsBaseInput(session, guardrails.DirectionRequest, filterGuardrailsMessages(history))
	input.Content.Text = toolResultText

	result, err := s.guardrailsEngine.Evaluate(c.Request.Context(), input)
	if err != nil {
		return guardrails.Result{}, false
	}
	if result.Verdict == guardrails.VerdictBlock {
		s.recordGuardrailsHistory(c, session, input, result, "tool_result", "")
	}
	return result, true
}
