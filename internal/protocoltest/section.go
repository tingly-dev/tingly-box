package protocoltest

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// This file holds the shared drivers behind the matrix's sections
// (single-hop, transitive, idempotent, flags, content_shapes). Each section
// contributes only its combos and per-combo execution; the env-per-scenario
// lifecycle, setup-failure fan-out, parallelism, and testing.T scaffolding
// live here once.

// scenarioCombo is one runnable cell within a per-scenario section: the
// TestResult metadata that identifies it plus the function that executes it
// against a live env. meta doubles as the setup-failure result when the
// scenario's env cannot be created.
type scenarioCombo struct {
	meta TestResult
	run  func(env *TestEnv) TestResult
}

// maxSectionParallelism bounds how many env-backed units (scenarios, recorder
// cases) run concurrently, regardless of core count. Each unit boots a full
// httptest.Server plus a SQLite-backed AppConfig (temp dir, listening socket,
// several sqlite/WAL file descriptors) — that cost is I/O/fd/memory-bound,
// not CPU-bound, so letting core count alone dictate how many run at once
// would over-commit fd and memory limits on high-core machines (common on CI
// runners) for no throughput benefit.
const maxSectionParallelism = 8

// sectionParallelism returns how many independent env-backed units (scenarios,
// recorder cases) a CLI section may run concurrently. Two run modes force
// sequential execution: --batch measures per-request timing (parallel load
// would skew min/avg/max), and --record-dir captures request/response traffic
// for replay (concurrent runs would interleave recordings).
func (m *Matrix) sectionParallelism() int {
	if m.BatchCount > 1 || m.RecordDir != "" {
		return 1
	}
	if procs := runtime.GOMAXPROCS(0); procs < maxSectionParallelism {
		return procs
	}
	return maxSectionParallelism
}

// runIndexed runs fn(0..n-1) with at most parallelism concurrent calls.
// Callers write results into index-addressed slots, so output order stays
// deterministic regardless of completion order or scheduling.
func runIndexed(n, parallelism int, fn func(i int)) {
	var g errgroup.Group
	g.SetLimit(parallelism)
	for i := 0; i < n; i++ {
		g.Go(func() error {
			fn(i)
			return nil
		})
	}
	_ = g.Wait()
}

// executePerScenario is the shared CLI-side driver for sections that provision
// one TestEnv per scenario (single-hop, transitive, idempotent). Scenarios are
// fully independent (each gets its own env), so they run concurrently up to
// sectionParallelism; combos within a scenario share the env and run
// sequentially. Results are assembled in scenario order, so the output is
// identical to a sequential run (modulo durations).
func (m *Matrix) executePerScenario(skipScenario func(Scenario) bool, combosFor func(Scenario) []scenarioCombo) []TestResult {
	var scenarios []Scenario
	for _, s := range m.Scenarios {
		if skipScenario != nil && skipScenario(s) {
			continue
		}
		scenarios = append(scenarios, s)
	}

	perScenario := make([][]TestResult, len(scenarios))
	runIndexed(len(scenarios), m.sectionParallelism(), func(i int) {
		perScenario[i] = m.executeScenarioCombos(scenarios[i], combosFor)
	})

	var results []TestResult
	for _, rs := range perScenario {
		results = append(results, rs...)
	}
	return results
}

// executeScenarioCombos runs one scenario's combos against a fresh env.
// Routes are keyed by (source, target, scenario), so combos within a scenario
// cannot interfere and env boots stay off the hot path. When env creation
// fails, every combo is reported as a setup failure so a broken environment
// surfaces per-combination in the results table.
func (m *Matrix) executeScenarioCombos(scenario Scenario, combosFor func(Scenario) []scenarioCombo) []TestResult {
	combos := combosFor(scenario)
	results := make([]TestResult, 0, len(combos))

	env, err := NewTestEnvForCLI(m.testEnvOpts()...)
	if err != nil {
		for _, c := range combos {
			results = append(results, setupFailureResult(c.meta, err))
		}
		return results
	}
	defer env.Close()

	for _, c := range combos {
		results = append(results, c.run(env))
	}
	return results
}

// setupFailureResult returns base with its Errors set to a single "setup"
// AssertionError wrapping err — the result reported when the env itself
// fails to boot, shared by the scenario-combo and recorder-case drivers.
func setupFailureResult(base TestResult, err error) TestResult {
	base.Errors = []AssertionError{{
		Assertion: "setup",
		Error:     fmt.Sprintf("failed to create test env: %v", err),
	}}
	return base
}

// runPerScenario is the testing.T counterpart of executePerScenario: one env
// per scenario subtest (scenarios run in parallel, combos sequentially within
// each — limiting file-descriptor usage), with run receiving the live env.
func (m *Matrix) runPerScenario(t *testing.T, skipScenario func(Scenario) bool, run func(t *testing.T, env *TestEnv, s Scenario)) {
	t.Helper()

	for _, scenario := range m.Scenarios {
		if skipScenario != nil && skipScenario(scenario) {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			t.Parallel()

			env, err := NewTestEnvForCLI(m.testEnvOpts()...)
			if err != nil {
				t.Fatalf("create test env: %v", err)
			}
			defer env.Close()

			run(t, env, scenario)
		})
	}
}

// recorderCase is one flagTB-style case body (flags, content_shapes): a
// section-prefixed result name, the short case name shown in the CLI table's
// Scenario column, and the case body itself.
type recorderCase struct {
	name     string
	scenario string
	run      func(flagTB, *TestEnv)
}

// runRecorderCases executes recorder cases with bounded parallelism — each
// case boots its own env, so cases are fully independent — assembling results
// in case order.
func (m *Matrix) runRecorderCases(cases []recorderCase) []TestResult {
	results := make([]TestResult, len(cases))
	runIndexed(len(cases), m.sectionParallelism(), func(i int) {
		results[i] = runRecorderCase(cases[i].name, cases[i].scenario, cases[i].run)
	})
	return results
}

// runRecorderCase executes one flagTB-style case body against a fresh env,
// converting recorded failures into a CLI TestResult.
func runRecorderCase(name, scenario string, run func(flagTB, *TestEnv)) TestResult {
	res := TestResult{Name: name, Scenario: scenario}
	start := time.Now()

	env, err := NewTestEnvForCLI()
	if err != nil {
		res = setupFailureResult(res, err)
		res.Duration = time.Since(start)
		return res
	}
	defer env.Close()

	rec := &flagRecorder{}
	func() {
		defer func() {
			rec.runCleanups()
			if r := recover(); r != nil && r != flagAbort {
				panic(r)
			}
		}()
		run(rec, env)
	}()

	res.Errors = rec.errs
	res.Passed = len(rec.errs) == 0
	res.Duration = time.Since(start)
	return res
}
