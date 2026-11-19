package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runLB(t *testing.T, c LbCmd) error {
	t.Helper()
	// The renderers write the trace to stdout; keep it out of the test log.
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	old := os.Stdout
	os.Stdout = devnull
	defer func() { os.Stdout = old }()
	return c.Run()
}

func TestLbBuiltinExamplesSelfCheck(t *testing.T) {
	for _, name := range []string{"single", "cascade", "multiaffinity"} {
		t.Run(name, func(t *testing.T) {
			if err := runLB(t, LbCmd{Example: name}); err != nil {
				t.Fatalf("example %s: %v", name, err)
			}
		})
	}
}

func TestLbExpectMismatchFails(t *testing.T) {
	const scenario = `rule_uuid: t
tactic: tier
services:
  - { provider: deepseek, model: deepseek-pro, tier: 0 }
faults:
  deepseek/deepseek-pro: [200]
expect:
%s
program:
  - { request: "" }
`
	for _, tc := range []struct{ name, expect, want string }{
		{"final_status", "  final_status: 500", "expect: final_status"},
		{"attempts", "  attempts: [other/model]", "expect: attempts"},
		{"breaker", "  breaker: { deepseek/deepseek-pro: open }", "expect: breaker"},
		{"ok", "  final_status: 200\n  attempts: [deepseek/deepseek-pro]", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.yaml")
			body := strings.Replace(scenario, "%s", tc.expect, 1)
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			err := runLB(t, LbCmd{File: path})
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}
