package envsubst

import (
	"testing"
)

func TestExpand(t *testing.T) {
	// lookup that knows FOO and BAR, nothing else.
	lookup := func(name string) (string, bool) {
		switch name {
		case "FOO":
			return "foo-val", true
		case "BAR":
			return "bar-val", true
		case "EMPTY":
			return "", true // set, but empty
		}
		return "", false
	}

	cases := []struct {
		name        string
		in          string
		want        string
		wantMissing []string
	}{
		{"braced", "${FOO}", "foo-val", nil},
		{"bare", "$FOO", "foo-val", nil},
		{"embedded braced", "pre-${FOO}-post", "pre-foo-val-post", nil},
		{"embedded bare", "x=$FOO end", "x=foo-val end", nil},
		{"multiple", "${FOO}@$BAR", "foo-val@bar-val", nil},
		{"adjacent", "${FOO}${BAR}", "foo-valbar-val", nil},
		{"ref plus literal suffix (url style)", "${FOO}/v1/", "foo-val/v1/", nil},

		// unset: leave literal + report missing
		{"unset braced", "${NOPE}", "${NOPE}", []string{"NOPE"}},
		{"unset bare", "$NOPE", "$NOPE", []string{"NOPE"}},
		{"mixed set and unset", "${FOO}-${NOPE}", "foo-val-${NOPE}", []string{"NOPE"}},
		{"two distinct unset", "${A}-${B}", "${A}-${B}", []string{"A", "B"}},
		{"same unset twice deduped", "${A}-${A}", "${A}-${A}", []string{"A"}},

		// set-but-empty is NOT unset: substitutes empty, not in missing
		{"set empty substituted", "[${EMPTY}]", "[]", nil},

		// non-references: '$' emitted literally
		{"no dollar", "plain", "plain", nil},
		{"dollar digit", "cost $5 and $10", "cost $5 and $10", nil},
		{"bare dollar at end", "trailing$", "trailing$", nil},
		{"bare dollar alone", "$", "$", nil},
		{"braced invalid name", "${1abc}", "${1abc}", nil}, // name can't start with digit
		{"braced no close", "${FOO", "${FOO", nil},
		{"underscore name", "$_FOO_BAR", "$_FOO_BAR", []string{"_FOO_BAR"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, gotMissing := Expand(c.in, lookup)
			if got != c.want {
				t.Fatalf("Expand(%q) = %q, want %q", c.in, got, c.want)
			}
			if !eqSlice(gotMissing, c.wantMissing) {
				t.Fatalf("missing: got %v, want %v", gotMissing, c.wantMissing)
			}
		})
	}
}

func TestExpandNoDollarShortCircuits(t *testing.T) {
	called := false
	lookup := func(string) (string, bool) { called = true; return "", false }
	got, missing := Expand("no refs here", lookup)
	if got != "no refs here" {
		t.Fatalf("got %q", got)
	}
	if missing != nil {
		t.Fatalf("want nil missing, got %v", missing)
	}
	if called {
		t.Fatal("lookup should not be called when string has no '$'")
	}
}

func TestExpandOS(t *testing.T) {
	t.Setenv("ENVSUBST_TEST_VAR", "os-val")
	t.Setenv("ENVSUBST_TEST_EMPTY", "")

	got := ExpandOS("${ENVSUBST_TEST_VAR}/$ENVSUBST_TEST_VAR")
	want := "os-val/os-val"
	if got != want {
		t.Fatalf("ExpandOS = %q, want %q", got, want)
	}

	// set-but-empty substitutes empty.
	if got, want := ExpandOS("[${ENVSUBST_TEST_EMPTY}]"), "[]"; got != want {
		t.Fatalf("empty var: got %q want %q", got, want)
	}

	// unset left literal (no panic, no empty substitution).
	if got, want := ExpandOS("${ENVSUBST_TEST_UNSET}"), "${ENVSUBST_TEST_UNSET}"; got != want {
		t.Fatalf("unset: got %q want %q", got, want)
	}
}

func eqSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
