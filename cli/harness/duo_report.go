package main

// Shared CLI scaffolding for the two duo-topology commands (`harness duo`,
// `harness routing`): booting the two-process environment, rendering blocks
// of DuoChecks, and finishing with JSON / PASS / non-zero exit.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// bootDuoEnv boots tb1+tb2 with the command's standard progress line and
// child-log relay wiring.
func bootDuoEnv(label string, jsonMode, verbose bool, cfg protocoltest.DuoEnvConfig) (*protocoltest.DuoEnv, error) {
	if verbose {
		cfg.ChildLog = os.Stderr
	}
	if !jsonMode {
		fmt.Printf("%s: booting tb1 (vmodel upstream) and tb2 (gateway) as server processes...\n", label)
	}
	env, err := protocoltest.NewDuoEnv(cfg)
	if err != nil {
		return nil, fmt.Errorf("boot duo environment: %w", err)
	}
	return env, nil
}

// printCheckBlock renders one named block of DuoChecks (a route, a routing
// scenario) and returns how many failed. Failing checks always print their
// detail line; verbose also prints the passing ones.
func printCheckBlock(name string, checks []protocoltest.DuoCheck, verbose bool) (failed int) {
	for _, c := range checks {
		if !c.Pass {
			failed++
		}
	}
	if failed == 0 {
		fmt.Printf("  ✔ %-22s %d checks\n", name, len(checks))
	} else {
		fmt.Printf("  ✘ %-22s %d/%d checks failed\n", name, failed, len(checks))
	}
	for _, c := range checks {
		if !c.Pass {
			fmt.Printf("      ✘ %-36s %s\n", c.Name, c.Detail)
		} else if verbose {
			fmt.Printf("      ✔ %-36s %s\n", c.Name, c.Detail)
		}
	}
	return failed
}

// emitDuoOutcome finishes a duo-topology command: the JSON payload when
// requested, the PASS line otherwise, and a non-zero exit on failure.
func emitDuoOutcome(jsonMode bool, payload any, pass bool, label string) error {
	if jsonMode {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return err
		}
	}
	if !pass {
		return fmt.Errorf("%s verification failed", label)
	}
	if !jsonMode {
		fmt.Printf("%s: PASS\n", label)
	}
	return nil
}
