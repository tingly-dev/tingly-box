package protocoltest

import "testing"

// TestFlagPaths is the go-test entry point of the rule-flag × path suite
// (flag_paths.go): every case runs as a parallel subtest against its own env.
func TestFlagPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("combination suite: run without -short or via `harness matrix --mode=flag_paths`")
	}
	for _, c := range flagPathsCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := judgeFlagPathsResult(runRecorderCase(c))
			if r.Failed() {
				for _, e := range r.Errors {
					t.Errorf("%s: %s", e.Assertion, e.Error)
				}
			}
		})
	}
}
