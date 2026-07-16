package shelltask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/task"
)

type nopController struct{}

func (nopController) UpdateProgress(context.Context, string) error         { return nil }
func (nopController) UpdatePayload(context.Context, json.RawMessage) error { return nil }
func (nopController) IsCancelled(context.Context) bool                     { return false }

func rawPayload(t *testing.T, p Payload) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func run(t *testing.T, p Payload) (*task.TaskResult, error) {
	t.Helper()
	handler := NewHandler()
	return handler.Run(context.Background(), &task.Task{ID: "t1", Type: TaskType, Payload: rawPayload(t, p)}, nopController{})
}

func TestRun_SuccessCompletesWithOutputSummary(t *testing.T) {
	result, err := run(t, Payload{Command: "echo hello board", WorkspacePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != task.OutcomeComplete || !strings.Contains(string(result.Result), "hello board") {
		t.Fatalf("result = %+v", result)
	}
}

func TestRun_NonZeroExitFails(t *testing.T) {
	_, err := run(t, Payload{Command: "echo boom >&2; exit 3", WorkspacePath: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "exit code 3") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
	if task.IsRetryable(err) {
		t.Fatal("deterministic exit failure must not be retryable")
	}
}

func TestRun_ResultFileDrivesOutcome(t *testing.T) {
	workspace := t.TempDir()
	cmd := `mkdir -p .tb && printf '{"state":"needs_input","summary":"pick a region","question":"eu or us?","artifacts":["out/report.md","/etc/passwd"]}' > .tb/result.json`
	result, err := run(t, Payload{Command: cmd, WorkspacePath: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != task.OutcomeNeedsInput {
		t.Fatalf("result = %+v", result)
	}
	var parsed Result
	if err := json.Unmarshal(result.Result, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Question != "eu or us?" || len(parsed.Artifacts) != 1 || parsed.Artifacts[0] != "out/report.md" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestRun_StaleResultFileIsIgnored(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".tb"), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := `{"state":"needs_input","summary":"stale"}`
	if err := os.WriteFile(filepath.Join(workspace, resultFileRelPath), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := run(t, Payload{Command: "true", WorkspacePath: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != task.OutcomeComplete || strings.Contains(string(result.Result), "stale") {
		t.Fatalf("stale result leaked: %+v", result)
	}
}

func TestRun_Timeout(t *testing.T) {
	_, err := run(t, Payload{Command: "sleep 5", WorkspacePath: t.TempDir(), TimeoutSeconds: 1})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidate_RejectsMissingWorkspace(t *testing.T) {
	_, err := run(t, Payload{Command: "true", WorkspacePath: "/nonexistent/tb-shell-test"})
	if err == nil || !strings.Contains(err.Error(), "workspace_path") {
		t.Fatalf("err = %v", err)
	}
}
