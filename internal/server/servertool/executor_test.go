package servertool

import (
	"context"
	"errors"
	"testing"

	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

type errorRuntime struct {
	err   error
	calls int
}

func (r *errorRuntime) CallTool(context.Context, string, string) (coretool.ToolResult, error) {
	r.calls++
	return coretool.ToolResult{}, r.err
}

func (*errorRuntime) ListCallableServerToolNames(context.Context) map[string]struct{} {
	return map[string]struct{}{"tingly_box_mcp__test__run": {}}
}

func (*errorRuntime) GetAdvisorMaxUses() int { return 0 }

func TestDefaultExecutorMarksOnlyPostDispatchErrors(t *testing.T) {
	runtimeErr := errors.New("runtime failed")
	runtime := &errorRuntime{err: runtimeErr}
	executor := NewDefaultExecutor(runtime, nil)

	_, _, err := executor.Execute(context.Background(), ToolCall{NormalizedName: "tingly_box_mcp__test__run"})
	if !errors.Is(err, runtimeErr) || !WasDispatched(err) || runtime.calls != 1 {
		t.Fatalf("runtime error = %v, dispatched=%v, calls=%d", err, WasDispatched(err), runtime.calls)
	}

	_, _, err = executor.Execute(context.Background(), ToolCall{NormalizedName: "not-an-mcp-tool"})
	if err == nil || WasDispatched(err) || runtime.calls != 1 {
		t.Fatalf("validation error = %v, dispatched=%v, calls=%d", err, WasDispatched(err), runtime.calls)
	}
}
