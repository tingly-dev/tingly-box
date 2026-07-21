package shelltask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunSpec is one shell command execution request.
type RunSpec struct {
	Command        string
	WorkspacePath  string
	TimeoutSeconds int
}

// EventSink receives run events (nil to drop).
type EventSink func(kind, summary string, data json.RawMessage)

// RunOnce executes one shell command in the workspace and returns its
// structured outcome. A non-zero exit is NOT an error here — it is reflected
// in Result.ExitCode and Result.State ("failed") so a pipeline's when/until
// can react to it. Only infrastructure failures (spawn, timeout, ctx cancel)
// return an error. Callers that treat non-zero exit as fatal (the standalone
// shell task) inspect the result themselves.
//
// On success the command may leave a structured outcome in .tb/result.json;
// a stale file from a prior run is removed first so it cannot masquerade.
func RunOnce(ctx context.Context, spec RunSpec, sink EventSink) (*Result, error) {
	if spec.TimeoutSeconds <= 0 {
		spec.TimeoutSeconds = defaultTimeoutSeconds
	}
	emit := func(kind, summary string, data json.RawMessage) {
		if sink != nil {
			sink(kind, summary, data)
		}
	}

	resultPath := filepath.Join(spec.WorkspacePath, resultFileRelPath)
	_ = os.Remove(resultPath)

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(spec.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "sh", "-c", spec.Command)
	cmd.Dir = spec.WorkspacePath
	cmd.WaitDelay = 2 * time.Second
	emit("run_started", fmt.Sprintf("Started shell run: %s", clipRunes(spec.Command, 200)), nil)

	started := time.Now()
	output, runErr := cmd.CombinedOutput()
	duration := time.Since(started)
	tail := clipTailRunes(string(output), maxOutputTailRunes)
	if tail != "" {
		emit("output", tail, nil)
	}

	if runCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("shell: timed out after %ds", spec.TimeoutSeconds)
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
			return nil, fmt.Errorf("shell: start command: %w", runErr)
		}
	}

	var result *Result
	if exitCode != 0 {
		// Non-zero exit is the deterministic failure signal; the exit code is
		// the truth, so a leftover result file does not override it.
		result = &Result{State: "failed", Summary: clipTailRunes(string(output), 2000), ExitReason: "nonzero_exit"}
	} else {
		result = outcomeFromResultFile(resultPath)
		if result == nil {
			result = &Result{State: "done", Summary: tail}
		}
	}
	result.ExitCode = &exitCode
	result.DurationMS = duration.Milliseconds()
	if result.ExitReason == "" {
		result.ExitReason = result.State
	}
	data, _ := json.Marshal(result)
	emit("outcome", result.Summary, data)
	return result, nil
}
