package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
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

func TestMatrixConfigFlag(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"matrix", "--mode", "single", "--config", "model.yaml"}); err != nil {
		t.Fatal(err)
	}
	if len(cli.Matrix.Definitions) != 1 || cli.Matrix.Definitions[0] != "model.yaml" {
		t.Fatalf("lost --config: %v", cli.Matrix.Definitions)
	}
}

// The old `matrix --fixture` spelling is gone: definitions are loaded with
// --config only, like `tb vmodel --config`.
func TestMatrixFixtureFlagRemoved(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"matrix", "--mode", "single", "--fixture", "model.yaml"}); err == nil {
		t.Fatal("--fixture must no longer be accepted")
	}
}

func TestScenariosFromDefinitions(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	two := write("two.yaml", `- id: first
  kind: mock
  mock: {content: One.}
  expect: {http_status: 200, content_equals: One.}
- id: second
  kind: mock
  mock: {content: Two.}
  expect: {http_status: 200, content_equals: Two.}
`)
	noExpect := write("no-expect.yaml", "id: bare\nkind: mock\nmock: {content: x}\n")

	t.Run("all", func(t *testing.T) {
		got, err := scenariosFromDefinitions([]string{two}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Name != "first" || got[1].Name != "second" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("only", func(t *testing.T) {
		got, err := scenariosFromDefinitions([]string{two}, []string{"second"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Name != "second" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("unknown only", func(t *testing.T) {
		_, err := scenariosFromDefinitions([]string{two}, []string{"first", "nope"})
		if err == nil || !strings.Contains(err.Error(), `scenario "nope" not found`) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("no expect", func(t *testing.T) {
		_, err := scenariosFromDefinitions([]string{noExpect}, nil)
		if err == nil || !strings.Contains(err.Error(), "at least one expect") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("missing file", func(t *testing.T) {
		if _, err := scenariosFromDefinitions([]string{filepath.Join(dir, "absent.yaml")}, nil); err == nil {
			t.Fatal("expected an error")
		}
	})
}
