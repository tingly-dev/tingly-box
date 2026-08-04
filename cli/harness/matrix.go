package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// MatrixCmd runs the protocol validation matrix tests.
//
// The set of (source → target) pairs is defined explicitly in
// internal/protocoltest.DefaultPairs() rather than as a Cartesian
// product, so what's exercised matches the dispatch graph documented
// in internal/protocol/README.md. Each pair runs against every
// scenario and both streaming modes.
//
// --source and --target filter pairs by their source/target component.
type MatrixCmd struct {
	Scenarios    []string `kong:"name='scenario',sep=',',help='Filter by scenario name (can repeat or comma-separate)'"`
	Sources      []string `kong:"name='source',sep=',',help='Filter by source protocol (can repeat or comma-separate)'"`
	Targets      []string `kong:"name='target',sep=',',help='Filter by target protocol (can repeat or comma-separate)'"`
	Streaming    bool     `kong:"name='streaming',help='Run only streaming tests'"`
	NonStream    bool     `kong:"name='non-streaming',help='Run only non-streaming tests'"`
	Mode         string   `kong:"name='mode',default='default',enum='default,all,single,transitive,idempotent,flags,content_shapes,cache_controls,bridges',help='Section selection: default (single + idempotent + dormant Bridges; two-hop OFF), all (every section), single (production A→B only), transitive (production A→B→C only), idempotent (production round-trip only), flags (per-rule flags only), content_shapes (request content-shape regression only), cache_controls (single-hop + ABA cache/no-cache requests), bridges (dormant Stage/Bridge topology only)'"`
	Client       string   `kong:"name='client',default='http',enum='http,gosdk,python,node,aisdk',help='Client driver: http (raw JSON over net/http, default), gosdk (official anthropic-sdk-go / openai-go), python (real Python SDKs via subprocess driver), node (real Node SDKs via subprocess driver), aisdk (AI SDK by Vercel via subprocess driver)'"`
	JsonOutput   bool     `kong:"name='json',help='Output results as JSON'"`
	Verbose      int      `kong:"name='verbose',short='v',type='counter',help='Verbose output (repeat for more detail)'"`
	RecordDir    string   `kong:"name='record-dir',env='HARNESS_RECORD_DIR',help='Directory for recording requests/responses (default: disabled)'"`
	BatchCount   int      `kong:"name='batch',default='1',help='Number of times to run each test (for stability/performance testing)'"`
	MCPEnabled   bool     `kong:"name='mcp',help='Enable MCP feature flag in test env'"`
	StageEnabled bool     `kong:"name='stage',help='Enable production Protocol Stage selection in the test server'"`
	Guardrails   bool     `kong:"name='guardrails',help='Enable an active allow-only Guardrails runtime in the test server'"`
}

// matrixSection describes one runnable section of the validation matrix and
// which --mode values include it. Adding a section means adding one entry
// here (plus its ExecuteAll* executor in internal/protocoltest) and extending
// the --mode enum on MatrixCmd.
type matrixSection struct {
	name       string
	modes      []string // --mode values that include this section
	httpOnly   bool     // requires --client=http
	exec       func(*protocoltest.Matrix) []protocoltest.TestResult
	bridgeExec func(*protocoltest.BridgeMatrix) []protocoltest.TestResult
}

// matrixSections is the section registry: the single source of truth for what
// each --mode runs. Section order here is output order.
var matrixSections = []matrixSection{
	{name: "single", modes: []string{"default", "all", "single"}, exec: (*protocoltest.Matrix).ExecuteAll},
	{name: "transitive", modes: []string{"all", "transitive"}, exec: (*protocoltest.Matrix).ExecuteAllTransitive},
	{name: "idempotent", modes: []string{"default", "all", "idempotent"}, exec: (*protocoltest.Matrix).ExecuteAllIdempotent},
	{name: "flags", modes: []string{"all", "flags"}, httpOnly: true, exec: (*protocoltest.Matrix).ExecuteAllFlags},
	{name: "content_shapes", modes: []string{"all", "content_shapes"}, httpOnly: true, exec: (*protocoltest.Matrix).ExecuteAllContentShapes},
	{name: "cache_controls", modes: []string{"all", "cache_controls"}, httpOnly: true, exec: (*protocoltest.Matrix).ExecuteAllCacheControls},
	{name: "bridges", modes: []string{"default", "all", "bridges"}, httpOnly: true, bridgeExec: (*protocoltest.BridgeMatrix).ExecuteAll},
}

// Help returns extended help text shown by `harness matrix --help`.
func (*MatrixCmd) Help() string {
	return `Examples:
  # Default: production single-hop + idempotent round-trips + dormant Bridges.
  # Two-hop (A→B→C) transitive chains are OFF by default.
  harness matrix

  # Run every section: single + two-hop + idempotent + flags + dormant Bridges
  harness matrix --mode=all

  # Run only two-hop (A→B→C) transitive chain tests
  harness matrix --mode=transitive

  # Run only idempotent round-trip tests
  harness matrix --mode=idempotent

  # Run only per-rule flag behavior tests
  harness matrix --mode=flags

  # Run only request content-shape regression tests
  harness matrix --mode=content_shapes

  # Run single-hop + ABA prompt-cache request tests
  harness matrix --mode=cache_controls

  # Run only the dormant Stage/Bridge topology (no production dispatch claim)
  harness matrix --mode=bridges
  harness matrix --mode=bridges --source=anthropic_v1 --target=anthropic_beta

  # Run only single-hop (A→B) tests
  harness matrix --mode=single

  # Exercise production Stage selection (Chat/Beta/V1 routes plus Responses routes)
  harness matrix --mode=single --stage --source=openai_chat --target=anthropic_beta
  harness matrix --mode=single --stage --source=anthropic_beta --target=openai_chat
  harness matrix --mode=single --stage --source=openai_responses --target=openai_responses
  harness matrix --mode=single --stage --source=openai_responses --target=anthropic_beta
  harness matrix --mode=single --stage --source=openai_responses --target=openai_chat
  harness matrix --mode=single --stage --source=anthropic_beta --target=openai_responses

  # Exercise every production Stage route with a server-owned MCP tool loop
  harness matrix --mode=single --stage --mcp --scenario=mcp_owned_tool

  # Persist and automatically validate RequestRecord boundaries for every MCP round
  harness matrix --mode=single --stage --mcp --scenario=mcp_owned_tool --record-dir=/tmp/tingly-mcp-records

  # Exercise the opt-in Beta identity RequestRecord canary and retain artifacts
  harness matrix --mode=single --stage --source=anthropic_beta --target=anthropic_beta --record-dir=/tmp/tingly-records

  # Exercise Beta Guardrail as a Stage without changing scenario semantics
  harness matrix --mode=single --stage --guardrails --source=anthropic_beta

  # Drive requests through real client stacks instead of raw HTTP
  harness matrix --mode=single --client=gosdk    # official Go SDKs, in-process
  harness matrix --mode=single --client=python   # real Python SDKs (subprocess driver)
  harness matrix --mode=single --client=node     # real Node SDKs (subprocess driver)
  harness matrix --mode=single --client=aisdk    # AI SDK by Vercel (subprocess driver)

  # Run specific scenario only
  harness matrix --scenario text

  # Run all scenarios for specific source/target
  harness matrix --source anthropic_v1 --target openai_chat

  # Run only streaming tests
  harness matrix --streaming

  # JSON output for CI/CD
  harness matrix --json

  # Verbose output with details
  harness matrix -vv`
}

// Run executes the matrix tests with the parsed flags.
func (m *MatrixCmd) Run() error {
	verbose := m.Verbose

	// Set log level based on verbose flag
	// Default (v=0): Warn level - minimal output
	// v=1: Info level - normal output
	// v=2+: Debug level - detailed output
	switch verbose {
	case 0:
		logrus.SetLevel(logrus.WarnLevel)
	case 1:
		logrus.SetLevel(logrus.InfoLevel)
	default:
		logrus.SetLevel(logrus.DebugLevel)
	}

	// Resolve flag conflicts early.
	if m.Streaming && m.NonStream {
		return fmt.Errorf("cannot specify both --streaming and --non-streaming")
	}
	if m.Client != "http" && m.Mode == "bridges" {
		return fmt.Errorf("--mode=bridges only supports --client=http (the Bridge matrix runs in-process and has no client transport)")
	}
	if m.Client != "http" {
		for _, sec := range matrixSections {
			if sec.httpOnly && m.Mode == sec.name {
				return fmt.Errorf("--mode=%s only supports --client=http (this suite drives raw requests directly)", m.Mode)
			}
		}
	}
	if m.Mode == "bridges" && m.MCPEnabled {
		return fmt.Errorf("--mode=bridges does not support --mcp (the Bridge matrix validates protocol topology only)")
	}
	if m.Mode == "bridges" && m.StageEnabled {
		return fmt.Errorf("--mode=bridges does not support --stage (use --mode=single to exercise the production Stage path)")
	}
	if m.Mode == "bridges" && m.Guardrails {
		return fmt.Errorf("--mode=bridges does not support --guardrails (use --mode=single to exercise the production Guardrail path)")
	}
	if m.Mode == "bridges" && m.RecordDir != "" {
		return fmt.Errorf("--mode=bridges does not support --record-dir (the Bridge matrix runs in-process without HTTP recording)")
	}
	if slices.Contains(m.Scenarios, protocoltest.MCPStageOwnedToolScenarioName) && (!m.MCPEnabled || !m.StageEnabled) {
		return fmt.Errorf("--scenario=%s requires both --stage and --mcp", protocoltest.MCPStageOwnedToolScenarioName)
	}

	client, err := resolveClient(m.Client)
	if err != nil {
		return err
	}

	// Build matrix with filters
	matrix := protocoltest.DefaultMatrix()
	if m.MCPEnabled && m.StageEnabled {
		matrix = matrix.WithMCPStageCoverage()
	}
	if client != nil {
		matrix = matrix.WithClient(client)
	}

	if len(m.Scenarios) > 0 {
		matrix = matrix.OnlyScenarios(m.Scenarios...)
	}
	if len(m.Sources) > 0 {
		matrix = matrix.OnlySources(m.Sources...)
	}
	if len(m.Targets) > 0 {
		matrix = matrix.OnlyTargets(m.Targets...)
	}
	if m.Streaming {
		matrix = matrix.OnlyStreaming(true)
	}
	if m.NonStream {
		matrix = matrix.OnlyStreaming(false)
	}
	if m.RecordDir != "" {
		matrix = matrix.WithRecordDir(m.RecordDir)
	}
	if m.BatchCount > 1 {
		matrix = matrix.WithBatchCount(m.BatchCount)
	}
	if m.MCPEnabled {
		matrix = matrix.WithMCPEnabled()
	}
	if m.StageEnabled {
		matrix = matrix.WithProtocolStage()
	}
	if m.Guardrails {
		matrix = matrix.WithGuardrails()
	}
	bridgeMatrix := protocoltest.DefaultBridgeMatrix()
	if len(m.Scenarios) > 0 {
		bridgeMatrix = bridgeMatrix.OnlyScenarios(m.Scenarios...)
	}
	if len(m.Sources) > 0 {
		bridgeMatrix = bridgeMatrix.OnlySources(m.Sources...)
	}
	if len(m.Targets) > 0 {
		bridgeMatrix = bridgeMatrix.OnlyTargets(m.Targets...)
	}
	if m.Streaming {
		bridgeMatrix = bridgeMatrix.OnlyStreaming(true)
	}
	if m.NonStream {
		bridgeMatrix = bridgeMatrix.OnlyStreaming(false)
	}
	if m.BatchCount > 1 {
		bridgeMatrix = bridgeMatrix.WithBatchCount(m.BatchCount)
	}

	// Collect results for the sections the selected --mode includes (see
	// matrixSections for the mode → section mapping).
	var combined []protocoltest.TestResult
	for _, sec := range matrixSections {
		if !slices.Contains(sec.modes, m.Mode) {
			continue
		}
		if sec.httpOnly && m.Client != "http" {
			// Only reachable with --mode=all: selecting an http-only section
			// directly with another client is rejected up front.
			logrus.Warnf("skipping %s section: only supported with --client=http", sec.name)
			continue
		}
		if sec.bridgeExec != nil {
			combined = append(combined, sec.bridgeExec(bridgeMatrix)...)
			continue
		}
		combined = append(combined, sec.exec(matrix)...)
	}
	results := filterResults(combined, m)
	executed := 0
	for _, result := range results {
		if !result.Skipped {
			executed++
		}
	}
	if executed == 0 {
		return fmt.Errorf("no executable test cases matched the selected matrix filters")
	}

	// Output results
	if m.JsonOutput {
		if err := printJSON(results); err != nil {
			return fmt.Errorf("failed to output JSON: %w", err)
		}
	} else {
		printTable(results, verbose)
		if m.RecordDir != "" && m.StageEnabled && m.MCPEnabled && slices.Contains(m.Scenarios, protocoltest.MCPStageOwnedToolScenarioName) {
			fmt.Printf("\n📼 RequestRecord artifacts verified: %d case(s) in %s\n", executed, m.RecordDir)
		}
	}

	// Determine exit code
	for _, r := range results {
		if !r.Passed && !r.Skipped {
			return fmt.Errorf("some tests failed")
		}
	}
	return nil
}

// resolveClient maps the --client flag to a protocoltest.Client driver.
// Returns nil for "http" (the matrix default). For subprocess drivers it
// fails fast with an actionable message when the interpreter or the driver's
// dependencies are missing.
func resolveClient(name string) (protocoltest.Client, error) {
	switch name {
	case "http", "":
		return nil, nil
	case "gosdk":
		return protocoltest.NewGoSDKClient(), nil
	case "python":
		dir, err := driverDir()
		if err != nil {
			return nil, err
		}
		if _, err := exec.LookPath("python3"); err != nil {
			return nil, fmt.Errorf("--client=python requires python3 on PATH")
		}
		if out, err := exec.Command("python3", "-c", "import anthropic, openai").CombinedOutput(); err != nil {
			return nil, fmt.Errorf("--client=python requires the anthropic and openai packages: pip install -r %s\n%s",
				filepath.Join(dir, "python", "requirements.txt"), out)
		}
		return protocoltest.NewPythonClient(dir), nil
	case "node":
		dir, err := nodeDriverDir(name, "node")
		if err != nil {
			return nil, err
		}
		return protocoltest.NewNodeClient(dir), nil
	case "aisdk":
		dir, err := nodeDriverDir(name, "aisdk")
		if err != nil {
			return nil, err
		}
		return protocoltest.NewAISDKClient(dir), nil
	default:
		return nil, fmt.Errorf("unknown client driver %q", name)
	}
}

// nodeDriverDir validates a node-based subprocess driver (interpreter on PATH
// and installed dependencies) and returns the tests/clients root.
func nodeDriverDir(clientName, subdir string) (string, error) {
	dir, err := driverDir()
	if err != nil {
		return "", err
	}
	if _, err := exec.LookPath("node"); err != nil {
		return "", fmt.Errorf("--client=%s requires node on PATH", clientName)
	}
	if _, err := os.Stat(filepath.Join(dir, subdir, "node_modules")); err != nil {
		return "", fmt.Errorf("--client=%s requires driver dependencies: npm install --prefix %s",
			clientName, filepath.Join(dir, subdir))
	}
	return dir, nil
}

// driverDir locates the tests/clients directory holding the subprocess
// drivers: $HARNESS_DRIVER_DIR if set, else tests/clients relative to the
// working directory (i.e. running from the repo root).
func driverDir() (string, error) {
	if dir := os.Getenv("HARNESS_DRIVER_DIR"); dir != "" {
		return dir, nil
	}
	dir := filepath.Join("tests", "clients")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("driver directory %q not found: run from the repo root or set HARNESS_DRIVER_DIR", dir)
	}
	return dir, nil
}

// filterResults applies the command's source/target/streaming filters to the
// combined results. The matrix filters its own pairs, but sections whose cases
// are not pair-derived (idempotent, flags, content_shapes) only honor these
// flags through this post-filter.
func filterResults(results []protocoltest.TestResult, m *MatrixCmd) []protocoltest.TestResult {
	var filtered []protocoltest.TestResult

	for _, r := range results {
		if len(m.Sources) > 0 && !slices.Contains(m.Sources, string(r.Source)) {
			continue
		}
		if len(m.Targets) > 0 && !slices.Contains(m.Targets, string(r.Target)) {
			continue
		}
		if m.Streaming && !r.Streaming {
			continue
		}
		if m.NonStream && r.Streaming {
			continue
		}
		filtered = append(filtered, r)
	}

	return filtered
}
