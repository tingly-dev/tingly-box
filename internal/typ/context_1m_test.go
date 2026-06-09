package typ

import (
	"context"
	"testing"
)

func TestStripContext1MSuffix(t *testing.T) {
	cases := []struct {
		in       string
		wantBase string
		wantHad  bool
	}{
		{"tingly/cc-sonnet[1m]", "tingly/cc-sonnet", true},
		{"tingly/cc-sonnet", "tingly/cc-sonnet", false},
		{"[1m]", "", true},
		{"", "", false},
		{"claude-opus-4-6[1m]", "claude-opus-4-6", true},
		// suffix only counts at the very end
		{"a[1m]b", "a[1m]b", false},
	}
	for _, c := range cases {
		base, had := StripContext1MSuffix(c.in)
		if base != c.wantBase || had != c.wantHad {
			t.Errorf("StripContext1MSuffix(%q) = (%q,%v), want (%q,%v)", c.in, base, had, c.wantBase, c.wantHad)
		}
	}
}

func TestWithGetContext1M(t *testing.T) {
	base := context.Background()

	if GetContext1M(base) {
		t.Error("empty context should report no 1M intent")
	}
	if GetContext1M(nil) {
		t.Error("nil context should report no 1M intent")
	}

	// want=false must leave the context untouched (no key attached).
	if ctx := WithContext1M(base, false); GetContext1M(ctx) {
		t.Error("WithContext1M(false) should not attach the intent")
	}

	if ctx := WithContext1M(base, true); !GetContext1M(ctx) {
		t.Error("WithContext1M(true) should attach the intent")
	}
}
