// Package shelltask executes one bounded shell command as a task run.
// It is the non-agent executor of the task board (design:
// .design/task-board.md §5): exit code maps to the run status, bounded
// stdout/stderr tails become run events, and an optional
// .tb/result.json in the workspace supplies a structured outcome.
package shelltask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/task"
)

const (
	// TaskType is the handler registration key.
	TaskType = "shell"

	defaultTimeoutSeconds = 30 * 60
	maxOutputTailRunes    = 8000
	resultFileRelPath     = ".tb/result.json"
)

// Payload is the persisted definition of one shell task.
type Payload struct {
	Command        string `json:"command"`
	WorkspacePath  string `json:"workspace_path"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

func (p *Payload) ApplyDefaults() {
	if p.TimeoutSeconds <= 0 {
		p.TimeoutSeconds = defaultTimeoutSeconds
	}
}

func (p Payload) Validate() error {
	if strings.TrimSpace(p.Command) == "" {
		return errors.New("command is required")
	}
	if strings.TrimSpace(p.WorkspacePath) == "" {
		return errors.New("workspace_path is required")
	}
	if !filepath.IsAbs(p.WorkspacePath) {
		return fmt.Errorf("workspace_path %q must be absolute", p.WorkspacePath)
	}
	info, err := os.Stat(p.WorkspacePath)
	if err != nil {
		return fmt.Errorf("workspace_path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace_path %q is not a directory", p.WorkspacePath)
	}
	return nil
}

// Result is the structured outcome of one shell run. A command may write it
// to .tb/result.json; otherwise it is synthesized from the exit status.
type Result struct {
	State      string   `json:"state"`
	Summary    string   `json:"summary"`
	Artifacts  []string `json:"artifacts,omitempty"`
	Question   string   `json:"question,omitempty"`
	ExitCode   *int     `json:"exit_code,omitempty"`
	DurationMS int64    `json:"duration_ms"`
	ExitReason string   `json:"exit_reason,omitempty"`
}

// Handler runs shell task payloads.
type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) Type() string { return TaskType }

func (h *Handler) Run(ctx context.Context, t *task.Task, ctl task.Controller) (*task.TaskResult, error) {
	var payload Payload
	if err := json.Unmarshal(t.Payload, &payload); err != nil {
		return nil, fmt.Errorf("shell task: decode payload: %w", err)
	}
	payload.ApplyDefaults()
	if err := payload.Validate(); err != nil {
		return nil, fmt.Errorf("shell task: %w", err)
	}
	runCtl, _ := ctl.(task.RunController)

	// A stale result file must never masquerade as this run's outcome.
	resultPath := filepath.Join(payload.WorkspacePath, resultFileRelPath)
	_ = os.Remove(resultPath)

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(payload.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "sh", "-c", payload.Command)
	cmd.Dir = payload.WorkspacePath
	// Don't block on orphaned grandchildren holding the output pipes after
	// the shell itself is killed at the deadline.
	cmd.WaitDelay = 2 * time.Second
	appendEvent(ctx, runCtl, "run_started", fmt.Sprintf("Started shell run: %s", clipRunes(payload.Command, 200)), nil)
	_ = ctl.UpdateProgress(ctx, "Shell command running")

	started := time.Now()
	output, runErr := cmd.CombinedOutput()
	duration := time.Since(started)
	tail := clipTailRunes(string(output), maxOutputTailRunes)
	if tail != "" {
		appendEvent(ctx, runCtl, "output", tail, nil)
	}

	if runCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("shell task: timed out after %ds", payload.TimeoutSeconds)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("shell task: start command: %w", runErr)
		}
	}
	if exitCode != 0 {
		// Non-zero exit is a deterministic failure; a plain (non-retryable)
		// error fails the task without pointless backoff retries.
		return nil, fmt.Errorf("shell task: exit code %d: %s", exitCode, clipTailRunes(string(output), 500))
	}

	result := outcomeFromResultFile(resultPath)
	if result == nil {
		result = &Result{State: "done", Summary: tail}
	}
	result.ExitCode = &exitCode
	result.DurationMS = duration.Milliseconds()
	if result.ExitReason == "" {
		result.ExitReason = result.State
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	appendEvent(ctx, runCtl, "outcome", result.Summary, data)
	if result.State == "needs_input" {
		return &task.TaskResult{Outcome: task.OutcomeNeedsInput, Result: data}, nil
	}
	return &task.TaskResult{Outcome: task.OutcomeComplete, Result: data}, nil
}

// outcomeFromResultFile reads the optional structured outcome the command
// left behind. Invalid or unknown content is ignored (the exit code already
// proved success); only done/needs_input are meaningful states here.
func outcomeFromResultFile(path string) *Result {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var result Result
	if json.Unmarshal(data, &result) != nil {
		return nil
	}
	result.State = strings.TrimSpace(result.State)
	if result.State != "done" && result.State != "needs_input" {
		return nil
	}
	result.Summary = clipRunes(result.Summary, maxOutputTailRunes)
	result.Question = clipRunes(result.Question, maxOutputTailRunes)
	safe := result.Artifacts[:0]
	for _, artifact := range result.Artifacts {
		artifact = strings.TrimSpace(artifact)
		if artifact == "" || filepath.IsAbs(artifact) {
			continue
		}
		clean := filepath.Clean(artifact)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			continue
		}
		safe = append(safe, clean)
	}
	result.Artifacts = safe
	return &result
}

func appendEvent(ctx context.Context, ctl task.RunController, kind, summary string, data json.RawMessage) {
	if ctl == nil {
		return
	}
	_ = ctl.AppendRunEvent(ctx, task.RunEvent{
		ID: uuid.NewString(), Kind: kind, Summary: summary, Data: data, CreatedAt: time.Now(),
	})
}

func clipRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func clipTailRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return "…" + string(runes[len(runes)-limit:])
}
