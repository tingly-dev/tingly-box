package github

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot"
)

func TestPrepare_NilLauncherStillHardens(t *testing.T) {
	opts := agentboot.ExecutionOptions{
		Env:                []string{"A=1"},
		DisallowedTools:    []string{"Bash(gh auth:*)", "Edit"},
		AppendSystemPrompt: "existing",
	}
	Prepare(context.Background(), nil, "/x", "s1", &opts)

	for _, want := range []string{"A=1", "GH_PROMPT_DISABLED=1", "GIT_TERMINAL_PROMPT=0"} {
		if !slices.Contains(opts.Env, want) {
			t.Errorf("env missing %q: %v", want, opts.Env)
		}
	}
	if n := countOf(opts.DisallowedTools, "Bash(gh auth:*)"); n != 1 {
		t.Errorf("deny rule duplicated %d times", n)
	}
	for _, want := range []string{"Edit", "Bash(git push:*)", "Bash(gh pr merge:*)"} {
		if !slices.Contains(opts.DisallowedTools, want) {
			t.Errorf("deny list missing %q: %v", want, opts.DisallowedTools)
		}
	}
	if !strings.HasPrefix(opts.AppendSystemPrompt, "existing\n\n") || !strings.Contains(opts.AppendSystemPrompt, "git push") {
		t.Errorf("prompt = %q", opts.AppendSystemPrompt)
	}
}

func TestEmptyGHConfigDir(t *testing.T) {
	dir, err := EmptyGHConfigDir(t.TempDir())
	if err != nil || dir == "" {
		t.Fatalf("dir=%q err=%v", dir, err)
	}
}

func countOf(s []string, v string) (n int) {
	for _, x := range s {
		if x == v {
			n++
		}
	}
	return
}
