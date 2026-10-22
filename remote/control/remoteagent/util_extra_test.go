package remoteagent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPathFrom_Absolute(t *testing.T) {
	got, err := ExpandPathFrom("/tmp/foo", "/home/me/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/foo" {
		t.Fatalf("absolute path mutated: %q", got)
	}
}

func TestExpandPathFrom_RelativeUsesBaseDir(t *testing.T) {
	got, err := ExpandPathFrom("src/api", "/home/me/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Clean("/home/me/proj/src/api")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExpandPathFrom_RelativeDotDotEscape(t *testing.T) {
	got, err := ExpandPathFrom("../sibling", "/home/me/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Clean("/home/me/sibling")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExpandPathFrom_RelativeWithoutBaseFallsBackToCwd(t *testing.T) {
	got, err := ExpandPathFrom(".", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute fallback, got %q", got)
	}
}

func TestExpandPathFrom_HomeAlias(t *testing.T) {
	got, err := ExpandPathFrom("~/x", "/whatever")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "/x") || strings.HasPrefix(got, "/whatever/") {
		t.Fatalf("home alias not honored: %q", got)
	}
}

func TestParsePositiveInt(t *testing.T) {
	tests := []struct {
		in     string
		want   int
		wantOk bool
	}{
		{"1", 1, true},
		{"42", 42, true},
		{"  7  ", 7, true},
		{"0", 0, false},
		{"", 0, false},
		{"-3", 0, false},
		{"3a", 0, false},
		{"abc", 0, false},
	}
	for _, tt := range tests {
		got, ok := parsePositiveInt(tt.in)
		if got != tt.want || ok != tt.wantOk {
			t.Errorf("parsePositiveInt(%q) = (%d,%v), want (%d,%v)", tt.in, got, ok, tt.want, tt.wantOk)
		}
	}
}
