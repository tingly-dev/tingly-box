package project

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestCoordinatorMCPValidatesAndScopesTools(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	p := mkProject(t, store, &db.Project{Name: "One"})
	other := mkProject(t, store, &db.Project{Name: "Other"})
	coord := &Coordinator{store: store, layout: Layout{Root: t.TempDir()}, hub: NewHub()}
	server, err := coord.coordinatorMCP(p, true)
	require.NoError(t, err)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	listed, err := cs.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, listed.Tools, len(coordinatorTools())+2)
	call := func(name string, args any) *mcp.CallToolResult {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		return result
	}
	require.True(t, call("start_thread", map[string]any{"task": "missing title"}).IsError)
	require.True(t, call("no_update", map[string]any{}).IsError)
	require.True(t, call("get_thread", map[string]any{"thread_id": other.MainThreadID}).IsError)
	require.False(t, call("list_threads", map[string]any{}).IsError)
	changed, err := store.GetProject(p.ID)
	require.NoError(t, err)
	changed.Goal = "changed"
	require.NoError(t, store.SaveProject(changed))
	require.True(t, call("list_threads", map[string]any{}).IsError)
}
