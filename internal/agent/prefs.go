package agent

import (
	"encoding/json"
	"strings"
)

// ClaudeCodePrefs is the user-tunable surface of Claude Code's env config.
// JSON tags map 1:1 to the env var names Claude Code reads, so marshaling
// produces exactly what should go under "env" in ~/.claude/settings.json.
// Empty fields are omitted — meaning "don't write this env, let Claude Code
// fall back to its own default".
type ClaudeCodePrefs struct {
	// Model routing
	AnthropicModel              string `json:"ANTHROPIC_MODEL,omitempty"`
	AnthropicDefaultHaikuModel  string `json:"ANTHROPIC_DEFAULT_HAIKU_MODEL,omitempty"`
	AnthropicDefaultSonnetModel string `json:"ANTHROPIC_DEFAULT_SONNET_MODEL,omitempty"`
	AnthropicDefaultOpusModel   string `json:"ANTHROPIC_DEFAULT_OPUS_MODEL,omitempty"`
	ClaudeCodeSubagentModel     string `json:"CLAUDE_CODE_SUBAGENT_MODEL,omitempty"`

	// Limits — kept as strings so empty = omit (avoids the "0 means unset"
	// ambiguity that a *int would force callers to deal with).
	APITimeoutMs              string `json:"API_TIMEOUT_MS,omitempty"`
	ClaudeCodeMaxOutputTokens string `json:"CLAUDE_CODE_MAX_OUTPUT_TOKENS,omitempty"`
	MaxThinkingTokens         string `json:"MAX_THINKING_TOKENS,omitempty"`
	BashDefaultTimeoutMs      string `json:"BASH_DEFAULT_TIMEOUT_MS,omitempty"`
	BashMaxTimeoutMs          string `json:"BASH_MAX_TIMEOUT_MS,omitempty"`
	McpTimeout                string `json:"MCP_TIMEOUT,omitempty"`
	McpToolTimeout            string `json:"MCP_TOOL_TIMEOUT,omitempty"`
	MaxMcpOutputTokens        string `json:"MAX_MCP_OUTPUT_TOKENS,omitempty"`

	// Privacy / behavior switches — "1" to enable, "" to omit.
	DisableTelemetry                     string `json:"DISABLE_TELEMETRY,omitempty"`
	DisableErrorReporting                string `json:"DISABLE_ERROR_REPORTING,omitempty"`
	ClaudeCodeDisableNonessentialTraffic string `json:"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC,omitempty"`
	DisableAutoupdater                   string `json:"DISABLE_AUTOUPDATER,omitempty"`
	UseBuiltinRipgrep                    string `json:"USE_BUILTIN_RIPGREP,omitempty"`

	// Network proxy. Backend accepts these; the current UI does not surface
	// them (too niche for the form, and they overlap with system-level proxy
	// settings managed elsewhere). Wire them up if/when a user asks.
	HTTPProxy  string `json:"HTTP_PROXY,omitempty"`
	HTTPSProxy string `json:"HTTPS_PROXY,omitempty"`
	NoProxy    string `json:"NO_PROXY,omitempty"`

	// Extra is an escape hatch for env vars not modeled above. Merged in
	// after marshaling; values here override the typed fields if keys collide.
	Extra map[string]string `json:"-"`
}

// ToEnv materializes prefs into an env map ready to be written under "env"
// in settings.json. baseURL and apiKey are sourced from the server context
// (not from prefs) and are always injected.
func (p ClaudeCodePrefs) ToEnv(baseURL, apiKey string) (map[string]string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	for k, v := range p.Extra {
		env[k] = v
	}
	env["ANTHROPIC_BASE_URL"] = strings.TrimRight(baseURL, "/") + "/tingly/claude_code"
	env["ANTHROPIC_AUTH_TOKEN"] = apiKey
	return env, nil
}

// DefaultClaudeCodePrefs returns tb's canonical defaults for the given
// mode. Used by the CLI harness directly and as the seed value for the
// GUI quick-config form when no user customization exists yet.
func DefaultClaudeCodePrefs(unified bool) ClaudeCodePrefs {
	p := ClaudeCodePrefs{
		APITimeoutMs:                         "3000000",
		ClaudeCodeMaxOutputTokens:            "32000",
		DisableTelemetry:                     "1",
		DisableErrorReporting:                "1",
		ClaudeCodeDisableNonessentialTraffic: "1",
	}
	if unified {
		p.AnthropicModel = "tingly/cc"
		p.AnthropicDefaultHaikuModel = "tingly/cc"
		p.AnthropicDefaultSonnetModel = "tingly/cc"
		p.AnthropicDefaultOpusModel = "tingly/cc"
		p.ClaudeCodeSubagentModel = "tingly/cc"
	} else {
		p.AnthropicModel = "tingly/cc-default"
		p.AnthropicDefaultHaikuModel = "tingly/cc-haiku"
		p.AnthropicDefaultSonnetModel = "tingly/cc-sonnet"
		p.AnthropicDefaultOpusModel = "tingly/cc-opus"
		p.ClaudeCodeSubagentModel = "tingly/cc-subagent"
	}
	return p
}
