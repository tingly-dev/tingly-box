package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// lbExampleFS embeds the built-in scenario YAMLs under testdata/lb/. They are
// the single source of truth for the built-in examples — lb --example <name>
// parses them at runtime, so the on-disk YAML schema and the built-in examples
// can never drift. User-supplied --file scenarios use the same schema.
//
//go:embed testdata/lb/*.yaml
var lbExampleFS embed.FS

// lbExampleNames is the canonical ordered list of built-in example names
// (matches the testdata/lb/*.yaml basenames). Used by --all and the unknown-
// example error message.
var lbExampleNames = []string{
	"cascade", "flat", "grid", "single", "regression", "ratelimit", "authflip",
	"crossmodel", "halfopen", "degrade", "inactive", "withintier", "multiaffinity",
}

// LbCmd is Tier "LB" — it drives the real load-balancing path (selection +
// failover dispatch) against programmable fake upstreams over a request
// sequence, with a deterministic breaker clock, and prints the routing trace.
// It reuses the same simulation engine as the Go scenario tests
// (internal/server.LBSimulator). See .design/tier-routing.md
// "Rule config shapes (taxonomy)".
type LbCmd struct {
	File    string `kong:"name='file',short='f',help='Scenario YAML file (see --example for the schema)'"`
	Example string `kong:"name='example',help='Run a built-in example instead of --file: cascade|flat|grid|single|regression|ratelimit|authflip|crossmodel|halfopen|degrade|inactive|withintier|multiaffinity'"`
	All     bool   `kong:"name='all',help='Run all built-in examples in sequence (useful for CI)'"`
	JSON    bool   `kong:"name='json',help='Emit the trace as JSON'"`
	Table   bool   `kong:"name='table',short='t',help='Compact table view (default is the pencil graph)'"`
	Verbose bool   `kong:"name='verbose',short='v',help='Show gateway logs (default: quiet)'"`
}

// lbScenario is the YAML schema for a scenario file.
type lbScenario struct {
	RuleUUID         string           `yaml:"rule_uuid"`
	Tactic           string           `yaml:"tactic"`             // "tier" (default) | "random"
	WithinTierTactic string           `yaml:"within_tier_tactic"` // "random" (default) | "token_based" | ...; tier tactic only
	AffinitySecs     int              `yaml:"affinity_secs"`      // 0 = affinity off
	Services         []lbServiceSpec  `yaml:"services"`
	Faults           map[string][]int `yaml:"faults"`   // serviceID ("provider/model") -> per-call status sequence (last repeats)
	SeedPin          *lbSeedSpec      `yaml:"seed_pin"` // optional: lock a session before the program runs
	Program          []lbStep         `yaml:"program"`  // ordered request / advance steps
	Expect           *lbExpect        `yaml:"expect"`   // optional: self-check assertions
}

// lbSeedSpec pre-locks a session to a service (e.g. to reproduce a stale pin).
type lbSeedSpec struct {
	Session  string `yaml:"session"`
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

type lbServiceSpec struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	Tier     int    `yaml:"tier"`
	Inactive bool   `yaml:"inactive"`
}

// lbStep is one program step: exactly one of Request / Advance.
type lbStep struct {
	Request *string `yaml:"request"` // session id (use "" for no affinity)
	Advance string  `yaml:"advance"` // duration, e.g. "31s"
}

// lbExpect is the optional self-check block. All fields optional; a missing
// field is not asserted. Evaluated against the LAST request's trace plus the
// FINAL snapshots after the program runs.
type lbExpect struct {
	// FinalStatus is the last request's FinalStatus.
	FinalStatus *int `yaml:"final_status"`
	// Attempts is the last request's Attempts (exact, in order).
	Attempts []string `yaml:"attempts"`
	// AttemptsContain is the last request's Attempts must include each (set membership, order-independent).
	AttemptsContain []string `yaml:"attempts_contain"`
	// AttemptsExclude is the last request's Attempts must NOT include any.
	AttemptsExclude []string `yaml:"attempts_exclude"`
	// Pin is the final affinity pin for the last request's session ("" = no pin).
	Pin *string `yaml:"pin"`
	// Pins is final pin per named session (multi-session).
	Pins map[string]string `yaml:"pins"`
	// Breaker is final breaker snapshot subset (serviceID -> closed|open|half_open).
	Breaker map[string]string `yaml:"breaker"`
	// Health is final health snapshot subset (serviceID -> healthy|unhealthy).
	Health map[string]string `yaml:"health"`
	// DistinctFirstAttempts is the set of distinct serviceIDs that ever appeared as
	// the FIRST attempt across ALL request steps (within-tier load sharing).
	DistinctFirstAttempts []string `yaml:"distinct_first_attempts"`
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
			// Temporarily set Example and clear File/All for this run.
			origFile := c.File
			origAll := c.All
			c.File = ""
			c.All = false
			c.Example = name
			fmt.Fprintf(os.Stderr, "▶ Running [%d/%d] %s\n", i+1, len(examples), name)
			if err := c.runOne(); err != nil {
				return fmt.Errorf("example %s failed: %w", name, err)
			}
			// Restore original values.
			c.File = origFile
			c.All = origAll
			c.Example = ""
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

	sim, cleanup, err := server.NewLBSimulator(rule, scn.Faults)
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
		if cerr := c.checkExpect(scn, out.Steps, out.FinalBreakers, out.FinalHealth, lastTrace, lastSession, finalPin, finalPins, firstAttempts); cerr != nil {
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

// checkExpect evaluates the Expect block after the program runs. On mismatch,
// returns an error → non-zero exit (CI leg fails).
func (c *LbCmd) checkExpect(scn *lbScenario, steps []lbStepOutput, finalBreakers, finalHealth map[string]string, lastTrace *server.LBTrace, lastSession string, finalPin string, finalPins map[string]string, firstAttempts map[string]bool) error {
	e := scn.Expect
	if e == nil {
		return nil
	}

	// Helper: string pointer to string (or "" if nil).
	ptrStr := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	// Helper: int pointer to int (or 0 if nil).
	ptrInt := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}

	// FinalStatus: last request's FinalStatus.
	if e.FinalStatus != nil {
		got := 0
		if lastTrace != nil {
			got = lastTrace.FinalStatus
		}
		if want := ptrInt(e.FinalStatus); got != want {
			return fmt.Errorf("expect: final_status: want %d, got %d", want, got)
		}
	}

	// Attempts: last request's Attempts (exact, in order).
	if len(e.Attempts) > 0 {
		got := []string{}
		if lastTrace != nil {
			got = lastTrace.Attempts
		}
		if !equalSlices(got, e.Attempts) {
			return fmt.Errorf("expect: attempts: want %v, got %v", e.Attempts, got)
		}
	}

	// AttemptsContain: last request must include each serviceID (set membership).
	for _, wantID := range e.AttemptsContain {
		found := false
		if lastTrace != nil {
			for _, gotID := range lastTrace.Attempts {
				if gotID == wantID {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("expect: attempts_contain: service %q not found in attempts (got %v)", wantID, lastTrace.Attempts)
		}
	}

	// AttemptsExclude: last request must NOT include any serviceID.
	for _, banID := range e.AttemptsExclude {
		if lastTrace != nil {
			for _, gotID := range lastTrace.Attempts {
				if gotID == banID {
					return fmt.Errorf("expect: attempts_exclude: service %q should not appear in attempts (got %v)", banID, lastTrace.Attempts)
				}
			}
		}
	}

	// Pin: final pin for the last request's session.
	if e.Pin != nil {
		wantPin := ptrStr(e.Pin)
		if finalPin != wantPin {
			return fmt.Errorf("expect: pin: want %q, got %q", wantPin, finalPin)
		}
	}

	// Pins: final pin per named session.
	for sess, wantPin := range e.Pins {
		gotPin, ok := finalPins[sess]
		if !ok {
			return fmt.Errorf("expect: pins: session %q not found in program", sess)
		}
		if gotPin != wantPin {
			return fmt.Errorf("expect: pins[%s]: want %q, got %q", sess, wantPin, gotPin)
		}
	}

	// Breaker: final breaker snapshot subset.
	for sid, wantState := range e.Breaker {
		gotState, ok := finalBreakers[sid]
		if !ok {
			return fmt.Errorf("expect: breaker: service %q not found in final breaker snapshot", sid)
		}
		if gotState != wantState {
			return fmt.Errorf("expect: breaker[%s]: want %s, got %s", sid, wantState, gotState)
		}
	}

	// Health: final health snapshot subset.
	for sid, wantState := range e.Health {
		gotState, ok := finalHealth[sid]
		if !ok {
			return fmt.Errorf("expect: health: service %q not found in final health snapshot", sid)
		}
		if gotState != wantState {
			return fmt.Errorf("expect: health[%s]: want %s, got %s", sid, wantState, gotState)
		}
	}

	// DistinctFirstAttempts: set of first-attempt serviceIDs across ALL request steps.
	if len(e.DistinctFirstAttempts) > 0 {
		wantSet := make(map[string]bool)
		for _, id := range e.DistinctFirstAttempts {
			wantSet[id] = true
		}
		for gotID := range firstAttempts {
			if !wantSet[gotID] {
				return fmt.Errorf("expect: distinct_first_attempts: unexpected service %q appeared (got set %v)", gotID, firstAttempts)
			}
		}
		for wantID := range wantSet {
			if !firstAttempts[wantID] {
				return fmt.Errorf("expect: distinct_first_attempts: expected service %q missing (got set %v)", wantID, firstAttempts)
			}
		}
	}

	return nil
}

// equalSlices compares two string slices for equality.
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

func inactiveTag(active bool) string {
	if active {
		return ""
	}
	return ",inactive"
}

// ---- output ----

type lbRunOutput struct {
	Rule          string            `json:"rule"`
	Tactic        string            `json:"tactic"`
	Affinity      int               `json:"affinity_secs"`
	Services      []string          `json:"services"`
	Steps         []lbStepOutput    `json:"steps"`
	FinalBreakers map[string]string `json:"final_breakers"`
	FinalHealth   map[string]string `json:"final_health"`
}

type lbStepOutput struct {
	Trace   *server.LBTrace `json:"trace,omitempty"`
	Advance string          `json:"advance,omitempty"`
}

func (o lbRunOutput) renderTable(w *os.File) {
	tactic := o.Tactic
	if tactic == "" {
		tactic = "tier"
	}
	fmt.Fprintf(w, "LB scenario %q  tactic=%s  affinity=%ds\n", o.Rule, tactic, o.Affinity)
	fmt.Fprintf(w, "services: %s\n\n", strings.Join(o.Services, "  "))
	fmt.Fprintf(w, "%-4s %-10s %-34s %-7s %s\n", "#", "session", "attempts", "status", "pin")

	n := 0
	for _, st := range o.Steps {
		if st.Advance != "" {
			fmt.Fprintf(w, "     -- advance %s --\n", st.Advance)
			continue
		}
		n++
		tr := st.Trace
		sess := tr.Session
		if sess == "" {
			sess = "-"
		}
		fmt.Fprintf(w, "%-4d %-10s %-34s %-7d %s\n",
			n, sess, strings.Join(tr.Attempts, " → "), tr.FinalStatus, dashIfEmpty(tr.PinAfter))
	}

	fmt.Fprintf(w, "\nfinal breakers: %s\n", formatBreakers(o.FinalBreakers))
	fmt.Fprintf(w, "final health:   %s\n", formatBreakers(o.FinalHealth))
}

// renderGraph prints a pencil-graph view: per request a hop line (failover path
// annotated with ✓/✗ + status) and a state line (each svc's breaker/health +
// the affinity pin) — mirroring .design/tier-routing.pencil.md.
func (o lbRunOutput) renderGraph(w *os.File, orderedIDs []string) {
	tactic := o.Tactic
	if tactic == "" {
		tactic = "tier"
	}
	fmt.Fprintf(w, "LB scenario %q  tactic=%s  affinity=%ds\n", o.Rule, tactic, o.Affinity)
	fmt.Fprintf(w, "services: %s\n", strings.Join(o.Services, "  "))
	fmt.Fprintf(w, "legend:   ✓=2xx committed   ✗=non-2xx (buffered/terminal)   →=in-request failover hop\n\n")

	n := 0
	for _, st := range o.Steps {
		if st.Advance != "" {
			fmt.Fprintf(w, "        ⋯ advance %s (breaker + health clocks move) ⋯\n\n", st.Advance)
			continue
		}
		n++
		tr := st.Trace

		hops := make([]string, len(tr.Attempts))
		for i, sid := range tr.Attempts {
			status := 0
			if i < len(tr.Statuses) {
				status = tr.Statuses[i]
			}
			mark := "✗"
			if status >= 200 && status < 300 {
				mark = "✓"
			}
			hops[i] = fmt.Sprintf("%s %s%d", sid, mark, status)
		}
		sess := tr.Session
		if sess == "" {
			sess = "-"
		}
		fmt.Fprintf(w, "#%-2d %-8s %s   →  client=%d\n", n, sess, strings.Join(hops, "  →  "), tr.FinalStatus)

		states := make([]string, 0, len(orderedIDs))
		for _, id := range orderedIDs {
			states = append(states, fmt.Sprintf("%s=%s/%s", id, tr.BreakerAfter[id], tr.HealthAfter[id]))
		}
		line := strings.Join(states, "   ")
		if tr.PinAfter != "" {
			line += "   pin=" + tr.PinAfter
		}
		fmt.Fprintf(w, "       state: %s\n\n", line)
	}
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatBreakers(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for id, st := range m {
		parts = append(parts, fmt.Sprintf("%s=%s", id, st))
	}
	return strings.Join(parts, "  ")
}

// ---- built-in examples ----

func sess(s string) *string { return &s }

func ptrInt(i int) *int       { return &i }
func ptrStr(s string) *string { return &s }
