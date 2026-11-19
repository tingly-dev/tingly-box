package main

import (
	"testing"

	"github.com/tingly-dev/tingly-box/vmodel/spec"
)

// `replay --upstream vmodel` names its vmodels by ID; they must stay defined in
// the canonical default definitions, or the replay would only fail at run time.
func TestReplayDefaultVModelsAreDefined(t *testing.T) {
	defined := make(map[string]bool)
	for _, def := range spec.DefaultDefinitions() {
		defined[def.ID] = true
	}
	for name, sc := range replayScenarios {
		if !defined[sc.defaultVModel] {
			t.Errorf("replay scenario %q uses vmodel %q, which is not in spec.DefaultDefinitions()", name, sc.defaultVModel)
		}
	}
}
