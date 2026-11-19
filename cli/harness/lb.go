package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel"
	"gopkg.in/yaml.v3"
)

// lbExampleFS embeds the built-in scenario YAMLs under testdata/lb/. They are
// the single source of truth for the built-in examples — lb --example <name>
// parses them at runtime, so the on-disk YAML schema and the built-in examples
// can never drift. User-supplied --file scenarios use the same schema.
//
//go:embed testdata/lb/*.yaml
var lbExampleFS embed.FS

// lbExampleNames lists the built-in example names, derived from the
// testdata/lb/*.yaml basenames at init so adding an example is a one-file
// change. Used by --all and the unknown-example error message.
var lbExampleNames = listLBExamples()

func listLBExamples() []string {
	entries, err := lbExampleFS.ReadDir("testdata/lb")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".yaml"); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// LbCmd is Tier "LB" — it drives the real load-balancing path (selection +
// failover dispatch) against programmable fake upstreams over a request
// sequence, with a deterministic breaker clock, and prints the routing trace.
// It reuses the same simulation engine as the Go scenario tests
// (internal/server.LBSimulator). See .design/tier-routing.md
// "Rule config shapes (taxonomy)".
type LbCmd struct {
	File    string `kong:"name='file',short='f',help='Scenario YAML file (see --example for the schema)'"`
	Example string `kong:"name='example',help='Run a built-in example instead of --file (default: cascade); names are listed at the end of --help'"`
	All     bool   `kong:"name='all',help='Run all built-in examples in sequence (useful for CI)'"`
	JSON    bool   `kong:"name='json',help='Emit the trace as JSON'"`
	Table   bool   `kong:"name='table',short='t',help='Compact table view (default is the pencil graph)'"`
	Verbose bool   `kong:"name='verbose',short='v',help='Show gateway logs (default: quiet)'"`
}

// Help returns the long usage for `harness lb --help`; the example list is
// derived from testdata/lb so it cannot drift from the embedded scenarios.
func (*LbCmd) Help() string {
	return "Built-in examples (--example <name>, or --all to run each): " + strings.Join(lbExampleNames, ", ")
}

func (c *LbCmd) Run() error {
	if !c.Verbose {
		// config.NewConfig runs migrations/seeds at Info; keep the trace clean.
		logrus.SetLevel(logrus.WarnLevel)
	}

	// --all mode: run all built-in examples in sequence.
	if c.All {
		if c.File != "" || c.Example != "" {
			return fmt.Errorf("--all cannot be used with --file or --example")
		}
		// Ordered list of all built-in examples (matches testdata/lb/*.yaml).
		examples := lbExampleNames
		for i, name := range examples {
			// Clear the global breaker store between examples to avoid state leakage.
			loadbalance.DefaultBreakerStore().Reset()
			one := *c
			one.All = false
			one.Example = name
			fmt.Fprintf(os.Stderr, "▶ Running [%d/%d] %s\n", i+1, len(examples), name)
			if err := one.runOne(); err != nil {
				return fmt.Errorf("example %s failed: %w", name, err)
			}
		}
		fmt.Fprintf(os.Stderr, "✅ All %d lb examples passed\n", len(examples))
		return nil
	}

	return c.runOne()
}

// runOne runs a single scenario (either --file, --example, or default).
func (c *LbCmd) runOne() error {
	scn, err := c.loadScenario()
	if err != nil {
		return err
	}

	rule, err := buildLBRule(scn)
	if err != nil {
		return err
	}

	faults := make(map[string]vmodel.SequenceConfig, len(scn.Faults))
	for id, spec := range scn.Faults {
		faults[id] = spec.toConfig()
	}
	sim, cleanup, err := server.NewLBSimulatorWithSequences(rule, faults)
	if err != nil {
		return err
	}
	defer cleanup()

	if scn.SeedPin != nil {
		sim.SeedPin(scn.SeedPin.Session, scn.SeedPin.Provider, scn.SeedPin.Model)
	}

	out := lbRunOutput{
		Rule:     scn.RuleUUID,
		Tactic:   strings.ToLower(strings.TrimSpace(scn.Tactic)),
		Affinity: scn.AffinitySecs,
	}
	for _, svc := range rule.Services {
		out.Services = append(out.Services, fmt.Sprintf("%s (T%d%s)", svc.ServiceID(), svc.Tier, inactiveTag(svc.Active)))
	}

	// Track distinct first-attempt serviceIDs for DistinctFirstAttempts assertion.
	firstAttempts := make(map[string]bool)
	var lastTrace *server.LBTrace
	var lastSession string
	// Capture final pins per session for Pins assertion.
	finalPins := make(map[string]string)

	for _, step := range scn.Program {
		if step.Advance != "" {
			d, perr := time.ParseDuration(step.Advance)
			if perr != nil {
				return fmt.Errorf("program: bad advance duration %q: %w", step.Advance, perr)
			}
			sim.Advance(d)
			out.Steps = append(out.Steps, lbStepOutput{Advance: step.Advance})
			continue
		}
		session := ""
		if step.Request != nil {
			session = *step.Request
		}
		tr, rerr := sim.Request(session)
		if rerr != nil {
			return fmt.Errorf("program: request (session=%q): %w", session, rerr)
		}
		out.Steps = append(out.Steps, lbStepOutput{Trace: &tr})
		lastTrace = &tr
		lastSession = session
		// Track the first attempt of each request for DistinctFirstAttempts.
		if len(tr.Attempts) > 0 {
			firstAttempts[tr.Attempts[0]] = true
		}
		// Capture the final pin for this session.
		if session != "" {
			finalPins[session] = sim.Pin(session)
		}
	}
	out.FinalBreakers = sim.BreakerStates()
	out.FinalHealth = sim.HealthStates()

	// Self-check: evaluate Expect if provided.
	if scn.Expect != nil {
		finalPin := ""
		if lastTrace != nil && lastSession != "" {
			finalPin = sim.Pin(lastSession)
		}
		outcome := lbOutcome{
			lastTrace:     lastTrace,
			finalPin:      finalPin,
			finalPins:     finalPins,
			finalBreakers: out.FinalBreakers,
			finalHealth:   out.FinalHealth,
			firstAttempts: firstAttempts,
		}
		if cerr := scn.Expect.check(outcome); cerr != nil {
			return cerr
		}
	}

	// Ordered service IDs (rule order) for the per-request state line.
	orderedIDs := make([]string, 0, len(rule.Services))
	for _, svc := range rule.Services {
		orderedIDs = append(orderedIDs, svc.ServiceID())
	}

	switch {
	case c.JSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	case c.Table:
		out.renderTable(os.Stdout)
	default:
		out.renderGraph(os.Stdout, orderedIDs)
	}
	return nil
}

func (c *LbCmd) loadScenario() (*lbScenario, error) {
	if c.File != "" && c.Example != "" {
		return nil, fmt.Errorf("pass only one of --file / --example")
	}
	if c.File != "" {
		raw, err := os.ReadFile(c.File)
		if err != nil {
			return nil, fmt.Errorf("read scenario: %w", err)
		}
		var scn lbScenario
		if err := yaml.Unmarshal(raw, &scn); err != nil {
			return nil, fmt.Errorf("parse scenario: %w", err)
		}
		return &scn, nil
	}
	name := c.Example
	if name == "" {
		name = "cascade"
		fmt.Fprintln(os.Stderr, "no --file/--example given; running built-in example 'cascade' (see --help)")
	}
	// Load the built-in example from the embed FS. The embed path is
	// testdata/lb/<name>.yaml; path.Join uses '/'-separated paths even on
	// Windows because embed.FS always uses forward slashes.
	embPath := path.Join("testdata/lb", name+".yaml")
	raw, err := lbExampleFS.ReadFile(embPath)
	if err != nil {
		// Produce a helpful error that lists the available examples.
		return nil, fmt.Errorf("unknown --example %q (have: %s)", name, strings.Join(lbExampleNames, ", "))
	}
	var scn lbScenario
	if err := yaml.Unmarshal(raw, &scn); err != nil {
		return nil, fmt.Errorf("parse built-in example %s: %w", name, err)
	}
	return &scn, nil
}

// buildLBRule turns a scenario spec into a typ.Rule.
func buildLBRule(scn *lbScenario) (*typ.Rule, error) {
	if len(scn.Services) == 0 {
		return nil, fmt.Errorf("scenario has no services")
	}
	uuid := scn.RuleUUID
	if uuid == "" {
		uuid = "lb-scenario"
	}
	services := make([]*loadbalance.Service, 0, len(scn.Services))
	for _, s := range scn.Services {
		if s.Provider == "" || s.Model == "" {
			return nil, fmt.Errorf("service needs provider and model: %+v", s)
		}
		services = append(services, &loadbalance.Service{
			Provider: s.Provider, Model: s.Model, Tier: s.Tier, Weight: 1, Active: !s.Inactive,
		})
	}

	rule := &typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gpt-5.4",
		Active:       true,
		Services:     services,
	}
	rule.Flags.SessionAffinity = scn.AffinitySecs

	switch strings.ToLower(strings.TrimSpace(scn.Tactic)) {
	case "", "tier":
		params := typ.DefaultTierParams().(*typ.TierParams)
		if scn.WithinTierTactic != "" {
			params.WithinTierTactic = loadbalance.ParseTacticType(scn.WithinTierTactic)
		}
		rule.LBTactic = typ.Tactic{Type: loadbalance.TacticTier, Params: params}
	case "random":
		rule.LBTactic = typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.DefaultRandomParams()}
	default:
		return nil, fmt.Errorf("unsupported tactic %q (have: tier, random)", scn.Tactic)
	}
	return rule, nil
}
