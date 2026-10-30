package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Every shipped script must pass its own step-by-step check through the
// in-process gateway, on both agent wire formats (Anthropic messages for
// claude, OpenAI Responses for codex).
func TestScriptTestdataPasses(t *testing.T) {
	files, err := filepath.Glob("testdata/scripts/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no scripts found: %v", err)
	}
	for _, f := range files {
		for _, agent := range []string{"claude", "codex"} {
			t.Run(filepath.Base(f)+"/"+agent, func(t *testing.T) {
				if err := (&ScriptCmd{Agent: agent, Files: []string{f}}).Run(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestScriptRejectsInvalidScript(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("steps:\n  - sya: oops\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (&ScriptCmd{Agent: "claude", Files: []string{bad}}).Run(); err == nil {
		t.Fatal("expected a parse error for an unknown step field")
	}
}
