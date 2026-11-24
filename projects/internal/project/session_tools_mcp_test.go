package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestSessionToolsMCPPublishesToItsThread(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	root := t.TempDir()
	p, err := store.OpenFolder(root, "")
	require.NoError(t, err)
	thread := &db.Thread{ProjectID: p.ID, Cwd: root, Title: "Test"}
	require.NoError(t, store.CreateThread(thread))
	require.NoError(t, os.WriteFile(filepath.Join(root, "result.txt"), []byte("hello"), 0o600))
	svc := &Service{Store: store, Layout: Layout{Root: t.TempDir()}, Hub: NewHub()}
	svc.Coordinator = &Coordinator{store: store, layout: svc.Layout, hub: svc.Hub}
	server := &sessionToolsMCP{service: svc}
	t.Cleanup(func() { _ = server.close(ctx) })
	configPath, err := server.config(thread.ID)
	require.NoError(t, err)
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var config struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &config))
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: config.MCPServers["tingly-projects"].URL}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "send_file")
	require.NotContains(t, names, "start_thread")
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "send_file", Arguments: map[string]any{"path": "result.txt", "caption": "Here"}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	structured, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok, "send_file answers with structured content as well as text")
	require.NotEmpty(t, structured["message_id"])
	events, err := store.ListThreadEvents(thread.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, db.EvFile, events[0].Kind)
	require.Equal(t, "Here", events[0].Content)
	// The same capability remains usable for later turns.
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "send_file", Arguments: map[string]any{"path": "result.txt"}})
	require.NoError(t, err)
}

func TestMainThreadMCPRequiresActiveCoordinatorTurn(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	p := mkProject(t, store, &db.Project{Name: "Project"})
	svc := &Service{Store: store, Layout: Layout{Root: t.TempDir()}, Hub: NewHub()}
	svc.Coordinator = &Coordinator{store: store, layout: svc.Layout, hub: svc.Hub}
	server := &sessionToolsMCP{service: svc}
	t.Cleanup(func() { _ = server.close(ctx) })
	configPath, err := server.config(p.MainThreadID)
	require.NoError(t, err)
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var config struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &config))
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: config.MCPServers["tingly-projects"].URL}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	call := func() *mcp.CallToolResult {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_threads", Arguments: map[string]any{}})
		require.NoError(t, err)
		return result
	}
	require.True(t, call().IsError)
	fileResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "send_file", Arguments: map[string]any{"path": "result.txt"}})
	require.NoError(t, err)
	require.True(t, fileResult.IsError)
	turn := server.coordinatorTurn(p.MainThreadID)
	require.NotNil(t, turn)
	turn.begin(p, true)
	require.False(t, call().IsError)
	turn.end()
	require.True(t, call().IsError)
	turn.begin(p, false)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "no_update", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.True(t, turn.quiet.Load())
	turn.end()
	turn.begin(p, true)
	require.False(t, turn.quiet.Load())
}
