package protocoltest

import (
	"fmt"
	"testing"
	"time"
)

// This file holds the shared drivers behind the matrix's sections
// (single-hop, transitive, idempotent, flags, content_shapes). Each section
// contributes only its combos and per-combo execution; the env-per-scenario
// lifecycle, setup-failure fan-out, and testing.T scaffolding live here once.

// scenarioCombo is one runnable cell within a per-scenario section: the
// TestResult metadata that identifies it plus the function that executes it
// against a live env. meta doubles as the setup-failure result when the
// scenario's env cannot be created.
type scenarioCombo struct {
	meta TestResult
	run  func(env *TestEnv) TestResult
}

// executePerScenario is the shared CLI-side driver for sections that provision
// one TestEnv per scenario (single-hop, transitive, idempotent). Routes are
// keyed by (source, target, scenario), so combos within a scenario cannot
// interfere and env boots stay off the hot path. When env creation fails,
// every combo is reported as a setup failure so a broken environment surfaces
// per-combination in the results table.
func (m *Matrix) executePerScenario(skipScenario func(Scenario) bool, combosFor func(Scenario) []scenarioCombo) []TestResult {
	var results []TestResult
	for _, scenario := range m.Scenarios {
		if skipScenario != nil && skipScenario(scenario) {
			continue
		}
		combos := combosFor(scenario)

		env, err := NewTestEnvForCLI(m.testEnvOpts()...)
		if err != nil {
			for _, c := range combos {
				r := c.meta
				r.Errors = []AssertionError{{
					Assertion: "setup",
					Error:     fmt.Sprintf("failed to create test env: %v", err),
				}}
				results = append(results, r)
			}
			continue
		}

		for _, c := range combos {
			results = append(results, c.run(env))
		}
		env.Close()
	}
	return results
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

// runRecorderCase executes one flagTB-style case body (flags, content_shapes)
// against a fresh env, converting recorded failures into a CLI TestResult.
// Scenario carries the case's short name so the CLI table (which shows the
// Scenario column, not Name) distinguishes the rows; Name keeps the section
// prefix.
func runRecorderCase(name, scenario string, run func(flagTB, *TestEnv)) TestResult {
	res := TestResult{Name: name, Scenario: scenario}
	start := time.Now()

	env, err := NewTestEnvForCLI()
	if err != nil {
		res.Errors = []AssertionError{{Assertion: "setup", Error: fmt.Sprintf("create test env: %v", err)}}
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
