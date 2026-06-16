package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// LbCmd is Tier "LB" — it drives the real load-balancing path (selection +
// failover dispatch) against programmable fake upstreams over a request
// sequence, with a deterministic breaker clock, and prints the routing trace.
// It reuses the same simulation engine as the Go scenario tests
// (internal/server.LBSimulator). See .design/priority-routing.md
// "Rule config shapes (taxonomy)".
type LbCmd struct {
	File    string `kong:"name='file',short='f',help='Scenario YAML file (see --example for the schema)'"`
	Example string `kong:"name='example',help='Run a built-in example instead of --file: cascade|flat|grid|single|regression|ratelimit|authflip'"`
	JSON    bool   `kong:"name='json',help='Emit the trace as JSON'"`
	Table   bool   `kong:"name='table',short='t',help='Compact table view (default is the pencil graph)'"`
	Verbose bool   `kong:"name='verbose',short='v',help='Show gateway logs (default: quiet)'"`
}

// lbScenario is the YAML schema for a scenario file.
type lbScenario struct {
	RuleUUID     string           `yaml:"rule_uuid"`
	Tactic       string           `yaml:"tactic"`        // "tier" (default) | "random"
	AffinitySecs int              `yaml:"affinity_secs"` // 0 = affinity off
	Services     []lbServiceSpec  `yaml:"services"`
	Faults       map[string][]int `yaml:"faults"`   // serviceID ("provider/model") -> per-call status sequence (last repeats)
	SeedPin      *lbSeedSpec      `yaml:"seed_pin"` // optional: lock a session before the program runs
	Program      []lbStep         `yaml:"program"`  // ordered request / advance steps
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

func (c *LbCmd) Run() error {
	if !c.Verbose {
		// config.NewConfig runs migrations/seeds at Info; keep the trace clean.
		logrus.SetLevel(logrus.WarnLevel)
	}

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
	}
	out.FinalBreakers = sim.BreakerStates()
	out.FinalHealth = sim.HealthStates()

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
	scn, ok := lbExamples[name]
	if !ok {
		return nil, fmt.Errorf("unknown --example %q (have: cascade, flat, grid, single, regression, ratelimit, authflip)", name)
	}
	return scn, nil
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
		RequestModel: "gpt-4",
		Active:       true,
		Services:     services,
	}
	rule.Flags.SessionAffinity = scn.AffinitySecs

	switch strings.ToLower(strings.TrimSpace(scn.Tactic)) {
	case "", "tier":
		rule.LBTactic = typ.Tactic{Type: loadbalance.TacticTier, Params: typ.DefaultTierParams()}
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
// the affinity pin) — mirroring .design/priority-routing.pencil.md.
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

var lbExamples = map[string]*lbScenario{
	// Cascade: primary fails 3x then recovers; watch the pin move to t1 and snap
	// back to t0 after the breaker's open window elapses.
	"cascade": {
		RuleUUID: "cascade", Tactic: "tier", AffinitySecs: 1800,
		Services: []lbServiceSpec{{Provider: "t0", Model: "gpt-4", Tier: 0}, {Provider: "t1", Model: "gpt-4", Tier: 1}},
		Faults:   map[string][]int{"t0/gpt-4": {500, 500, 500, 200}, "t1/gpt-4": {200}},
		Program: []lbStep{
			{Request: sess("s1")}, {Request: sess("s1")}, {Request: sess("s1")},
			{Request: sess("s1")},
			{Advance: "31s"},
			{Request: sess("s1")}, {Request: sess("s1")},
		},
	},
	// Flat: one tier, two peers, random tactic — affinity stickiness to one peer.
	"flat": {
		RuleUUID: "flat", Tactic: "random", AffinitySecs: 1800,
		Services: []lbServiceSpec{{Provider: "a", Model: "gpt-4", Tier: 0}, {Provider: "b", Model: "gpt-4", Tier: 0}},
		Faults:   map[string][]int{"a/gpt-4": {200}, "b/gpt-4": {200}},
		Program:  []lbStep{{Request: sess("s1")}, {Request: sess("s1")}, {Request: sess("s1")}},
	},
	// Grid: two tiers x two peers; the whole top tier 500s and trips, then
	// selection drops to the low tier.
	"grid": {
		RuleUUID: "grid", Tactic: "tier", AffinitySecs: 0,
		Services: []lbServiceSpec{
			{Provider: "t0a", Model: "gpt-4", Tier: 0}, {Provider: "t0b", Model: "gpt-4", Tier: 0},
			{Provider: "t1a", Model: "gpt-4", Tier: 1}, {Provider: "t1b", Model: "gpt-4", Tier: 1},
		},
		Faults: map[string][]int{"t0a/gpt-4": {500}, "t0b/gpt-4": {500}, "t1a/gpt-4": {200}, "t1b/gpt-4": {200}},
		Program: []lbStep{
			{Request: sess("")}, {Request: sess("")}, {Request: sess("")},
			{Request: sess("")}, {Request: sess("")},
		},
	},
	// Single: one service; a 500 has nowhere to fail over to.
	"single": {
		RuleUUID: "single", Tactic: "tier", AffinitySecs: 0,
		Services: []lbServiceSpec{{Provider: "only", Model: "gpt-4", Tier: 0}},
		Faults:   map[string][]int{"only/gpt-4": {200, 500, 200}},
		Program:  []lbStep{{Request: sess("")}, {Request: sess("")}, {Request: sess("")}},
	},
	// Regression: a stale pin to the fallback tier must snap back to the healthy
	// primary on the next request.
	"regression": {
		RuleUUID: "regression", Tactic: "tier", AffinitySecs: 1800,
		Services: []lbServiceSpec{{Provider: "t1", Model: "gpt-4", Tier: 0}, {Provider: "t2", Model: "gpt-4", Tier: 1}},
		Faults:   map[string][]int{"t1/gpt-4": {200}, "t2/gpt-4": {200}},
		SeedPin:  &lbSeedSpec{Session: "s1", Provider: "t2", Model: "gpt-4"}, // stale pin to the fallback tier
		Program:  []lbStep{{Request: sess("s1")}, {Request: sess("s1")}},
	},
	// Rate-limit: a single 429 marks t0 unhealthy via the health monitor, so it's
	// skipped on the NEXT request even though its breaker has one strike — then it
	// recovers after the rate-limit window.
	"ratelimit": {
		RuleUUID: "ratelimit", Tactic: "tier", AffinitySecs: 0,
		Services: []lbServiceSpec{{Provider: "t0", Model: "gpt-4", Tier: 0}, {Provider: "t1", Model: "gpt-4", Tier: 1}},
		Faults:   map[string][]int{"t0/gpt-4": {429, 200}, "t1/gpt-4": {200}},
		Program: []lbStep{
			{Request: sess("")}, // t0 429 → fail over to t1; t0 rate-limited
			{Request: sess("")}, // t0 health-excluded → straight to t1
			{Advance: "31s"},
			{Request: sess("")}, // window elapsed → back to t0
		},
	},
	// Auth flip: 401 is terminal (no failover masks it) AND marks t0 immediately
	// unhealthy, so it's excluded next request.
	"authflip": {
		RuleUUID: "authflip", Tactic: "tier", AffinitySecs: 0,
		Services: []lbServiceSpec{{Provider: "t0", Model: "gpt-4", Tier: 0}, {Provider: "t1", Model: "gpt-4", Tier: 1}},
		Faults:   map[string][]int{"t0/gpt-4": {401, 200}, "t1/gpt-4": {200}},
		Program: []lbStep{
			{Request: sess("")}, // t0 401 → terminal (client sees 401); t0 unhealthy
			{Request: sess("")}, // t0 excluded → t1
		},
	},
}
