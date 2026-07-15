package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

type dispatchedErrorServer struct {
	err error
}

func (s dispatchedErrorServer) CallMCPToolWithHooks(context.Context, string, string, []map[string]any) (context.Context, coretool.ToolResult, error) {
	return context.Background(), coretool.ToolResult{}, &servertool.DispatchError{Err: s.err}
}

func TestServerToolExecutorPreservesDispatchBoundary(t *testing.T) {
	runtimeErr := errors.New("runtime failed")
	executor := NewServerToolExecutor(dispatchedErrorServer{err: runtimeErr})
	_, result, err := executor.ExecuteToolWithContext(context.Background(), testTool{id: "toolu-1", name: "lookup"}, nil)
	if !errors.Is(err, runtimeErr) || !result.Dispatched || !result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

type testTool struct {
	id   string
	name string
}

func (t testTool) ID() string      { return t.id }
func (t testTool) Name() string    { return t.name }
func (testTool) Arguments() string { return "{}" }
