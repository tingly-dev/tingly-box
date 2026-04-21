package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// RealAgentTestResult holds the result of one real-model Agent run.
type RealAgentTestResult struct {
	EntryName    string
	Agent        string
	APIStyle     string
	Prompt       string
	Model        string // upstream model name (entry.Model in real mode, builtin RequestModel in mock mode)
	RequestModel string // gateway-facing rule RequestModel (tingly/cc, tingly-codex, tingly-opencode)
	BaseURL      string // upstream base URL (real mode); empty for mock mode
	Success      bool
	TimedOut     bool // true if the agent CLI was killed by the per-entry timeout
	Duration     time.Duration
	Output       string
	Error        string
	ExitCode     int
}

// missingFields returns the names of fields required to run a test that are missing or placeholder.
func missingFields(entry protocol_validate.RealModelEntry) []string {
	var miss []string
	if strings.TrimSpace(entry.BaseURL) == "" {
		miss = append(miss, "baseurl")
	}
	apiKey := strings.TrimSpace(entry.APIKey)
	if apiKey == "" || apiKey == "YOUR_API_KEY" {
		miss = append(miss, "apikey")
	}
	model := strings.TrimSpace(entry.Model)
	if model == "" || model == "MODEL_NAME" {
		miss = append(miss, "model")
	}
	// api_style is now required
	if strings.TrimSpace(entry.APIStyle) == "" {
		miss = append(miss, "api_style")
	}
	return miss
}

// loadProvidersConfig reads and parses a providers config file (YAML).
// Returns the expanded list of test entries.
func loadProvidersConfig(path string) ([]protocol_validate.RealModelEntry, error) {
	return protocol_validate.LoadProvidersConfig(path)
}

// runRealAgentTests iterates over all model entries and runs the agent against each.
// It returns the per-entry results and a terminal error if the entire run could
// not proceed (e.g., bad config, no runnable entries). A non-nil results slice
// with failed entries returns a non-nil error summarising the failure count, so
// callers can still render the detailed report.
//
// If writer is non-nil, each per-entry result is appended durably as soon as
// it completes. Any (agent, entry) key already present in skip is bypassed so
// --resume can pick up where a previous run left off. If filter is non-empty,
// only entries whose name matches (case-insensitive) are considered; unknown
// filter names are warned about but do not fail the run.
func runRealAgentTests(agentName string, modelsFile string, prompt string, writer *summaryWriter, skip map[resumeKey]struct{}, filter []string) ([]*RealAgentTestResult, error) {
	profileType := parseAgentType(agentName)
	if profileType == "" {
		return nil, fmt.Errorf("unknown agent: %q (available: claude, codex, opencode)", agentName)
	}

	if prompt == "" {
		if p, ok := defaultPrompts[agentName]; ok {
			prompt = p
		} else {
			prompt = "What is the capital of France?"
		}
	}

	entries, err := loadProvidersConfig(modelsFile)
	if err != nil {
		return nil, err
	}

	// Build a case-insensitive lookup of filter names, if any.
	var wanted map[string]struct{}
	if len(filter) > 0 {
		wanted = make(map[string]struct{}, len(filter))
		for _, name := range filter {
			n := strings.ToLower(strings.TrimSpace(name))
			if n != "" {
				wanted[n] = struct{}{}
			}
		}
	}
	matched := make(map[string]struct{}, len(wanted))

	// Separate runnable entries from incomplete ones.
	var runnable []protocol_validate.RealModelEntry
	var skipped []string
	for _, entry := range entries {
		if wanted != nil {
			if _, ok := wanted[strings.ToLower(strings.TrimSpace(entry.Name))]; !ok {
				continue
			}
			matched[strings.ToLower(strings.TrimSpace(entry.Name))] = struct{}{}
		}
		miss := missingFields(entry)
		if len(miss) > 0 {
			skipped = append(skipped, fmt.Sprintf("%s (missing: %s)", entry.Name, strings.Join(miss, ", ")))
		} else {
			runnable = append(runnable, entry)
		}
	}

	// Warn about filter names that didn't match any config entry.
	if wanted != nil {
		var unknown []string
		for name := range wanted {
			if _, ok := matched[name]; !ok {
				unknown = append(unknown, name)
			}
		}
		if len(unknown) > 0 {
			fmt.Printf("⚠️  --filter names not found in config: %s\n\n", strings.Join(unknown, ", "))
		}
	}

	if len(skipped) > 0 {
		fmt.Printf("⚠️  Skipping %d incomplete entries:\n", len(skipped))
		for _, s := range skipped {
			fmt.Printf("   • %s\n", s)
		}
		fmt.Println()
	}

	if len(runnable) == 0 {
		if wanted != nil {
			return nil, fmt.Errorf("no runnable entries in %s matched --filter %v", modelsFile, filter)
		}
		return nil, fmt.Errorf("no runnable entries in %s — fill in apikey, baseurl, and model fields", modelsFile)
	}

	fmt.Printf("🧪 Real Agent test: %s\n", agentName)
	fmt.Printf("📝 Prompt: %s\n", prompt)
	fmt.Printf("📋 Models: %d runnable entries from %s\n\n", len(runnable), modelsFile)

	results := make([]*RealAgentTestResult, 0, len(runnable))
	for i, entry := range runnable {
		if _, ok := skip[resumeKey{Agent: agentName, Entry: entry.Name}]; ok {
			fmt.Printf("⏭  [%d/%d] %s — skip (resume)\n\n", i+1, len(runnable), entry.Name)
			continue
		}
		fmt.Printf("── [%d/%d] %s ──\n", i+1, len(runnable), entry.Name)
		r := runOneRealAgentTest(profileType, entry, prompt)
		results = append(results, r)
		printRealAgentTestResult(r)
		if writer != nil {
			if aerr := writer.Append(r); aerr != nil {
				fmt.Printf("⚠️  summary append failed: %v\n", aerr)
			}
		}
		fmt.Println()
	}

	failed := 0
	for _, r := range results {
		if !r.Success {
			failed++
		}
	}
	if failed > 0 {
		return results, fmt.Errorf("%d of %d real Agent tests failed", failed, len(results))
	}
	return results, nil
}

// runOneRealAgentTest runs the agent CLI for a single model entry.
func runOneRealAgentTest(agentType protocol_validate.AgentType, entry protocol_validate.RealModelEntry, prompt string) *RealAgentTestResult {
	result := &RealAgentTestResult{
		EntryName: entry.Name,
		Agent:     string(agentType),
		Prompt:    prompt,
		Model:     entry.Model,
		BaseURL:   entry.BaseURL,
	}
	start := time.Now()

	// Start isolated gateway (virtual server is also started but unused here)
	env, err := protocol_validate.NewAgentTestEnv(agentType)
	if err != nil {
		result.Error = fmt.Sprintf("create test env: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer env.Close(false)

	apiStyle, err := protocol_validate.ResolveAPIStyle(entry)
	if err != nil {
		result.Error = fmt.Sprintf("resolve api_style: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	result.APIStyle = apiStyle
	providerName := fmt.Sprintf("%s", entry.Name)

	if err := env.SetupRealAgent(agentType, providerName, entry.Model, entry.BaseURL, entry.APIKey, apiStyle); err != nil {
		result.Error = fmt.Sprintf("setup real Agent: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// The client CLI must send the gateway rule's RequestModel (the fixed
	// tingly/* name), not entry.Model. The gateway matches the built-in-<agent>
	// rule by RequestModel, then forwards upstream via Service{Provider, Model=entry.Model}.
	// Sending entry.Model directly bypasses rule matching and breaks the
	// provider → rule → service routing contract.
	requestModel := builtinRequestModel(agentType)
	if requestModel == "" {
		result.Error = fmt.Sprintf("unsupported Agent type: %s", agentType)
		result.Duration = time.Since(start)
		return result
	}
	result.RequestModel = requestModel

	var agentResult *AgentTestResult
	switch agentType {
	case protocol_validate.AgentTypeClaudeCode:
		agentResult, err = executeClaudeWithEnv(env, requestModel, prompt)
	case protocol_validate.AgentTypeCodex:
		agentResult, err = executeCodexWithEnv(env, requestModel, prompt)
	case protocol_validate.AgentTypeOpenCode:
		agentResult, err = executeOpenCodeWithEnv(env, requestModel, prompt)
	}

	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Success = agentResult.Success
	result.TimedOut = agentResult.TimedOut
	result.Output = agentResult.Output
	result.Error = agentResult.Error
	result.ExitCode = agentResult.ExitCode
	return result
}

// printRealAgentTestResult prints the result of one model entry.
func printRealAgentTestResult(r *RealAgentTestResult) {
	duration := fmt.Sprintf("%dms", r.Duration.Milliseconds())
	switch {
	case r.Success:
		fmt.Printf("✅ PASS  [%s]  model=%s  duration=%s\n", r.EntryName, r.Model, duration)
		if r.Output != "" {
			lines := strings.Split(strings.TrimSpace(r.Output), "\n")
			limit := 10
			if len(lines) < limit {
				limit = len(lines)
			}
			for _, line := range lines[:limit] {
				fmt.Printf("  %s\n", line)
			}
			if len(lines) > 10 {
				fmt.Printf("  ... (%d more lines)\n", len(lines)-10)
			}
		}
	case r.TimedOut:
		fmt.Printf("⏱  TIMEOUT  [%s]  model=%s  duration=%s\n", r.EntryName, r.Model, duration)
		if r.Error != "" {
			fmt.Printf("  %s\n", r.Error)
		}
	default:
		fmt.Printf("❌ FAIL  [%s]  model=%s  duration=%s\n", r.EntryName, r.Model, duration)
		if r.Error != "" {
			fmt.Printf("  Error: %s\n", r.Error)
		}
		if r.Output != "" {
			lines := strings.Split(strings.TrimSpace(r.Output), "\n")
			limit := 5
			if len(lines) < limit {
				limit = len(lines)
			}
			for _, line := range lines[:limit] {
				fmt.Printf("  %s\n", line)
			}
		}
	}
}

// printAgentSummary prints the unified Agent-test summary table. It is shared
// by every path — mock, real, and batch — so the report shape is identical
// regardless of mode or single-vs-batch invocation.
func printAgentSummary(results []*RealAgentTestResult) {
	pass, fail, timedOut := 0, 0, 0
	for _, r := range results {
		switch {
		case r.Success:
			pass++
		case r.TimedOut:
			timedOut++
		default:
			fail++
		}
	}

	fmt.Printf("📊 Agent Test Summary\n")
	fmt.Printf("Total: %d | ✓ Pass: %d | ✗ Fail: %d | ⏱ Timeout: %d\n\n", len(results), pass, fail, timedOut)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Agent\tEntry\tModel\tStatus\tDuration\tAPI Style")
	fmt.Fprintln(w, "-----\t-----\t-----\t------\t--------\t---------")
	for _, r := range results {
		var status string
		switch {
		case r.Success:
			status = "✓ PASS"
		case r.TimedOut:
			status = "⏱ TIMEOUT"
		default:
			status = "✗ FAIL"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Agent,
			r.EntryName,
			r.Model,
			status,
			r.Duration.Round(time.Millisecond),
			r.APIStyle,
		)
	}
	w.Flush()
	fmt.Println()
}
