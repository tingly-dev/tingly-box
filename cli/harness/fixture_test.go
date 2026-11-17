package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatrixFixtureValidation(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "fixture.yaml")
	noExpect := filepath.Join(dir, "no-expect.json")
	if err := os.WriteFile(valid, []byte("name: imported\nmock_responses:\n  openai_chat:\n    body: {}\nexpect:\n  http_status: 200\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noExpect, []byte(`{"name":"no-expect","mock_responses":{"openai_chat":{"body":{}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		cmd  MatrixCmd
		want string
	}{
		{"mode", MatrixCmd{Mode: "default", Fixtures: []string{valid}}, "requires --mode=single"},
		{"duplicate", MatrixCmd{Mode: "single", Fixtures: []string{valid, valid}}, "duplicate fixture name"},
		{"assertions", MatrixCmd{Mode: "single", Fixtures: []string{noExpect}}, "at least one expect"},
		{"filter", MatrixCmd{Mode: "single", Fixtures: []string{valid}, Scenarios: []string{"missing"}}, "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}
