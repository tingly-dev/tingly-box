package main

import (
	"fmt"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/protocoltest"
	"github.com/tingly-dev/tingly-box/vmodel/spec"
)

// scenariosFromDefinitions loads VModel definition files, compiles them into
// one runtime and adapts every definition (or only the ids in only, when
// given) to a matrix scenario. It uses the same loader as `tb vmodel --config`
// and rejects definitions without an expect block, since the matrix has
// nothing to assert for them.
func scenariosFromDefinitions(paths, only []string) ([]protocoltest.Scenario, error) {
	var definitions []spec.Definition
	for _, path := range paths {
		loaded, err := spec.Load(path)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, loaded...)
	}
	runtime, err := spec.New(definitions)
	if err != nil {
		return nil, err
	}

	var scenarios []protocoltest.Scenario
	seen := make(map[string]bool)
	for _, id := range runtime.IDs() {
		if len(only) > 0 && !slices.Contains(only, id) {
			continue
		}
		scn, err := runtime.Scenario(id)
		if err != nil {
			return nil, err
		}
		if len(scn.Assertions) == 0 {
			return nil, fmt.Errorf("definition %q requires at least one expect assertion for matrix tests", scn.Name)
		}
		seen[scn.Name] = true
		scenarios = append(scenarios, scn)
	}
	for _, name := range only {
		if !seen[name] {
			return nil, fmt.Errorf("scenario %q not found in --config files", name)
		}
	}
	return scenarios, nil
}
