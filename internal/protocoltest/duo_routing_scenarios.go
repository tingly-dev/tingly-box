package protocoltest

// Built-in routing scenarios: one per smart-routing position category, plus
// evaluation-order and affinity-scoping regressions. Every scenario is
// self-checking (wire-level service identity + smart_routing trace).
//
// Deliberately not covered here:
//   - service_ttft / service_capacity — stats-driven positions that pass on
//     empty data; asserting real threshold behavior needs accumulated
//     runtime statistics, which these one-shot scenarios don't build up.
//   - proxy_vision — a processor-bearing op whose bypass semantics mutate
//     the request; it needs image fixtures and its own scenario shape.

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadRoutingScenarios reads user-defined scenarios from a YAML file:
//
//	scenarios:
//	  - name: my-rule
//	    rule:
//	      services: [{svc: b}]
//	      smart:
//	        - description: big
//	          ops: [{position: token, operation: ge, value: "50000"}]
//	          services: [{svc: a}]
//	    requests:
//	      - name: big
//	        body: {size_kb: 256}
//	        expect: {svc: a, outcome: matched, matched: big}
func LoadRoutingScenarios(path string) ([]*DuoRoutingScenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Scenarios []*DuoRoutingScenario `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(doc.Scenarios) == 0 {
		return nil, fmt.Errorf("%s: no scenarios defined", path)
	}
	for _, sc := range doc.Scenarios {
		if sc.Name == "" {
			return nil, fmt.Errorf("%s: scenario without a name", path)
		}
	}
	return doc.Scenarios, nil
}

// BuiltinRoutingScenarios returns the self-checking scenario catalog.
// Time-dependent scenarios are constructed relative to the current wall
// clock (the smart-routing clock is not injectable across processes).
func BuiltinRoutingScenarios() []*DuoRoutingScenario {
	return []*DuoRoutingScenario{
		routingPipelineHealthBeforeSmart(),
		routingPipelineSmartBeforeLoadBalancer(),
		routingPipelineAffinityBeforeLoadBalancer(),
		routingPipelineSmartBeforeAffinity(),
		routingTokenThreshold(),
		routingThinking(),
		routingContextKeyword(),
		routingModelGlob(),
		routingTimeRange(),
		routingFirstMatchOrder(),
		routingClaudeCodeKind(),
	}
}

var fullSelectionPipeline = []string{"health", "smart_routing", "affinity", "load_balancer"}

func routingPipelineHealthBeforeSmart() *DuoRoutingScenario {
	return &DuoRoutingScenario{
		Name:        "pipeline-health-before-smart-routing",
		Description: "health removes a rate-limited service before smart routing intersects the matched partition",
		Rule: DuoRoutingRule{
			LBTactic:         "tier",
			WithinTierTactic: "random",
			Services: []DuoRoutingService{
				{Svc: "b", Tier: 1},
				{Svc: "c", Tier: 2},
			},
			Smart: []DuoSmartPartition{{
				Description: "health-sensitive partition",
				Ops:         []DuoSmartOpSpec{{Position: "context_user", Operation: "contains", Value: "ROUTE-HEALTH"}},
				Services: []DuoRoutingService{
					{Model: FailMockPreContent429, Tier: 0},
					{Svc: "b", Tier: 1},
				},
			}},
		},
		Requests: []DuoRoutingRequest{
			{
				Name: "mark-primary-unhealthy", Body: DuoRoutingBody{UserText: "ROUTE-HEALTH setup"},
				Expect: DuoRoutingExpect{
					Svc: "b", Outcome: "matched", Matched: "health-sensitive partition",
					Source: "smart_routing", SelectedModel: FailMockPreContent429, Stages: fullSelectionPipeline,
				},
			},
			{
				Name: "health-filtered-before-smart", Body: DuoRoutingBody{UserText: "ROUTE-HEALTH verify"},
				Expect: DuoRoutingExpect{
					Svc: "b", Outcome: "matched", Matched: "health-sensitive partition",
					Source: "smart_routing", SelectedModel: DuoServiceModel("b"), Stages: fullSelectionPipeline,
				},
			},
		},
	}
}

// routingPipelineSmartBeforeLoadBalancer deliberately makes the global LB preference conflict
// with the smart partition. If LB ran first it would choose base tier-0 A;
// the expected tier-1 B proves SmartRouting narrowed the candidates before
// the terminal tier tactic selected within that subset. The no-match request
// is the control and must return to A under the same rule.
func routingPipelineSmartBeforeLoadBalancer() *DuoRoutingScenario {
	return &DuoRoutingScenario{
		Name:        "pipeline-smart-routing-before-load-balancer",
		Description: "smart routing narrows candidates before the tier load balancer selects",
		Rule: DuoRoutingRule{
			LBTactic:         "tier",
			WithinTierTactic: "random",
			Services:         []DuoRoutingService{{Svc: "a", Tier: 0}},
			Smart: []DuoSmartPartition{{
				Description: "smart partition",
				Ops:         []DuoSmartOpSpec{{Position: "context_user", Operation: "contains", Value: "ROUTE-SMART"}},
				Services: []DuoRoutingService{
					{Svc: "b", Tier: 1},
					{Svc: "c", Tier: 2},
				},
			}},
		},
		Requests: []DuoRoutingRequest{
			{
				Name: "match", Body: DuoRoutingBody{UserText: "please ROUTE-SMART now"},
				Expect: DuoRoutingExpect{
					Svc: "b", Outcome: "matched", Matched: "smart partition",
					Source: "smart_routing", Stages: fullSelectionPipeline,
				},
			},
			{
				Name: "no-match", Body: DuoRoutingBody{UserText: "ordinary request"},
				Expect: DuoRoutingExpect{
					Svc: "a", Outcome: "no_match", Source: "load_balancer", Stages: fullSelectionPipeline,
				},
			},
		},
	}
}

func routingPipelineAffinityBeforeLoadBalancer() *DuoRoutingScenario {
	const session = "duo-affinity-before-lb"
	return &DuoRoutingScenario{
		Name:        "pipeline-affinity-before-load-balancer",
		Description: "the first request creates a partition pin; the second terminates at affinity without evaluating load balancing",
		Rule: DuoRoutingRule{
			AffinitySecs: 300,
			LBTactic:     "tier",
			Services:     []DuoRoutingService{{Svc: "c", Tier: 2}},
			Smart: []DuoSmartPartition{{
				Description: "sticky partition",
				Ops:         []DuoSmartOpSpec{{Position: "context_user", Operation: "contains", Value: "ROUTE-STICKY"}},
				Services: []DuoRoutingService{
					{Svc: "a", Tier: 0},
					{Svc: "b", Tier: 1},
				},
			}},
		},
		Requests: []DuoRoutingRequest{
			{
				Name: "create-pin", Session: session, Body: DuoRoutingBody{UserText: "ROUTE-STICKY first"},
				Expect: DuoRoutingExpect{
					Svc: "a", Outcome: "matched", Matched: "sticky partition",
					Source: "smart_routing", Stages: fullSelectionPipeline,
				},
			},
			{
				Name: "affinity-short-circuit", Session: session, Body: DuoRoutingBody{UserText: "ROUTE-STICKY second"},
				Expect: DuoRoutingExpect{
					Svc: "a", Outcome: "matched", Matched: "sticky partition",
					Source: "affinity", Stages: []string{"health", "smart_routing", "affinity"},
				},
			},
		},
	}
}

// FindRoutingScenarios resolves scenario names ("all" or empty = every
// built-in).
func FindRoutingScenarios(names []string) ([]*DuoRoutingScenario, error) {
	all := BuiltinRoutingScenarios()
	if len(names) == 0 {
		return all, nil
	}
	byName := make(map[string]*DuoRoutingScenario, len(all))
	known := make([]string, 0, len(all))
	for _, sc := range all {
		byName[sc.Name] = sc
		known = append(known, sc.Name)
	}
	var out []*DuoRoutingScenario
	for _, n := range names {
		if n == "all" {
			return all, nil
		}
		sc, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("unknown routing scenario %q (known: %v)", n, known)
		}
		out = append(out, sc)
	}
	return out, nil
}

func svc(id string) DuoRoutingService { return DuoRoutingService{Svc: id} }

// routingTokenThreshold: context size decides the partition — the classic
// "route long-context turns to the big-window provider" rule.
func routingTokenThreshold() *DuoRoutingScenario {
	return &DuoRoutingScenario{
		Name:        "token-threshold",
		Description: "token ge threshold selects the large-context partition; small requests fall back",
		Rule: DuoRoutingRule{
			Services: []DuoRoutingService{svc("b")},
			Smart: []DuoSmartPartition{{
				Description: "large context",
				Ops:         []DuoSmartOpSpec{{Position: "token", Operation: "ge", Value: "50000"}},
				Services:    []DuoRoutingService{svc("a")},
			}},
		},
		Requests: []DuoRoutingRequest{
			{
				Name: "big", Body: DuoRoutingBody{SizeKB: 256},
				Expect: DuoRoutingExpect{Svc: "a", Outcome: "matched", Matched: "large context"},
			},
			{
				Name: "small", Beta: true, Body: DuoRoutingBody{SizeKB: 4},
				Expect: DuoRoutingExpect{Outcome: "no_match"},
			},
		},
	}
}

// routingThinking: the thinking flag (beta surface) selects a partition.
func routingThinking() *DuoRoutingScenario {
	return &DuoRoutingScenario{
		Name:        "thinking",
		Description: "thinking-enabled requests route to the reasoning partition",
		Rule: DuoRoutingRule{
			Services: []DuoRoutingService{svc("b")},
			Smart: []DuoSmartPartition{{
				Description: "reasoning",
				Ops:         []DuoSmartOpSpec{{Position: "thinking", Operation: "enabled"}},
				Services:    []DuoRoutingService{svc("a")},
			}},
		},
		Requests: []DuoRoutingRequest{
			{
				Name: "thinking-on", Beta: true, Body: DuoRoutingBody{Thinking: true},
				Expect: DuoRoutingExpect{Svc: "a", Outcome: "matched", Matched: "reasoning"},
			},
			{
				Name: "thinking-off", Beta: true, Body: DuoRoutingBody{},
				Expect: DuoRoutingExpect{Outcome: "no_match"},
			},
		},
	}
}

// routingContextKeyword: user-content matching, through a non-chat target so
// the partition also exercises the Responses conversion path.
func routingContextKeyword() *DuoRoutingScenario {
	return &DuoRoutingScenario{
		Name:        "context-keyword",
		Description: "context_user contains keyword selects the partition (responses-target service)",
		Rule: DuoRoutingRule{
			Services: []DuoRoutingService{svc("b")},
			Smart: []DuoSmartPartition{{
				Description: "keyword",
				Ops:         []DuoSmartOpSpec{{Position: "context_user", Operation: "contains", Value: "ROUTE-EMERALD"}},
				Services:    []DuoRoutingService{{Svc: "a", Target: "responses"}},
			}},
		},
		Requests: []DuoRoutingRequest{
			{
				Name: "with-keyword", Body: DuoRoutingBody{UserText: "please handle ROUTE-EMERALD for me"},
				Expect: DuoRoutingExpect{Svc: "a", Outcome: "matched", Matched: "keyword"},
			},
			{
				Name: "without-keyword", Body: DuoRoutingBody{UserText: "an ordinary request"},
				Expect: DuoRoutingExpect{Outcome: "no_match"},
			},
		},
	}
}

// routingModelGlob: the model position sees the request's model name.
func routingModelGlob() *DuoRoutingScenario {
	return &DuoRoutingScenario{
		Name:        "model-glob",
		Description: "model glob matches the request model",
		Rule: DuoRoutingRule{
			Services: []DuoRoutingService{svc("b")},
			Smart: []DuoSmartPartition{{
				Description: "duo models",
				Ops:         []DuoSmartOpSpec{{Position: "model", Operation: "glob", Value: "duo-route-*"}},
				Services:    []DuoRoutingService{svc("a")},
			}},
		},
		Requests: []DuoRoutingRequest{{
			Name: "glob-hit", Body: DuoRoutingBody{},
			Expect: DuoRoutingExpect{Svc: "a", Outcome: "matched", Matched: "duo models"},
		}},
	}
}

// duoTimeWindow builds a time_range op value for a window at the given
// offsets from now (UTC). Overnight wrap (start > end) is supported by the
// evaluator, so offsets crossing midnight are fine.
func duoTimeWindow(startOffset, endOffset time.Duration) string {
	now := time.Now().UTC()
	return fmt.Sprintf(`{"start":%q,"end":%q,"timezone":"UTC"}`,
		now.Add(startOffset).Format("15:04"), now.Add(endOffset).Format("15:04"))
}

// routingTimeRange: two windows — one that cannot contain now, one that
// does — verifying both the miss and the (ordered) hit. Windows are hours
// wide, so wall-clock drift during the run cannot flip the outcome.
func routingTimeRange() *DuoRoutingScenario {
	return &DuoRoutingScenario{
		Name:        "time-range",
		Description: "time_range windows relative to now: closed window skipped, open window matched",
		Rule: DuoRoutingRule{
			Services: []DuoRoutingService{svc("c")},
			Smart: []DuoSmartPartition{
				{
					Description: "future window",
					Ops:         []DuoSmartOpSpec{{Position: "time", Operation: "time_range", Value: duoTimeWindow(2*time.Hour, 3*time.Hour)}},
					Services:    []DuoRoutingService{svc("a")},
				},
				{
					Description: "current window",
					Ops:         []DuoSmartOpSpec{{Position: "time", Operation: "time_range", Value: duoTimeWindow(-2*time.Hour, 2*time.Hour)}},
					Services:    []DuoRoutingService{svc("b")},
				},
			},
		},
		Requests: []DuoRoutingRequest{{
			Name: "now", Body: DuoRoutingBody{},
			Expect: DuoRoutingExpect{Svc: "b", Outcome: "matched", Matched: "current window"},
		}},
	}
}

// routingFirstMatchOrder: a request matching two partitions lands on the
// first — rule order is the tie-breaker, not specificity.
func routingFirstMatchOrder() *DuoRoutingScenario {
	return &DuoRoutingScenario{
		Name:        "first-match-order",
		Description: "when two partitions match, the first wins",
		Rule: DuoRoutingRule{
			Services: []DuoRoutingService{svc("c")},
			Smart: []DuoSmartPartition{
				{
					Description: "first",
					Ops:         []DuoSmartOpSpec{{Position: "token", Operation: "ge", Value: "10"}},
					Services:    []DuoRoutingService{svc("a")},
				},
				{
					Description: "second",
					Ops:         []DuoSmartOpSpec{{Position: "context_user", Operation: "contains", Value: "MATCH-BOTH"}},
					Services:    []DuoRoutingService{svc("b")},
				},
			},
		},
		Requests: []DuoRoutingRequest{{
			Name: "matches-both", Body: DuoRoutingBody{UserText: "this request should MATCH-BOTH partitions and take the first"},
			Expect: DuoRoutingExpect{Svc: "a", Outcome: "matched", Matched: "first"},
		}},
	}
}

// routingClaudeCodeKind: the claude_code scenario fingerprints the system
// prompt into main/subagent/compact; partitions split subagent and compact
// traffic while main falls back to the base pool.
func routingClaudeCodeKind() *DuoRoutingScenario {
	return &DuoRoutingScenario{
		Name:        "claude-code-kind",
		Description: "agent.claude_code request-kind detection routes subagent and compact traffic",
		Rule: DuoRoutingRule{
			Scenario: "claude_code",
			Services: []DuoRoutingService{svc("c")},
			Smart: []DuoSmartPartition{
				{
					Description: "subagents",
					Ops:         []DuoSmartOpSpec{{Position: "agent.claude_code", Operation: "equals", Value: "subagent"}},
					Services:    []DuoRoutingService{svc("a")},
				},
				{
					Description: "compaction",
					Ops:         []DuoSmartOpSpec{{Position: "agent.claude_code", Operation: "equals", Value: "compact"}},
					Services:    []DuoRoutingService{svc("b")},
				},
			},
		},
		Requests: []DuoRoutingRequest{
			{
				Name: "subagent", Beta: true,
				Body:   DuoRoutingBody{System: "You are an agent spawned to handle a focused sub-task for the duo harness."},
				Expect: DuoRoutingExpect{Svc: "a", Outcome: "matched", Matched: "subagents"},
			},
			{
				Name: "compact", Beta: true,
				Body:   DuoRoutingBody{System: "Provide a structured summary of the conversation above, preserving decisions and open questions."},
				Expect: DuoRoutingExpect{Svc: "b", Outcome: "matched", Matched: "compaction"},
			},
			{
				Name: "main", Beta: true,
				Body:   DuoRoutingBody{System: "You are Claude Code, Anthropic's official CLI for Claude."},
				Expect: DuoRoutingExpect{Outcome: "no_match"},
			},
		},
	}
}

// routingPipelineSmartBeforeAffinity is the G3 regression: with session affinity on,
// a pin acquired in one content partition must not drag requests of the
// other partition — content routing wins, pins are scoped per partition.
func routingPipelineSmartBeforeAffinity() *DuoRoutingScenario {
	const session = "duo-g3-session"
	big := DuoRoutingBody{SizeKB: 256}
	small := DuoRoutingBody{SizeKB: 4}
	return &DuoRoutingScenario{
		Name:        "pipeline-smart-routing-before-affinity",
		Description: "G3: session pins are scoped per smart partition; content routing beats a cross-partition pin",
		Rule: DuoRoutingRule{
			AffinitySecs: 300,
			Services:     []DuoRoutingService{svc("c")},
			Smart: []DuoSmartPartition{
				{
					Description: "big",
					Ops:         []DuoSmartOpSpec{{Position: "token", Operation: "ge", Value: "50000"}},
					Services:    []DuoRoutingService{svc("a")},
				},
				{
					Description: "small",
					Ops:         []DuoSmartOpSpec{{Position: "token", Operation: "lt", Value: "50000"}},
					Services:    []DuoRoutingService{svc("b")},
				},
			},
		},
		Requests: []DuoRoutingRequest{
			{
				Name: "big-1", Session: session, Body: big,
				Expect: DuoRoutingExpect{
					Svc: "a", Outcome: "matched", Matched: "big",
					Source: "smart_routing", Stages: fullSelectionPipeline,
				},
			},
			{
				Name: "small-after-pin", Session: session, Body: small,
				Expect: DuoRoutingExpect{
					Svc: "b", Outcome: "matched", Matched: "small",
					Source: "smart_routing", Stages: fullSelectionPipeline,
				},
			},
			{
				Name: "big-2", Session: session, Body: big,
				Expect: DuoRoutingExpect{
					Svc: "a", Outcome: "matched", Matched: "big",
					Source: "affinity", Stages: []string{"health", "smart_routing", "affinity"},
				},
			},
		},
	}
}
