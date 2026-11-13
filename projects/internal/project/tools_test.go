package project

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// toolsFixture is a project with a worker and a folder session, and the tool
// registry of a service that has a coordinator.
type toolsFixture struct {
	store  *db.Store
	svc    *Service
	tools  *sessionToolsMCP
	p      *db.Project
	worker *db.Thread
	folder *db.Thread
}

func newToolsFixture(t *testing.T) *toolsFixture {
	t.Helper()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	p := mkProject(t, store, &db.Project{Name: "One"})
	worker := &db.Thread{ProjectID: p.ID, ParentID: p.MainThreadID, Agent: db.AgentClaudeCode, Title: "W"}
	require.NoError(t, store.CreateThread(worker))
	folderProject, err := store.OpenFolder(realTempDir(t), "")
	require.NoError(t, err)
	folder := &db.Thread{ProjectID: folderProject.ID, Agent: db.AgentClaudeCode, Title: "F"}
	require.NoError(t, store.CreateThread(folder))
	svc := &Service{Store: store, Layout: Layout{Root: t.TempDir()}, Hub: NewHub()}
	svc.Coordinator = &Coordinator{store: store, layout: svc.Layout, hub: svc.Hub}
	return &toolsFixture{store: store, svc: svc, tools: &sessionToolsMCP{service: svc}, p: p, worker: worker, folder: folder}
}

func (f *toolsFixture) client(t *testing.T, threadID string) (*mcp.ClientSession, *coordinatorToolTurn) {
	t.Helper()
	return connectTools(t, f.svc, threadID, false)
}

// connectTools builds the MCP server of thread threadID's session the way
// config does, begins the coordinator's turn if it has one, and connects a
// client to it.
func connectTools(t *testing.T, svc *Service, threadID string, userTriggered bool) (*mcp.ClientSession, *coordinatorToolTurn) {
	t.Helper()
	srv, turn, err := (&sessionToolsMCP{service: svc}).buildFor(threadID)
	require.NoError(t, err)
	if turn != nil {
		th, err := svc.Store.GetThread(threadID)
		require.NoError(t, err)
		p, err := svc.Store.GetProject(th.ProjectID)
		require.NoError(t, err)
		turn.begin(p, userTriggered)
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs, turn
}

func listTools(t *testing.T, cs *mcp.ClientSession) []*mcp.Tool {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	return res.Tools
}

func toolNames(tools []*mcp.Tool) []string {
	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

// This list is the boundary: a tool added to a scope has to be added here, so
// it cannot become visible to a worker or a folder session by accident.
func TestToolsOfEachScope(t *testing.T) {
	f := newToolsFixture(t)
	coordinator := []string{"forget", "get_thread", "list_shared_files", "list_threads", "message_thread", "no_update", "propose_threads",
		"read_conversation", "read_shared_file", "remember", "rename_thread", "request_decision", "resolve_thread", "send_file",
		"start_thread", "update_instructions"}
	for _, c := range []struct {
		name     string
		threadID string
		want     []string
	}{
		{"coordinator", f.p.MainThreadID, coordinator},
		{"worker", f.worker.ID, []string{"send_file"}},
		{"folder session", f.folder.ID, []string{"send_file"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			cs, _ := f.client(t, c.threadID)
			tools := listTools(t, cs)
			assert.Equal(t, c.want, toolNames(tools))
			for _, tool := range tools {
				props := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
				assert.NotContains(t, props, "project_id", "%s: the project comes from the capability URL", tool.Name)
				if c.name != "coordinator" {
					assert.NotContains(t, props, "thread_id", "%s: a worker's tools act on itself only", tool.Name)
				}
			}
		})
	}
}

// What the coordinator sees on every request. Raise the limit on purpose, not
// by accident: each tool costs tokens on every turn.
func TestCoordinatorToolListStaysSmall(t *testing.T) {
	f := newToolsFixture(t)
	cs, _ := f.client(t, f.p.MainThreadID)
	b, err := json.Marshal(listTools(t, cs))
	require.NoError(t, err)
	t.Logf("coordinator tool list: %d bytes", len(b))
	assert.Less(t, len(b), 12000)
}
