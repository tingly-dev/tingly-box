package main

import (
	"github.com/alecthomas/kong"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatrixDefinitionValidation(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "definition.yaml")
	noExpect := filepath.Join(dir, "no-expect.json")
	if err := os.WriteFile(valid, []byte("id: imported\nkind: recording\nresponses:\n  openai_chat:\n    body: {}\nexpect:\n  http_status: 200\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noExpect, []byte(`{"id":"no-expect","kind":"recording","responses":{"openai_chat":{"body":{}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		cmd  MatrixCmd
		want string
	}{
		{"mode", MatrixCmd{Mode: "default", Definitions: []string{valid}}, "requires --mode=single"},
		{"duplicate", MatrixCmd{Mode: "single", Definitions: []string{valid, valid}}, "duplicate definition id"},
		{"assertions", MatrixCmd{Mode: "single", Definitions: []string{noExpect}}, "at least one expect"},
		{"filter", MatrixCmd{Mode: "single", Definitions: []string{valid}, Scenarios: []string{"missing"}}, "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMatrixConfigAndFixtureAlias(t *testing.T) {
	for _, flag := range []string{"--config", "--fixture"} {
		var cli CLI
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse([]string{"matrix", "--mode", "single", flag, "model.yaml"}); err != nil {
			t.Fatal(err)
		}
		if len(cli.Matrix.Definitions) != 1 || cli.Matrix.Definitions[0] != "model.yaml" {
			t.Fatalf("lost %s alias", flag)
		}
	}
}
