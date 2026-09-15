package e2e_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// Journey: pick a folder on this machine and work in it directly.
//
//	browse → start with a path → agent answers → diff shows the edit →
//	push refused (in place) → folder is "recent" and listed as a local source →
//	a second task on the same folder reuses the workspace → a plain (non-git)
//	folder works too, with an empty diff.
func TestJourney_LocalFolderInPlace(t *testing.T) {
	requireE2E(t)
	s := bootStack(t, nil)
	dir := newGitDir(t, "playground")

	// Allowlist: nothing handed over yet, so nothing can be listed — not
	// the folder, not its parent, not the top level.
	var listing managedagent.DirListing
	if code := s.do(http.MethodGet, "/api/v1/agent/fs/dirs", nil, &listing); code != 200 || len(listing.Entries) != 0 {
		t.Fatalf("top level before adding: %d %+v", code, listing)
	}
	for _, p := range []string{dir, filepath.Dir(dir)} {
		if code := s.do(http.MethodGet, "/api/v1/agent/fs/dirs?path="+p, nil, nil); code != 403 {
			t.Fatalf("browse %s before adding: want 403, got %d", p, code)
		}
	}
	if code := s.do(http.MethodGet, "/api/v1/agent/fs/dirs?path=relative/path", nil, nil); code != 400 {
		t.Fatalf("browse relative path: want 400, got %d", code)
	}

	d := s.createSession(map[string]any{"path": dir, "prompt": "What is the capital of France?"})
	id := d.Session.ID
	if d.Folder.Path != dir {
		t.Fatalf("the session must run in the folder itself, got %+v", d.Folder)
	}

	d, ev := s.waitIdle(id)
	if !hasEvent(ev, managedagent.EventAssistantMessage, protocoltest.VirtualMockAnswerMarker) {
		t.Fatalf("no assistant answer\n%s", eventsDump(ev))
	}
	if s.env.VirtualServer().CallCount() == 0 {
		t.Fatal("the virtual upstream never saw a request; the CLI routed elsewhere")
	}
	// Nothing of ours was created on disk beside the event log.
	entries, _ := os.ReadDir(filepath.Join(s.env.ConfigDir(), "agent"))
	for _, e := range entries {
		if e.Name() != "events" {
			t.Fatalf("the agent directory must hold only event logs, found %q", e.Name())
		}
	}

	// The agent's (simulated) edit shows up in the diff of the folder itself.
	if err := os.WriteFile(filepath.Join(dir, "NOTES.md"), []byte("by agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var diff managedagent.Diff
	if code := s.do(http.MethodGet, "/api/v1/agent/sessions/"+id+"/diff", nil, &diff); code != 200 || diff.ChangedFiles != 1 {
		t.Fatalf("diff: %d %+v", code, diff)
	}
	// The folder is now on the allowlist, most recently used first.
	var recent struct {
		Folders []managedagent.Folder `json:"folders"`
	}
	if code := s.do(http.MethodGet, "/api/v1/agent/folders", nil, &recent); code != 200 {
		t.Fatalf("folders: %d", code)
	}
	if len(recent.Folders) != 1 || recent.Folders[0].Path != dir || recent.Folders[0].ID != d.Folder.ID {
		t.Fatalf("folders: %+v", recent.Folders)
	}
	// Submitting the folder opened it — and only it — for browsing.
	if code := s.do(http.MethodGet, "/api/v1/agent/fs/dirs?path="+dir, nil, &listing); code != 200 || listing.Path != dir || !listing.IsRepo || listing.Parent != "" {
		t.Fatalf("browse after adding: %d %+v", code, listing)
	}
	if code := s.do(http.MethodGet, "/api/v1/agent/fs/dirs?path="+filepath.Dir(dir), nil, nil); code != 403 {
		t.Fatalf("parent must stay closed: %d", code)
	}
	if code := s.do(http.MethodGet, "/api/v1/agent/fs/dirs", nil, &listing); code != 200 || len(listing.Entries) != 1 || listing.Entries[0].Path != dir {
		t.Fatalf("top level after adding: %d %+v", code, listing)
	}
	// A second task in the same folder is fine, and reuses the folder.
	d2 := s.createSession(map[string]any{"path": dir, "prompt": "And Germany?"})
	if d2.Folder.ID != d.Folder.ID {
		t.Fatalf("the second task must run in the same folder: %s vs %s", d2.Folder.ID, d.Folder.ID)
	}
	s.waitIdle(d2.Session.ID)
	for _, sid := range []string{id, d2.Session.ID} {
		if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+sid+"/archive", nil, nil); code != 200 {
			t.Fatalf("archive %s: %d", sid, code)
		}
	}

	// Removing the folder withdraws the grant and leaves the directory alone.
	if code := s.do(http.MethodDelete, "/api/v1/agent/folders/"+d.Folder.ID, nil, nil); code != 204 {
		t.Fatalf("remove folder: %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "NOTES.md")); err != nil {
		t.Fatalf("removing a folder must not touch its contents: %v", err)
	}
	if code := s.do(http.MethodGet, "/api/v1/agent/fs/dirs?path="+dir, nil, nil); code != 403 {
		t.Fatalf("browsing a withdrawn folder: want 403, got %d", code)
	}

	// A plain folder (no git) is fine: the agent works, the diff is empty.
	plain := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	d3 := s.createSession(map[string]any{"path": plain, "prompt": "Hello"})
	s.waitIdle(d3.Session.ID)
	if code := s.do(http.MethodGet, "/api/v1/agent/sessions/"+d3.Session.ID+"/diff", nil, &diff); code != 200 || diff.ChangedFiles != 0 || diff.Patch != "" {
		t.Fatalf("plain folder diff: %d %+v", code, diff)
	}

	// Missing folder is a validation error, not a crash.
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions", map[string]any{"path": filepath.Join(plain, "nope"), "prompt": "x"}, nil); code != 400 {
		t.Fatalf("missing folder: want 400, got %d", code)
	}
	t.Logf("local folder journey ok\n%s", eventsDump(ev))
}
