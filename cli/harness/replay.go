package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// fixtureFS holds captured agent request bodies, grouped by API style and
// scenario:
//
//	testdata/fixtures/<style>/<scenario>.json
//
// where style is "anthropic" (claude, opencode) or "openai_responses" (codex).
// Each fixture is the on-the-wire request body an agent CLI sends to the
// gateway. Replaying it exercises the gateway's built-in rule + dispatch
// pipeline without spawning the real CLI.
//
//go:embed testdata/fixtures
var fixtureFS embed.FS

// replayScenario binds a matrix Scenario to the replay machinery.
//
// The matrix Scenario carries both the deterministic mock responses (used by
// the "virtual" upstream) and content-level assertions. For the "vmodel" and
// "real" upstreams the response is not controlled by the test, so only the
// upstream-independent `structural` assertions are run.
type replayScenario struct {
	matrix        protocol_validate.Scenario
	streaming     bool
	defaultVModel string // vmodel registry ID used when --upstream=vmodel
	structural    []protocol_validate.Assertion
}

// replayScenarios is the set of scenarios `harness replay` can run. The map
// key is the scenario name as it appears on the CLI and in fixture paths.
var replayScenarios = map[string]replayScenario{
	"text": {
		matrix:        protocol_validate.TextScenario(),
		streaming:     false,
		defaultVModel: "echo-model",
		structural: []protocol_validate.Assertion{
			protocol_validate.AssertHTTPStatus(200),
			protocol_validate.AssertContentNonEmpty(),
		},
	},
	"tool_use": {
		matrix:        protocol_validate.ToolUseScenario(),
		streaming:     false,
		defaultVModel: "web-search-example",
		structural: []protocol_validate.Assertion{
			protocol_validate.AssertHTTPStatus(200),
			protocol_validate.AssertHasToolCalls(1),
		},
	},
	"streaming_text": {
		matrix:        protocol_validate.StreamingTextScenario(),
		streaming:     true,
		defaultVModel: "echo-model",
		structural: []protocol_validate.Assertion{
			protocol_validate.AssertHTTPStatus(200),
			protocol_validate.AssertStreamEventCount(1),
			protocol_validate.AssertContentNonEmpty(),
		},
	},
}

// replayScenarioOrder is the stable run order for replay scenarios.
var replayScenarioOrder = []string{"text", "tool_use", "streaming_text"}

// replaySkip lists "<upstream>/<agent>/<scenario>" runs that hit known,
// documented gaps or bugs. Skipping keeps the suite green while the issue is
// tracked; remove an entry once the underlying problem is fixed.
//
// Each entry is a real defect surfaced by replay — not a test artifact.
var replaySkip = map[string]string{
	// Transform-pipeline gap: codex speaks the OpenAI Responses API, and
	// tool_use round-trips through the Responses source path, whose
	// tool_call conversion is incomplete. Mirrors protocol_validate's
	// skipSourceScenarios["openai_responses|tool_use"]. Fails on every upstream.
	"virtual/codex/tool_use": "Responses API source: tool_use conversion incomplete",
	"vmodel/codex/tool_use":  "Responses API source: tool_use conversion incomplete",
	"real/codex/tool_use":    "Responses API source: tool_use conversion incomplete",
}

// ReplayCmd replays captured agent request fixtures through the gateway's
// built-in rule, routed to a selectable upstream.
//
// Unlike `agent --mock` (which spawns the real CLI against a VirtualServer
// mock), replay sends a fixed request body directly. This makes it hermetic
// and fast — no CLI install, no subprocess — and lets it cover the in-process
// vmodel dispatch path that the spawn-the-CLI modes never touch.
type ReplayCmd struct {
	Upstream  string   `kong:"name='upstream',default='vmodel',enum='virtual,vmodel,real',help='Upstream to route through: virtual (VirtualServer mock), vmodel (in-process vmodel), real (live provider via --config)'"`
	Scenario  []string `kong:"name='scenario',sep=',',help='Scenario(s) to run: text, tool_use, streaming_text (default: all)'"`
	VModel    string   `kong:"name='vmodel',help='Override the vmodel registry ID for every scenario (upstream=vmodel only)'"`
	Config    string   `kong:"name='config',help='Provider config file — required for upstream=real; the first runnable entry is used'"`
	AgentType string   `kong:"arg,name='agent',help='Agent: claude | codex | opencode | batch'"`
}

// Help returns extended help for `harness replay --help`.
func (*ReplayCmd) Help() string {
	return `Replay captured agent request fixtures through the gateway.

A fixture is the on-the-wire request body an agent CLI sends. Replaying it
exercises the gateway's built-in rule and dispatch pipeline without spawning
the real CLI — hermetic, fast, and able to cover the in-process vmodel
dispatch path.

Upstreams:
  virtual   Route to the in-process VirtualServer mock. The response is fully
            controlled by the scenario, so the scenario's content-level
            assertions are checked.
  vmodel    Route to a seeded builtin virtual-model provider (in-process
            dispatch). Structural assertions only.
  real      Route to a live provider read from --config (first runnable
            entry). Structural assertions only.

Examples:
  harness replay claude
  harness replay batch --upstream virtual
  harness replay codex --scenario text,tool_use
  harness replay claude --upstream vmodel --vmodel echo-model
  harness replay claude --upstream real --config providers.yaml`
}

// replayRow is the per-(agent,scenario) outcome of a replay run.
type replayRow struct {
	Agent      string
	Scenario   string
	Upstream   string
	Passed     bool
	Skipped    bool
	SkipReason string
	Duration   time.Duration
	Failures   []string
	Err        string // setup / transport error (no assertions were run)
	RawBody    []byte // gateway response body — kept for debugging failures
}

// Run executes the replay subcommand.
func (r *ReplayCmd) Run() error {
	if r.AgentType == "" {
		return fmt.Errorf("agent type is required (claude | codex | opencode | batch)")
	}

	agents, err := resolveReplayAgents(r.AgentType)
	if err != nil {
		return err
	}
	scenarios, err := resolveReplayScenarios(r.Scenario)
	if err != nil {
		return err
	}

	var realEntry *protocol_validate.RealModelEntry
	if r.Upstream == "real" {
		realEntry, err = firstRunnableEntry(r.Config)
		if err != nil {
			return err
		}
	} else if r.Config != "" {
		fmt.Printf("⚠️  --config is ignored unless --upstream=real\n\n")
	}

	fmt.Printf("🧪 Replay: agents=%v scenarios=%v upstream=%s\n", agents, scenarios, r.Upstream)
	if realEntry != nil {
		fmt.Printf("📋 Real entry: %s (%s)\n", realEntry.Name, realEntry.BaseURL)
	}
	fmt.Println()

	var rows []replayRow
	for _, agent := range agents {
		for _, scenarioName := range scenarios {
			row := r.runOne(agent, scenarioName, realEntry)
			printReplayRow(row)
			rows = append(rows, row)
		}
	}

	fmt.Println()
	printReplaySummary(rows)

	for _, row := range rows {
		if !row.Passed {
			return fmt.Errorf("replay: %d of %d runs failed", countReplayFailures(rows), len(rows))
		}
	}
	return nil
}

// runOne replays a single (agent, scenario) pair against the configured upstream.
func (r *ReplayCmd) runOne(agentName, scenarioName string, realEntry *protocol_validate.RealModelEntry) replayRow {
	row := replayRow{Agent: agentName, Scenario: scenarioName, Upstream: r.Upstream}
	start := time.Now()

	if reason, skip := replaySkip[r.Upstream+"/"+agentName+"/"+scenarioName]; skip {
		row.Skipped = true
		row.SkipReason = reason
		row.Passed = true // skipped runs do not count as failures
		return row
	}

	agentType := parseAgentType(agentName)
	sc := replayScenarios[scenarioName]

	body, err := loadFixture(agentType, scenarioName)
	if err != nil {
		row.Err = err.Error()
		row.Duration = time.Since(start)
		return row
	}
	// Normalize the fixture's model field to the built-in rule's RequestModel
	// so fixtures stay upstream-agnostic — the rule (not the fixture) decides
	// which provider+model the request resolves to.
	body, err = rewriteModel(body, builtinRequestModel(agentType))
	if err != nil {
		row.Err = fmt.Sprintf("rewrite fixture model: %v", err)
		row.Duration = time.Since(start)
		return row
	}

	env, err := protocol_validate.NewAgentTestEnv(agentType)
	if err != nil {
		row.Err = fmt.Sprintf("create test env: %v", err)
		row.Duration = time.Since(start)
		return row
	}
	defer env.Close(false)

	// Wire the built-in rule to the selected upstream.
	switch r.Upstream {
	case "virtual":
		err = env.SetupVirtualAgentScenario(agentType, sc.matrix)
	case "vmodel":
		vmodelID := sc.defaultVModel
		if r.VModel != "" {
			vmodelID = r.VModel
		}
		err = env.SetupVModelAgent(agentType, vmodelID)
	case "real":
		apiStyle, sErr := protocol_validate.ResolveAPIStyle(*realEntry)
		if sErr != nil {
			err = sErr
			break
		}
		err = env.SetupRealAgent(agentType, realEntry.Name, realEntry.Model,
			realEntry.BaseURL, realEntry.APIKey, apiStyle)
	default:
		err = fmt.Errorf("unknown upstream %q", r.Upstream)
	}
	if err != nil {
		row.Err = fmt.Sprintf("setup %s upstream: %v", r.Upstream, err)
		row.Duration = time.Since(start)
		return row
	}

	result, err := env.ReplayFixture(agentType, body, sc.streaming)
	if err != nil {
		row.Err = err.Error()
		row.Duration = time.Since(start)
		return row
	}
	row.RawBody = result.RawBody

	// For the virtual upstream the response is deterministic, so the
	// scenario's content-level assertions apply. For vmodel / real the
	// response is not controlled by the test — only structural checks.
	assertions := sc.structural
	if r.Upstream == "virtual" {
		assertions = sc.matrix.Assertions
	}
	for _, a := range assertions {
		if aerr := a.Check(result); aerr != nil {
			row.Failures = append(row.Failures, fmt.Sprintf("%s: %v", a.Name, aerr))
		}
	}

	row.Passed = len(row.Failures) == 0
	row.Duration = time.Since(start)
	return row
}

// resolveReplayAgents expands the agent CLI argument into a concrete list.
func resolveReplayAgents(arg string) ([]string, error) {
	if strings.EqualFold(arg, "batch") {
		return batchAgents, nil
	}
	if parseAgentType(arg) == "" {
		return nil, fmt.Errorf("unknown agent: %q (available: claude, codex, opencode, batch)", arg)
	}
	return []string{strings.ToLower(arg)}, nil
}

// resolveReplayScenarios validates the --scenario filter, defaulting to all.
func resolveReplayScenarios(filter []string) ([]string, error) {
	if len(filter) == 0 {
		return replayScenarioOrder, nil
	}
	var out []string
	for _, name := range filter {
		name = strings.TrimSpace(name)
		if _, ok := replayScenarios[name]; !ok {
			return nil, fmt.Errorf("unknown scenario %q (available: %s)",
				name, strings.Join(replayScenarioOrder, ", "))
		}
		out = append(out, name)
	}
	return out, nil
}

// firstRunnableEntry loads a provider config and returns its first entry that
// has all fields needed to run (used by --upstream=real).
func firstRunnableEntry(configFile string) (*protocol_validate.RealModelEntry, error) {
	if configFile == "" {
		return nil, fmt.Errorf("--upstream=real requires --config <file>")
	}
	entries, err := loadProvidersConfig(configFile)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if len(missingFields(entries[i])) == 0 {
			return &entries[i], nil
		}
	}
	return nil, fmt.Errorf("no runnable entries in %s — fill in apikey, baseurl, model, api_style", configFile)
}

// loadFixture reads testdata/fixtures/<style>/<scenario>.json from the
// embedded FS. style is derived from the agent type.
func loadFixture(agentType protocol_validate.AgentType, scenario string) ([]byte, error) {
	style := "anthropic"
	if agentType == protocol_validate.AgentTypeCodex {
		style = "openai_responses"
	}
	path := fmt.Sprintf("testdata/fixtures/%s/%s.json", style, scenario)
	body, err := fixtureFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load fixture %s: %w", path, err)
	}
	return body, nil
}

// rewriteModel sets the top-level "model" field of a JSON request body.
func rewriteModel(body []byte, model string) ([]byte, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	m["model"] = model
	return json.Marshal(m)
}

// printReplayRow prints the outcome of one (agent, scenario) replay.
func printReplayRow(row replayRow) {
	tag := fmt.Sprintf("[%s/%s]", row.Agent, row.Scenario)
	dur := row.Duration.Round(time.Millisecond)
	switch {
	case row.Skipped:
		fmt.Printf("⏭  SKIP  %s  %s\n", tag, row.SkipReason)
	case row.Err != "":
		fmt.Printf("❌ ERROR %s  duration=%s\n  %s\n", tag, dur, row.Err)
	case row.Passed:
		fmt.Printf("✅ PASS  %s  duration=%s\n", tag, dur)
	default:
		fmt.Printf("❌ FAIL  %s  duration=%s\n", tag, dur)
		for _, f := range row.Failures {
			fmt.Printf("  %s\n", f)
		}
		if len(row.RawBody) > 0 {
			fmt.Printf("  body: %s\n", truncateBody(row.RawBody))
		}
	}
}

// truncateBody returns a single-line, length-capped view of a response body.
func truncateBody(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// printReplaySummary prints the unified replay summary table.
func printReplaySummary(rows []replayRow) {
	pass, fail, skip := 0, 0, 0
	for _, row := range rows {
		switch {
		case row.Skipped:
			skip++
		case row.Passed:
			pass++
		default:
			fail++
		}
	}
	fmt.Printf("📊 Replay Summary\n")
	fmt.Printf("Total: %d | ✓ Pass: %d | ✗ Fail: %d | ⏭ Skip: %d\n\n", len(rows), pass, fail, skip)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Agent\tScenario\tUpstream\tStatus\tDuration")
	fmt.Fprintln(w, "-----\t--------\t--------\t------\t--------")
	// Stable order: as collected.
	for _, row := range rows {
		status := "✓ PASS"
		switch {
		case row.Skipped:
			status = "⏭ SKIP"
		case row.Err != "":
			status = "✗ ERROR"
		case !row.Passed:
			status = "✗ FAIL"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			row.Agent, row.Scenario, row.Upstream, status, row.Duration.Round(time.Millisecond))
	}
	w.Flush()
	fmt.Println()
}

// countReplayFailures counts rows that did not pass.
func countReplayFailures(rows []replayRow) int {
	n := 0
	for _, row := range rows {
		if !row.Passed {
			n++
		}
	}
	return n
}
