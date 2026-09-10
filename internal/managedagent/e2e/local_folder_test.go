package e2e_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// Journey: pick a folder on this machine and work in it directly.
//
//	browse → start with local_path → agent answers → diff shows the edit →
//	push refused (in place) → folder is "recent" and listed as a local source →
//	a second task on the same folder reuses the workspace → a plain (non-git)
//	folder works too, with an empty diff.
func TestJourney_LocalFolderInPlace(t *testing.T) {
	requireE2E(t)
	s := bootStack(t, nil)
	dir := newGitDir(t, "playground")

	// The picker can see the folder and knows it is a repo.
	var listing managedagent.DirListing
	if code := s.do(http.MethodGet, "/api/v1/agent/fs/dirs?path="+filepath.Dir(dir), nil, &listing); code != 200 {
		t.Fatalf("browse: %d", code)
	}
	var seen bool
	for _, e := range listing.Entries {
		if e.Path == dir {
			seen = true
			if !e.IsRepo {
				t.Fatalf("browse: %s not flagged as repo", dir)
			}
		}
	}
	if !seen {
		t.Fatalf("browse: %s missing from %+v", dir, listing.Entries)
	}
	if code := s.do(http.MethodGet, "/api/v1/agent/fs/dirs?path=relative/path", nil, nil); code != 400 {
		t.Fatalf("browse relative path: want 400, got %d", code)
	}

	d := s.createSession(map[string]any{"local_path": dir, "prompt": "What is the capital of France?"})
	id := d.Session.ID
	if d.Workspace.Path != dir || d.Workspace.AgentCwd != dir || d.Workspace.Branch != "" || d.Workspace.State != managedagent.WorkspaceReady {
		t.Fatalf("in-place workspace expected, got %+v", d.Workspace)
	}

	d, ev := s.waitIdle(id)
	if !hasEvent(ev, managedagent.EventAssistantMessage, protocoltest.VirtualMockAnswerMarker) {
		t.Fatalf("no assistant answer\n%s", eventsDump(ev))
	}
	if s.env.VirtualServer().CallCount() == 0 {
		t.Fatal("the virtual upstream never saw a request; the CLI routed elsewhere")
	}
	// Nothing was cloned or branched.
	if ws, _ := os.ReadDir(constant.GetAgentWorkspacesDir(s.env.ConfigDir())); len(ws) != 0 {
		t.Fatalf("a workspace clone was created for an in-place folder: %v", ws)
	}

	// The agent's (simulated) edit shows up in the diff of the folder itself.
	if err := os.WriteFile(filepath.Join(dir, "NOTES.md"), []byte("by agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var diff managedagent.Diff
	if code := s.do(http.MethodGet, "/api/v1/agent/sessions/"+id+"/diff", nil, &diff); code != 200 || diff.ChangedFiles != 1 {
		t.Fatalf("diff: %d %+v", code, diff)
	}
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/push", nil, nil); code != 409 {
		t.Fatalf("push in place: want 409, got %d", code)
	}

	// The folder is now "recent" and listed as a local source, not a repo.
	var recent struct {
		Folders []managedagent.RecentFolder `json:"folders"`
	}
	if code := s.do(http.MethodGet, "/api/v1/agent/fs/recent", nil, &recent); code != 200 {
		t.Fatalf("recent: %d", code)
	}
	if len(recent.Folders) == 0 || recent.Folders[0].Path != dir || recent.Folders[0].Source != "tasks" {
		t.Fatalf("recent folders: %+v", recent.Folders)
	}
	var sources struct {
		Sources []managedagent.Source `json:"sources"`
	}
	s.do(http.MethodGet, "/api/v1/agent/sources", nil, &sources)
	if len(sources.Sources) != 1 || sources.Sources[0].Kind != managedagent.SourceKindLocal || sources.Sources[0].URL != dir {
		t.Fatalf("sources: %+v", sources.Sources)
	}

	// A second task on the same folder shares the workspace.
	d2 := s.createSession(map[string]any{"local_path": dir, "prompt": "And Germany?"})
	if d2.Workspace.ID != d.Workspace.ID {
		t.Fatalf("second session got a different workspace: %s vs %s", d2.Workspace.ID, d.Workspace.ID)
	}
	s.waitIdle(d2.Session.ID)

	// A plain folder (no git) is fine: the agent works, the diff is empty.
	plain := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	d3 := s.createSession(map[string]any{"local_path": plain, "prompt": "Hello"})
	s.waitIdle(d3.Session.ID)
	if code := s.do(http.MethodGet, "/api/v1/agent/sessions/"+d3.Session.ID+"/diff", nil, &diff); code != 200 || diff.ChangedFiles != 0 || diff.Patch != "" {
		t.Fatalf("plain folder diff: %d %+v", code, diff)
	}

	// Missing folder is a validation error, not a crash.
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions", map[string]any{"local_path": filepath.Join(plain, "nope"), "prompt": "x"}, nil); code != 400 {
		t.Fatalf("missing folder: want 400, got %d", code)
	}
	t.Logf("local folder journey ok\n%s", eventsDump(ev))
}
