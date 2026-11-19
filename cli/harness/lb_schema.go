package main

import (
	"gopkg.in/yaml.v3"

	"github.com/tingly-dev/tingly-box/vmodel"
)

// lbScenario is the YAML schema for a scenario file.
type lbScenario struct {
	RuleUUID         string                 `yaml:"rule_uuid"`
	Tactic           string                 `yaml:"tactic"`             // "tier" (default) | "random"
	WithinTierTactic string                 `yaml:"within_tier_tactic"` // "random" (default); tier tactic only
	AffinitySecs     int                    `yaml:"affinity_secs"`      // 0 = affinity off
	Services         []lbServiceSpec        `yaml:"services"`
	Faults           map[string]lbFaultSpec `yaml:"faults"`   // serviceID ("provider/model") -> fault program
	SeedPin          *lbSeedSpec            `yaml:"seed_pin"` // optional: lock a session before the program runs
	Program          []lbStep               `yaml:"program"`  // ordered request / advance steps
	Expect           *lbExpect              `yaml:"expect"`   // optional: self-check assertions
}

// lbFaultSpec is one service's fault program. It accepts two YAML shapes for
// backward compatibility and expressiveness, both backed by vmodel.Sequence:
//
//	faults:
//	  openai/gpt-5.4: [500, 500, 200]          # shorthand: status list (last repeats)
//	  openai/gpt-5.5:                           # structured form
//	    steps: [{status: 500, repeat: 3}, {status: 200}]
//	    on_exhaust: loop                        # loop | clamp (default) | fail
//
// The shorthand clamps (last status repeats), matching the historical faults
// semantics. Only each step's HTTP status affects LB/failover decisions; the
// content/error fields a vmodel step can carry are ignored by the simulator.
type lbFaultSpec struct {
	Steps     []vmodel.SequenceStep `yaml:"steps"`
	OnExhaust vmodel.ExhaustPolicy  `yaml:"on_exhaust"`
}

// UnmarshalYAML accepts either a bare status list (SequenceNode) or the
// structured {steps, on_exhaust} mapping.
func (f *lbFaultSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.SequenceNode {
		var statuses []int
		if err := node.Decode(&statuses); err != nil {
			return err
		}
		f.Steps = vmodel.Steps(statuses...)
		f.OnExhaust = vmodel.ExhaustClamp
		return nil
	}
	var raw struct {
		Steps     []vmodel.SequenceStep `yaml:"steps"`
		OnExhaust vmodel.ExhaustPolicy  `yaml:"on_exhaust"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	f.Steps = raw.Steps
	f.OnExhaust = raw.OnExhaust
	if f.OnExhaust == "" {
		f.OnExhaust = vmodel.ExhaustClamp // faults default: last step repeats
	}
	return nil
}

// toConfig converts the fault spec into the vmodel.SequenceConfig consumed by
// the simulator.
func (f lbFaultSpec) toConfig() vmodel.SequenceConfig {
	return vmodel.SequenceConfig{Steps: f.Steps, OnExhaust: f.OnExhaust}
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
