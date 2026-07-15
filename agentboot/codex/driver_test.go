package codex

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot"
)

func TestDriverPrepare_NewThread(t *testing.T) {
	workspace := t.TempDir()
	driver := NewDriver(DefaultConfig())
	spec, err := driver.Prepare(context.Background(), "do work", agentboot.ExecutionOptions{
		ProjectPath:        workspace,
		OutputFormat:       agentboot.OutputFormatStreamJSON,
		Model:              "gpt-test",
		AppendSystemPrompt: "return an outcome",
		Env:                []string{"TASK_TEST=value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := []string{
		"codex", "exec",
		"-c", `approval_policy="never"`,
		"-c", `sandbox_mode="workspace-write"`,
		"--json", "--skip-git-repo-check", "--model", "gpt-test", "--cd", workspace,
	}
	if len(spec.Command) < len(wantPrefix) || !slices.Equal(spec.Command[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("command prefix\nwant: %q\n got: %q", wantPrefix, spec.Command)
	}
	if got := spec.Command[len(spec.Command)-1]; !strings.Contains(got, "do work") || !strings.Contains(got, "return an outcome") {
		t.Fatalf("composed prompt = %q", got)
	}
	if spec.WorkDir != workspace || !slices.Contains(spec.Env, "TASK_TEST=value") {
		t.Fatalf("launch spec = %+v", spec)
	}
	if slices.Contains(spec.Command, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatal("unsafe bypass flag must never be present")
	}
}

func TestDriverPrepare_Resume(t *testing.T) {
	workspace := t.TempDir()
	driver := NewDriver(DefaultConfig())
	spec, err := driver.Prepare(context.Background(), "continue", agentboot.ExecutionOptions{
		ProjectPath:  workspace,
		OutputFormat: agentboot.OutputFormatStreamJSON,
		SessionID:    "thread-1",
		Resume:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(spec.Command[:3], []string{"codex", "exec", "resume"}) {
		t.Fatalf("resume command = %q", spec.Command)
	}
	if slices.Contains(spec.Command, "--cd") {
		t.Fatal("codex exec resume does not accept --cd")
	}
	if got := spec.Command[len(spec.Command)-2:]; !slices.Equal(got, []string{"thread-1", "continue"}) {
		t.Fatalf("resume tail = %q", got)
	}
}

func TestDriverPrepare_RejectsSelectedNewThreadID(t *testing.T) {
	driver := NewDriver(DefaultConfig())
	_, err := driver.Prepare(context.Background(), "work", agentboot.ExecutionOptions{
		ProjectPath:  t.TempDir(),
		OutputFormat: agentboot.OutputFormatStreamJSON,
		SessionID:    "cannot-select",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
