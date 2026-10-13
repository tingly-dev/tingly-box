// Headless one-shot agent setup for CI / fully-managed environments.
//
// `ci apply` configures a single (provider, model, agent) triple from flags
// alone, with no stdin reads and no interactive prompts. It is idempotent:
// a provider with the same name is updated in place rather than duplicated,
// and routing rules / agent config files converge to the requested state.

package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// CICmdKong is the headless setup command. The only subcommand today is
// `apply`; it is wired as the default so plain `tingly-box ci ...` keeps
// working even before any other subcommand is added.
type CICmdKong struct {
	Apply   CIApplyCmdKong   `kong:"cmd,name='apply',default='1',help='Apply a (provider, model, agent) configuration in one shot'"`
	Install CIInstallCmdKong `kong:"cmd,name='install',help='Install an agent CLI via npm'"`
}

// CIApplyCmdKong carries all flags accepted by `ci apply`. Every required
// flag is enforced inside Run so that we can emit a single combined error
// message (Kong's per-flag required errors are noisy and exit before we can
// suggest related missing flags).
type CIApplyCmdKong struct {
	Agent         string `kong:"flag,name='agent',help='Agent type: cc | oc | cx (aliases of claude-code / opencode / codex)'"`
	ProviderURL   string `kong:"flag,name='provider-url',help='Provider API base URL (used as upsert key)'"`
	ProviderToken string `kong:"flag,name='provider-token',help='Provider API token'"`
	ProviderStyle string `kong:"flag,name='provider-style',help='Provider API style: openai | anthropic'"`
	Model         string `kong:"flag,name='model',help='Model name'"`
	Unified       bool   `kong:"flag,name='unified',default='true',negatable,help='Unified mode (claude-code only)'"`
	StatusLine    bool   `kong:"flag,name='status-line',help='Install status line script (claude-code only)'"`
	DryRun        bool   `kong:"flag,name='dry-run',help='Print the plan without applying changes'"`
}

// Run validates flags, parses the agent type, normalises the API style, then
// delegates the actual work to applyCISpec. Errors here exit with status 2
// (configuration error) via os.Exit so CI runners can distinguish bad input
// from runtime failures (which return through Kong as status 1).
func (c *CIApplyCmdKong) Run(appManager *AppManager) error {
	spec, err := c.toSpec()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ci: %v\n", err)
		os.Exit(2)
	}

	if c.DryRun {
		printCIPlan(spec)
		return nil
	}

	return applyCISpec(appManager, spec)
}

// ciSpec is the validated, internal representation of a `ci apply` request.
// It is intentionally separate from CIApplyCmdKong so that tests (and any
// future YAML loader) can build one without going through Kong.
type ciSpec struct {
	AgentType     agent.AgentType
	ProviderURL   string
	ProviderToken string
	ProviderStyle protocol.APIStyle
	Model         string
	Unified       bool
	StatusLine    bool
}

// toSpec converts CLI flags into a validated ciSpec, collecting every
// missing-flag complaint into a single error so the user sees them all at
// once rather than having to fix and re-run repeatedly.
func (c *CIApplyCmdKong) toSpec() (*ciSpec, error) {
	var missing []string
	if strings.TrimSpace(c.Agent) == "" {
		missing = append(missing, "--agent")
	}
	if strings.TrimSpace(c.ProviderURL) == "" {
		missing = append(missing, "--provider-url")
	}
	if strings.TrimSpace(c.ProviderToken) == "" {
		missing = append(missing, "--provider-token")
	}
	if strings.TrimSpace(c.ProviderStyle) == "" {
		missing = append(missing, "--provider-style")
	}
	if strings.TrimSpace(c.Model) == "" {
		missing = append(missing, "--model")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required flag(s): %s", strings.Join(missing, ", "))
	}

	agentType, err := agent.ParseAgentType(c.Agent)
	if err != nil {
		return nil, fmt.Errorf("invalid --agent %q (accepted: cc, oc, cx)", c.Agent)
	}

	style, err := parseAPIStyle(c.ProviderStyle)
	if err != nil {
		return nil, err
	}

	// status-line / unified only make sense for claude-code. We don't reject
	// them outright for non-cc agents (the user may be scripting a matrix and
	// passing the same flags everywhere) — silently ignored below via the
	// agent-specific branch in ApplyAgent.
	return &ciSpec{
		AgentType:     agentType,
		ProviderURL:   c.ProviderURL,
		ProviderToken: c.ProviderToken,
		ProviderStyle: style,
		Model:         c.Model,
		Unified:       c.Unified,
		StatusLine:    c.StatusLine,
	}, nil
}

// parseAPIStyle accepts "openai" / "anthropic" (case-insensitive). OAuth
// providers are intentionally rejected — they need a browser flow that has
// no place in a CI invocation.
func parseAPIStyle(s string) (protocol.APIStyle, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "openai":
		return protocol.APIStyleOpenAI, nil
	case "anthropic":
		return protocol.APIStyleAnthropic, nil
	default:
		return "", fmt.Errorf("invalid --provider-style %q (accepted: openai, anthropic)", s)
	}
}

// printCIPlan emits the intended actions without touching disk. Token is
// redacted so the output is safe to capture in CI logs.
func printCIPlan(s *ciSpec) {
	fmt.Println("ci apply (dry-run):")
	fmt.Printf("  agent:    %s\n", s.AgentType)
	fmt.Printf("  provider: %s\n", s.ProviderURL)
	fmt.Printf("    token:  %s\n", redactToken(s.ProviderToken))
	fmt.Printf("    style:  %s\n", s.ProviderStyle)
	fmt.Printf("  model:    %s\n", s.Model)
	if s.AgentType == agent.AgentTypeClaudeCode {
		fmt.Printf("  unified:  %v\n", s.Unified)
		fmt.Printf("  status_line: %v\n", s.StatusLine)
	}
}

// redactToken keeps the first/last 4 chars so an operator can sanity-check
// they passed the right secret without leaking the whole value to logs.
func redactToken(t string) string {
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "…" + t[len(t)-4:]
}
