package protocoltest

import (
	"strings"
	"testing"
)

// KnownGap documents a case whose correct behavior is asserted by a test but
// which current code does not yet deliver. Registering it keeps the suite
// green while making the gap visible (the case is reported, not hidden), and
// a fix is forced to delete the entry: a registered case that starts passing
// fails the run.
//
// IDs match .design/protocol-stage-v2.md.
type KnownGap struct {
	ID     string
	Reason string
}

// knownGaps is keyed by test-case name (t.Name() below the top-level test).
var knownGaps = map[string]KnownGap{}

func registerKnownGaps(gap KnownGap, cases ...string) bool {
	for _, c := range cases {
		knownGaps[c] = gap
	}
	return true
}

// checkCase reports one case, honoring the known-gap registry. failures is
// the list of violated expectations (empty = pass); detail (e.g. the client
// response) is printed only for unexpected failures.
func checkCase(t *testing.T, key string, failures []string, detail string) {
	t.Helper()
	gap, known := knownGaps[key]
	switch {
	case known && len(failures) == 0:
		t.Fatalf("known gap %s is fixed for %s: remove it from knownGaps", gap.ID, key)
	case known:
		t.Logf("known gap %s: %s", gap.ID, strings.Join(failures, "; "))
	case len(failures) > 0:
		t.Errorf("%s\n%s", strings.Join(failures, "\n"), detail)
	}
}
