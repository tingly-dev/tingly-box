package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// AgentCmd runs an agent CLI (claude, codex, opencode) end-to-end through the
// tingly-box gateway. Use --mock for a virtual upstream or --config <file> for
// real providers; exactly one of the two modes must be selected.
type AgentCmd struct {
	Mock        bool          `kong:"name='mock',help='Virtual-model mode: run against an in-process mock upstream'"`
	Config      string        `kong:"name='config',help='Real-provider mode: path to provider config file (YAML)'"`
	Prompt      string        `kong:"name='prompt',help='Prompt to send (overrides positional arg and default)'"`
	Summary     string        `kong:"name='summary',default='harness-summary.csv',help='Path to CSV summary file (per-row results, written durably)'"`
	OutputDir   string        `kong:"name='output-dir',help='Directory for full output files (default: harness-output/)'"`
	Resume      string        `kong:"name='resume',help='Resume — skip every (agent,entry) already recorded in the summary file'"`
	OnlyFailing bool          `kong:"name='only-failing',help='Only re-run (agent,entry) pairs whose latest summary row is FAIL/TIMEOUT. Real-provider mode only; mutually exclusive with --resume.'"`
	Filter      []string      `kong:"name='filter',sep=',',help='Only run providers whose name matches (case-insensitive, comma-separated). Real-provider mode only.'"`
	Timeout     time.Duration `kong:"name='timeout',short='t',default='2m',help='Per-entry timeout for the agent CLI invocation (e.g. 30s, 2m). 0 disables.'"`
	AgentType   string        `kong:"arg,optional,name='agent',help='Agent type: claude | codex | opencode | batch'"`
	Args        []string      `kong:"arg,optional,name='prompt-args',help='Optional positional prompt parts (joined with spaces)'"`
}

// Help returns the long usage for `harness agent --help`.
func (*AgentCmd) Help() string {
	return `Agent argument:

  claude | codex | opencode   Run a single agent
  batch                       Run every supported agent in sequence. Each agent
                              uses its own default prompt unless --prompt or a
                              positional prompt is provided. All agents run even
                              if one fails; the command exits non-zero if any
                              agent failed.

Two modes, selected by an explicit flag:

  --mock                 Virtual-model mode
    Spins up an in-process gateway wired to a virtual upstream (mock responses)
    and runs the real agent CLI against it. Exercises protocol translation and
    rule matching without touching any real upstream.

  --config <file>        Real-provider mode
    Reads a list of real providers from a YAML config file. For each provider
    and model combination, spins up an isolated gateway, registers the provider,
    binds the built-in rule (builtin:claude_code:cc / builtin:codex:default / builtin:opencode:default)
    to a Service pointing at that provider+model, and runs the agent CLI against
    the gateway. Reports pass/fail per entry.

Exactly one of --mock or --config must be supplied.

Config file format — YAML (.yaml/.yml):
  providers:
    - name: "my-provider"
      baseurl: "https://api.anthropic.com"
      apikey: "sk-ant-..."
      api_style: "anthropic"
      models:
        - "claude-3-5-sonnet-20241022"
        - "claude-3-5-opus-20241022"

Examples:
  # Virtual-model mode
  harness agent claude   --mock
  harness agent claude   --mock "What is 2+2?"
  harness agent opencode --mock "Hello, world!"
  harness agent batch    --mock

  # Real-provider mode
  harness agent claude --config providers.yaml
  harness agent codex  --config providers.yaml "What is 2+2?"
  harness agent batch  --config providers.yaml

  # Resume an interrupted run (skips every (agent,entry) already in the CSV)
  harness agent batch --config providers.yaml --resume ""

  # Run only specific entries by name (real-provider mode)
  harness agent claude --config providers.yaml --filter acme,beta
  harness agent batch  --config providers.yaml --filter acme

Persistence:
  Every run appends per-row results to harness-summary.csv in the working
  directory (override with --summary <file>). Full prompt and output are written
  to separate markdown files in harness-output/ (override with --output-dir <dir>).
  Rows are flushed immediately, so partial progress survives Ctrl-C / crashes.
  With --resume, any (agent,entry) pair already recorded in the summary file
  is skipped. Output IDs use UUIDs, so no ID state management is needed.

Timeouts:
  Each per-entry agent CLI invocation is capped by --timeout (default 2m). On
  timeout the child process is killed and the row is recorded with status
  TIMEOUT in the summary. Pass --timeout 0 to disable.

Generate a config template with: harness init-config`
}

// Run executes the agent subcommand.
func (a *AgentCmd) Run() error {
	agentName := a.AgentType
	if agentName == "" {
		return fmt.Errorf("agent type is required (claude | codex | opencode | batch)")
	}
	prompt := a.Prompt
	if prompt == "" && len(a.Args) > 0 {
		prompt = strings.Join(a.Args, " ")
	}

	switch {
	case a.Mock && a.Config != "":
		return fmt.Errorf("--mock and --config are mutually exclusive; pick exactly one")
	case !a.Mock && a.Config == "":
		return fmt.Errorf("must specify a mode: --mock (virtual upstream) or --config <file> (real providers)")
	}

	if a.OnlyFailing && a.Resume != "" {
		return fmt.Errorf("--only-failing and --resume are mutually exclusive")
	}
	if a.OnlyFailing && a.Mock {
		fmt.Printf("⚠️  --only-failing is ignored in --mock mode (no named entries)\n\n")
	}

	// Make the per-entry timeout visible to the executors (they read
	// the package-level variable before launching the agent CLI).
	agentRunTimeout = a.Timeout
	if a.Timeout > 0 {
		fmt.Printf("⏱  Per-entry timeout: %s\n", a.Timeout)
	}

	// Open durable summary writer and load resume keys before any work runs.
	writer, err := openSummaryWriter(a.Summary, a.OutputDir)
	if err != nil {
		return err
	}
	defer writer.Close()
	fmt.Printf("📒 Summary: %s (per-row, append-on-write)\n", a.Summary)
	if a.OutputDir != "" {
		fmt.Printf("📁 Output: %s\n", a.OutputDir)
	} else {
		fmt.Printf("📁 Output: %s\n", DefaultOutputDir)
	}

	var skip map[resumeKey]struct{}
	if a.Resume != "" {
		skip, err = loadResumeKeys(a.Summary)
		if err != nil {
			return err
		}
		fmt.Printf("⏭  Resume: skipping %d previously-recorded (agent,entry) rows\n", len(skip))
	}

	// --only-failing: re-run just the red items from the prior summary. Computed
	// here (not in runRealAgentTests) so the "no prior summary" error surfaces
	// before any work runs.
	var only map[resumeKey]struct{}
	if a.OnlyFailing && a.Config != "" {
		only, err = loadFailedKeys(a.Summary)
		if err != nil {
			if errors.Is(err, errNoPriorSummary) {
				return fmt.Errorf("--only-failing: %w", err)
			}
			return err
		}
		if len(only) == 0 {
			fmt.Printf("✅ No FAIL/TIMEOUT rows in %s — nothing to re-run\n", a.Summary)
		} else {
			fmt.Printf("🔁 Only-failing: re-running %d red (agent,entry) rows\n", len(only))
		}
	}
	fmt.Println()

	if strings.EqualFold(agentName, "batch") {
		return runBatchAgentTests(a.Mock, a.Config, prompt, writer, skip, only, a.Filter)
	}

	var results []*RealAgentTestResult
	var runErr error
	if a.Config != "" {
		results, runErr = runRealAgentTests(agentName, a.Config, prompt, writer, skip, only, a.Filter)
	} else {
		if len(a.Filter) > 0 {
			fmt.Printf("⚠️  --filter is ignored in --mock mode (no named entries)\n\n")
		}
		results, runErr = runVirtualAgentTest(agentName, prompt, writer, skip)
	}
	if len(results) > 0 {
		printAgentSummary(results)
	}
	return runErr
}

// Default test prompts for each profile type
var defaultPrompts = map[string]string{
	"claude":   "What is the capital of France?",
	"codex":    "What is 2+2?",
	"opencode": "Hello, world!",
}

// batchAgents is the ordered list of agents to run in batch mode.
var batchAgents = []string{"claude", "codex", "opencode"}

// parseAgentType converts a string to AgentType
func parseAgentType(s string) protocoltest.AgentType {
	switch strings.ToLower(s) {
	case "claude", "claude-code", "claudecode", "cc":
		return protocoltest.AgentTypeClaudeCode
	case "codex":
		return protocoltest.AgentTypeCodex
	case "opencode", "open-code", "oc":
		return protocoltest.AgentTypeOpenCode
	default:
		return ""
	}
}
