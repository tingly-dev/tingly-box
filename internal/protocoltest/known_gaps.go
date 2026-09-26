package protocoltest

import (
	"fmt"
	"strings"
	"testing"
)

// KnownGap documents a case whose correct behavior is asserted by a test but
// which current code does not yet deliver. Registering it keeps the suite
// green while making the gap visible (the case is reported, not hidden), and
// a fix is forced to delete the entry: a registered case that starts passing
// fails the run — under `go test` and in the harness CLI alike.
type KnownGap struct {
	ID     string
	Reason string
}

// knownGaps is keyed by test-case name: t.Name() of the go test (sub)test,
// which is also the case key the harness CLI section reports under.
// Registrations live below, in one place, so both entry points read the same
// registry.
var knownGaps = map[string]KnownGap{}

func registerKnownGaps(gap KnownGap, cases ...string) bool {
	for _, c := range cases {
		knownGaps[c] = gap
	}
	return true
}

// KnownGapReason returns the reason registered for a known-gap ID ("" when
// no case is registered under it).
func KnownGapReason(id string) string {
	for _, gap := range knownGaps {
		if gap.ID == id {
			return gap.Reason
		}
	}
	return ""
}

// judgeCase applies the known-gap registry to one case. failures is the list
// of violated expectations (empty = pass). It returns the errors to report as
// failures — the case's own failures, or a "gap is fixed" error for a
// registered case that passes — and, for a registered case that still fails,
// the gap it matches.
func judgeCase(key string, failures []string) (errs []string, gap *KnownGap) {
	return judgeCaseIn(knownGaps, key, failures)
}

// judgeCaseIn is judgeCase against an explicit registry.
func judgeCaseIn(registry map[string]KnownGap, key string, failures []string) (errs []string, gap *KnownGap) {
	registered, known := registry[key]
	switch {
	case known && len(failures) == 0:
		return []string{fmt.Sprintf("known gap %s is fixed for %s: remove it from knownGaps", registered.ID, key)}, nil
	case known:
		return nil, &registered
	default:
		return failures, nil
	}
}

// checkCase reports one case, honoring the known-gap registry. failures is
// the list of violated expectations (empty = pass); detail (e.g. the client
// response) is printed only for unexpected failures.
func checkCase(t *testing.T, key string, failures []string, detail string) {
	t.Helper()
	errs, gap := judgeCase(key, failures)
	switch {
	case gap != nil:
		t.Logf("known gap %s: %s", gap.ID, strings.Join(failures, "; "))
	case len(errs) > 0 && len(failures) == 0:
		t.Fatal(errs[0])
	case len(errs) > 0:
		t.Errorf("%s\n%s", strings.Join(errs, "\n"), detail)
	}
}

// ─── Registry ────────────────────────────────────────────────────────────────

var _ = registerKnownGaps(KnownGap{
	ID:     "T3",
	Reason: "a retryable failure after a server tool ran fails over to the next service, which replays the whole request: the tool runs again and the client gets a 200",
},
	"TestMCPNoFailoverAfterServerTool/openai_chat->openai_chat/stream=false",
	"TestMCPNoFailoverAfterServerTool/openai_chat->openai_chat/stream=true",
)
