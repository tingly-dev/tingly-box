package evaluate

import guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"

// SessionMeta is the minimal request-scoped metadata needed to build a
// guardrails.Input from an adapted request or response view.
type SessionMeta struct {
	Scenario     string
	Model        string
	RequestModel string
	ProviderName string
}

// ResponseView is the protocol-neutral response shape used by Guardrails
// evaluation. Adapters should populate only the fields Guardrails needs.
type ResponseView struct {
	Text           string
	Command        *guardrailscore.Command
	MessageHistory []guardrailscore.Message
}

// RequestView is the protocol-neutral request shape used by Guardrails
// evaluation on request-side content such as tool_result filtering.
type RequestView struct {
	Text           string
	Command        *guardrailscore.Command
	MessageHistory []guardrailscore.Message
}

// ToolResultRequestView is the adapted request-side view for tool_result
// filtering. It carries both the normalized RequestView and a small amount of
// extraction metadata that later stages may want for logging or short-circuit
// checks without reparsing the original protocol request.
type ToolResultRequestView struct {
	View          RequestView
	HasToolResult bool
	BlockCount    int
	PartCount     int
}

// WithResponseView overlays a normalized response view onto an existing input
// context while preserving scenario/model/metadata/runtime fields.
func WithResponseView(input guardrailscore.Input, view ResponseView) guardrailscore.Input {
	input.Direction = guardrailscore.DirectionResponse
	input.Content = guardrailscore.Content{
		Text:     view.Text,
		Command:  view.Command,
		Messages: view.MessageHistory,
	}
	return input
}

// WithRequestView overlays a normalized request view onto an existing input
// context while preserving scenario/model/metadata/runtime fields.
func WithRequestView(input guardrailscore.Input, view RequestView) guardrailscore.Input {
	input.Direction = guardrailscore.DirectionRequest
	input.Content = guardrailscore.Content{
		Text:     view.Text,
		Command:  view.Command,
		Messages: view.MessageHistory,
	}
	return input
}

// ToResponseInput converts an adapted response view into the shared
// guardrails.Input shape consumed by the policy engine.
func ToResponseInput(session SessionMeta, view ResponseView) guardrailscore.Input {
	return WithResponseView(guardrailscore.Input{
		Scenario:  session.Scenario,
		Model:     session.Model,
		Metadata: map[string]interface{}{
			"provider":      session.ProviderName,
			"request_model": session.RequestModel,
		},
	}, view)
}

// ToRequestInput converts an adapted request view into the shared
// guardrails.Input shape consumed by the policy engine.
func ToRequestInput(session SessionMeta, view RequestView) guardrailscore.Input {
	return WithRequestView(guardrailscore.Input{
		Scenario:  session.Scenario,
		Model:     session.Model,
		Metadata: map[string]interface{}{
			"provider":      session.ProviderName,
			"request_model": session.RequestModel,
		},
	}, view)
}
