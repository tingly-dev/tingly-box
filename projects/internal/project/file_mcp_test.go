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

func TestFileMCPPublishesToItsThread(t *testing.T) {
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
	server := &fileMCP{service: svc}
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
	require.Len(t, tools.Tools, 1)
	require.Equal(t, "send_file", tools.Tools[0].Name)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "send_file", Arguments: map[string]any{"path": "result.txt", "caption": "Here"}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	events, err := store.ListThreadEvents(thread.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, db.EvFile, events[0].Kind)
	require.Equal(t, "Here", events[0].Content)
	// The same capability remains usable for later turns.
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "send_file", Arguments: map[string]any{"path": "result.txt"}})
	require.NoError(t, err)
}
