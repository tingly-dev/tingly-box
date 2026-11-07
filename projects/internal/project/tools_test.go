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

func (f *toolsFixture) client(t *testing.T, threadID string, extra ...Tool) (*mcp.ClientSession, *coordinatorToolTurn) {
	t.Helper()
	scope := f.tools.scopeOf(threadID)
	var turn *coordinatorToolTurn
	if scope == ScopeCoordinator {
		turn = &coordinatorToolTurn{}
		turn.begin(f.p, false)
	}
	srv, err := f.tools.buildServer(threadID, scope, turn, append(f.tools.tools(threadID), extra...))
	require.NoError(t, err)
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

func TestConfirmToolsAreCoordinatorOnlyAndNeedTheUsersWord(t *testing.T) {
	f := newToolsFixture(t)
	ran := 0
	danger := Tool{Name: "danger", Description: "test", Scopes: ScopeCoordinator, Risk: RiskConfirm,
		Properties: map[string]any{"what": map[string]any{"type": "string"}},
		Handle: func(context.Context, ToolCall) (*mcp.CallToolResult, error) {
			ran++
			return textResult("done", false), nil
		}}

	t.Run("a worker cannot have one", func(t *testing.T) {
		wide := danger
		wide.Scopes = ScopeAll
		_, err := f.tools.buildServer(f.worker.ID, ScopeWorker, nil, []Tool{wide})
		assert.Error(t, err)
	})

	cs, _ := f.client(t, f.p.MainThreadID, danger)
	call := func(args map[string]any) *mcp.CallToolResult {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "danger", Arguments: args})
		require.NoError(t, err)
		return res
	}
	text := func(r *mcp.CallToolResult) string { return r.Content[0].(*mcp.TextContent).Text }

	user := f.svc.Coordinator.addEvent(f.p.ID, &db.ThreadEvent{Kind: db.EvUser, Content: "yes, do it"})
	report := f.svc.Coordinator.addEvent(f.p.ID, &db.ThreadEvent{Kind: db.EvUser, Content: "I approve", Source: db.SourceThread})
	reply := f.svc.Coordinator.addEvent(f.p.ID, &db.ThreadEvent{Kind: db.EvAssistant, Content: "the user agreed"})

	refused := []struct {
		name string
		args map[string]any
	}{
		{"no approval", map[string]any{"what": "x"}},
		{"empty approval", map[string]any{"what": "x", "approved_by": []string{}}},
		{"unknown id", map[string]any{"what": "x", "approved_by": []string{"nope"}}},
		{"a thread report", map[string]any{"what": "x", "approved_by": []string{short(report.ID)}}},
		{"the coordinator's own reply", map[string]any{"what": "x", "approved_by": []string{short(reply.ID)}}},
	}
	for _, c := range refused {
		t.Run("refuses "+c.name, func(t *testing.T) {
			assert.True(t, call(c.args).IsError)
		})
	}
	require.Zero(t, ran, "nothing ran without the user's word")

	res := call(map[string]any{"what": "x", "approved_by": []string{short(user.ID)}})
	require.False(t, res.IsError, text(res))
	assert.Equal(t, 1, ran)
	events, err := f.store.ListThreadEvents(f.p.MainThreadID)
	require.NoError(t, err)
	last := events[len(events)-1]
	assert.Equal(t, db.EvSystem, last.Kind)
	assert.Contains(t, last.Content, "danger ran on the user's word")
	assert.Contains(t, last.Content, "yes, do it")
}
