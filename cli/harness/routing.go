package main

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// RoutingCmd is Tier "Routing": smart-routing e2e verification on the duo
// topology. Rules (with smart-routing partitions) are created through tb2's
// production rule API; requests with controlled shapes are driven over real
// HTTP; every decision is asserted at wire level (which tb1 service-identity
// vmodel answered) and against the /api/v1/requests smart_routing trace —
// the same explanation surface users debug their routing configs with.
type RoutingCmd struct {
	Scenarios string `kong:"name='scenarios',default='all',help='Comma-separated built-in scenario names, or \"all\"'"`
	File      string `kong:"name='file',help='YAML file with user-defined scenarios (run instead of built-ins)'"`
	List      bool   `kong:"name='list',help='List built-in scenarios and exit'"`
	JSON      bool   `kong:"name='json',help='Emit results as JSON'"`
	Verbose   bool   `kong:"name='verbose',short='v',help='Relay child instance logs and show passing checks'"`
}

// routingResult is the JSON output shape.
type routingResult struct {
	TB1URL    string                 `json:"tb1_url"`
	TB2URL    string                 `json:"tb2_url"`
	Scenarios []routingScenarioBlock `json:"scenarios"`
	Pass      bool                   `json:"pass"`
}

type routingScenarioBlock struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Checks      []protocoltest.DuoCheck `json:"checks"`
	Pass        bool                    `json:"pass"`
}

func (cmd *RoutingCmd) Run() error {
	if cmd.List {
		for _, sc := range protocoltest.BuiltinRoutingScenarios() {
			fmt.Printf("%-22s %s\n", sc.Name, sc.Description)
		}
		return nil
	}

	var scenarios []*protocoltest.DuoRoutingScenario
	var err error
	if cmd.File != "" {
		scenarios, err = protocoltest.LoadRoutingScenarios(cmd.File)
	} else {
		var names []string
		for _, n := range strings.Split(cmd.Scenarios, ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
		scenarios, err = protocoltest.FindRoutingScenarios(names)
	}
	if err != nil {
		return err
	}

	env, err := bootDuoEnv("routing", cmd.JSON, cmd.Verbose, protocoltest.DuoEnvConfig{})
	if err != nil {
		return err
	}
	defer env.Close()

	result := routingResult{TB1URL: env.TB1.BaseURL, TB2URL: env.TB2.BaseURL, Pass: true}
	if !cmd.JSON {
		fmt.Printf("routing: %d scenarios (tb2 %s → tb1 %s)\n", len(scenarios), env.TB2.BaseURL, env.TB1.BaseURL)
	}

	for _, sc := range scenarios {
		checks := env.RunRoutingScenario(sc)
		block := routingScenarioBlock{Name: sc.Name, Description: sc.Description, Checks: checks, Pass: true}
		for _, c := range checks {
			if !c.Pass {
				block.Pass = false
				result.Pass = false
			}
		}
		result.Scenarios = append(result.Scenarios, block)
		if !cmd.JSON {
			printCheckBlock(sc.Name, checks, cmd.Verbose)
		}
	}

	return emitDuoOutcome(cmd.JSON, result, result.Pass, "routing")
}
