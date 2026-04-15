package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AdvisorFormat indicates the API format of the advisor endpoint.
type AdvisorFormat int

const (
	FormatOpenAI AdvisorFormat = iota
	FormatAnthropic
)

func detectAdvisorFormat(cfg typ.AdvisorConfig) AdvisorFormat {
	url := strings.ToLower(cfg.BaseURL)
	model := strings.ToLower(cfg.Model)
	if strings.Contains(url, "anthropic") || strings.HasPrefix(model, "claude-") {
		return FormatAnthropic
	}
	return FormatOpenAI
}

// AdvisorToolSource is an in-process ToolSource that serves the advisor tool.
type AdvisorToolSource struct {
	*BaseToolSource
	config typ.AdvisorConfig
}

// NewAdvisorToolSource creates a new advisor tool source.
func NewAdvisorToolSource(sourceConfig typ.MCPSourceConfig) (*AdvisorToolSource, error) {
	base := NewBaseToolSource(sourceConfig.ID, TransportType("advisor"))
	cfg := typ.AdvisorConfig{MaxUsesPerRequest: 3}
	if sourceConfig.Advisor != nil {
		cfg = *sourceConfig.Advisor
	}
	if cfg.MaxUsesPerRequest <= 0 {
		cfg.MaxUsesPerRequest = 3
	}
	return &AdvisorToolSource{
		BaseToolSource: base,
		config:         cfg,
	}, nil
}

// Connect is a no-op for the in-process advisor source.
func (s *AdvisorToolSource) Connect(ctx context.Context) error {
	s.setState(StateConnected, nil)
	return nil
}

// Disconnect is a no-op for the in-process advisor source.
func (s *AdvisorToolSource) Disconnect(ctx context.Context) error {
	s.setState(StateDisconnected, nil)
	return nil
}

// IsConnected always returns true for the in-process advisor source.
func (s *AdvisorToolSource) IsConnected() bool {
	return true
}

// ListTools returns the advisor tool definition.
func (s *AdvisorToolSource) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "Why the executor is consulting the advisor.",
			},
		},
		"required": []string{"reason"},
	}
	schemaBytes, _ := json.Marshal(schema)
	return []ToolDefinition{{
		Name:        "advisor",
		Description: s.description(3), // placeholder; will be dynamic later
		InputSchema: schemaBytes,
	}}, nil
}

// CallTool executes the advisor tool.
func (s *AdvisorToolSource) CallTool(ctx context.Context, toolName string, arguments string) (string, error) {
	// Skeleton only for now.
	return `{"assessment":"not yet implemented","recommendation":"please wait"}`, nil
}

// GetSourceConfig returns the source configuration.
func (s *AdvisorToolSource) GetSourceConfig() interface{} {
	return s.config
}

// HealthCheck is a no-op for the in-process advisor source.
func (s *AdvisorToolSource) HealthCheck(ctx context.Context) error {
	return nil
}

// EnableHealthCheck is a no-op.
func (s *AdvisorToolSource) EnableHealthCheck(ctx context.Context, interval time.Duration) {}

// DisableHealthCheck is a no-op.
func (s *AdvisorToolSource) DisableHealthCheck(ctx context.Context) {}

func (s *AdvisorToolSource) description(remainingUses int) string {
	return "Consult a more powerful advisor model for strategic guidance. " +
		"Use this when facing architectural decisions, complex debugging, unclear trade-offs, or when stuck. " +
		"You have " + string(rune('0'+remainingUses)) + " advisor consultation(s) remaining this request."
}

const advisorSystemPrompt = `You are an advisor to a coding agent. You share the agent's full conversation context and provide strategic guidance.

Your role:
- Provide plans, corrections, or stop signals
- Be concise and actionable — the executor will act on your advice immediately
- Focus on the "why" and the "what", not the "how" (the executor handles execution)
- Flag risks, edge cases, or better approaches the executor may have missed
- IMPORTANT: If you notice issues the executor did NOT ask about — bugs, security flaws, design problems, missed edge cases — proactively report them. The executor may have blind spots; your job is to catch what they miss.

You do NOT:
- Call tools or execute commands
- Produce user-facing output
- Repeat information already in the conversation
- Ask follow-up questions (give your best guidance with available context)

Structure your response as valid JSON:
{
  "assessment": "What's the situation? (1-2 sentences)",
  "recommendation": "What should the executor do? (actionable steps)",
  "unsolicited_findings": "Anything else you noticed that the executor should know, even if they didn't ask. Skip this field if there's nothing to add."
}`
