package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// forkFixture is a service over an in-memory store and a fresh layout root.
func forkFixture(t *testing.T) (*Service, *db.Store, Layout) {
	t.Helper()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	layout := Layout{Root: realTempDir(t)}
	svc := NewService(store, layout, Agents{}, testHost{})
	t.Cleanup(func() { svc.Stop(context.Background()) })
	return svc, store, layout
}

// TestForkCopiesAssetsButNotConversations forks a project that has settings,
// a library, memory, threads and a conversation, and checks what came along
// and what did not.
func TestForkCopiesAssetsButNotConversations(t *testing.T) {
	svc, store, layout := forkFixture(t)
	repo := newRepo(t)
	p := mkProject(t, store, &db.Project{
		Name: "Origin", Goal: "Ship v1", Instructions: "Be careful.",
		Repos:                []db.Repo{{Path: repo, BaseBranch: "main"}},
		CoordinatorModel:     "coord-x",
		ThreadModel:          "thread-x",
		ThreadPermissionMode: "auto",
		MaxRunningThreads:    7,
		DisableWorktree:      true,
		Starred:              true,
		Paused:               true,
	})
	require.NoError(t, layout.Ensure(p.ID))
	write := func(rel, content string) {
		path := filepath.Join(layout.LibraryDir(p.ID), filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write("uploads/brief.md", "# brief")
	write("outputs/draft/report.md", "the report")
	require.NoError(t, os.MkdirAll(layout.TmpDir(p.ID, "old-thread"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(layout.TmpDir(p.ID, "old-thread"), "scratch.txt"), []byte("scratch"), 0o644))
	require.NoError(t, layout.WriteMemory(p.ID, "style.md", "Keep it short."))
	require.NoError(t, layout.WriteMemory(p.ID, MemoryIndex, "# Project memory\n\n- [Style](style.md) — how to write\n"))

	// A worker thread with a transcript, and a conversation on the main thread.
	worker := &db.Thread{ProjectID: p.ID, Title: "Draft the report", Status: db.ThreadReady, Origin: "coordinator"}
	require.NoError(t, store.CreateThread(worker))
	require.NoError(t, store.AddThreadEvent(&db.ThreadEvent{ThreadID: worker.ID, Kind: db.EvAssistant, Content: "done"}))
	require.NoError(t, store.AddThreadEvent(&db.ThreadEvent{ThreadID: mainThread(t, store, p.ID), Kind: db.EvUser, Content: "please draft the report"}))

	fork, err := svc.Fork(p.ID, "")
	require.NoError(t, err)

	// Settings came along; the fork is an active, unstarred project.
	require.Equal(t, "Origin (fork)", fork.Name)
	require.NotEqual(t, p.ID, fork.ID)
	require.Equal(t, p.Goal, fork.Goal)
	require.Equal(t, p.Instructions, fork.Instructions)
	require.Equal(t, p.Repos, fork.Repos)
	require.Equal(t, p.CoordinatorModel, fork.CoordinatorModel)
	require.Equal(t, p.ThreadModel, fork.ThreadModel)
	require.Equal(t, p.ThreadPermissionMode, fork.ThreadPermissionMode)
	require.Equal(t, p.MaxRunningThreads, fork.MaxRunningThreads)
	require.Equal(t, p.DisableWorktree, fork.DisableWorktree)
	require.False(t, fork.Starred)
	require.False(t, fork.Paused)
	require.Nil(t, fork.ArchivedAt)

	// The conversation and threads did not: a fresh main thread, no events,
	// no workers, no coordinator session.
	require.NotEqual(t, p.MainThreadID, fork.MainThreadID)
	main, err := store.GetThread(fork.MainThreadID)
	require.NoError(t, err)
	require.Equal(t, db.AgentLLM, main.Agent)
	require.Empty(t, main.SessionID)
	evs, err := store.ListThreadEvents(fork.MainThreadID)
	require.NoError(t, err)
	require.Empty(t, evs)
	threads, err := store.ListThreads(fork.ID)
	require.NoError(t, err)
	require.Empty(t, threads)

	// The library came along, scratch space did not.
	for rel, want := range map[string]string{
		"uploads/brief.md":        "# brief",
		"outputs/draft/report.md": "the report",
	} {
		f, _, err := layout.OpenLibraryFile(fork.ID, rel)
		require.NoError(t, err, rel)
		buf := make([]byte, 64)
		n, _ := f.Read(buf)
		require.NoError(t, f.Close())
		require.Equal(t, want, string(buf[:n]), rel)
	}
	_, err = os.Stat(layout.TmpDir(fork.ID, "old-thread"))
	require.True(t, os.IsNotExist(err), "scratch space must not be copied")

	// Memory came along, index included.
	got, err := layout.ReadMemory(fork.ID, "style.md")
	require.NoError(t, err)
	require.Equal(t, "Keep it short.", got)
	got, err = layout.ReadMemory(fork.ID, MemoryIndex)
	require.NoError(t, err)
	require.Contains(t, got, "[Style](style.md)")

	// The origin is untouched: its rows and its files are as they were.
	src, err := store.GetProject(p.ID)
	require.NoError(t, err)
	require.Equal(t, "Origin", src.Name)
	require.True(t, src.Starred)
	require.True(t, src.Paused)
	srcThreads, err := store.ListThreads(p.ID)
	require.NoError(t, err)
	require.Len(t, srcThreads, 1)
	srcConvo, err := store.ListThreadEvents(mainThread(t, store, p.ID))
	require.NoError(t, err)
	require.Len(t, srcConvo, 1)
	for _, rel := range []string{"uploads/brief.md", "outputs/draft/report.md", ".tmp/old-thread/scratch.txt"} {
		_, err := os.Stat(filepath.Join(layout.LibraryDir(p.ID), filepath.FromSlash(rel)))
		require.NoError(t, err, rel)
	}
}

// TestForkNamesExplicit forks under the name the caller gave.
func TestForkNamesExplicit(t *testing.T) {
	svc, store, layout := forkFixture(t)
	p := mkProject(t, store, &db.Project{Name: "Origin"})
	require.NoError(t, layout.Ensure(p.ID))
	fork, err := svc.Fork(p.ID, "  Custom name  ")
	require.NoError(t, err)
	require.Equal(t, "Custom name", fork.Name)
}

// TestForkFillsRowDefaults forks a row from before the permission-mode default
// existed: the fork starts where a new project would. max_running_threads
// needs no fill here — migrate backfills every row to >= 1 at open.
func TestForkFillsRowDefaults(t *testing.T) {
	svc, store, layout := forkFixture(t)
	p := mkProject(t, store, &db.Project{Name: "Origin"})
	require.NoError(t, layout.Ensure(p.ID))
	fork, err := svc.Fork(p.ID, "")
	require.NoError(t, err)
	require.Equal(t, DefaultPermissionMode, fork.ThreadPermissionMode)
}

// TestForkRejectsFolder refuses a working folder: its assets are the folder's
// own files, not the project's.
func TestForkRejectsFolder(t *testing.T) {
	svc, store, _ := forkFixture(t)
	folder, err := store.OpenFolder(realTempDir(t), "")
	require.NoError(t, err)
	_, err = svc.Fork(folder.ID, "")
	require.Error(t, err)
}

// TestForkRollsBackWhenCopyFails removes the source's directory, so the copy
// step fails after the fork's rows and directory were made: nothing of the
// fork may stay behind.
func TestForkRollsBackWhenCopyFails(t *testing.T) {
	svc, store, layout := forkFixture(t)
	p := mkProject(t, store, &db.Project{Name: "Origin"})
	require.NoError(t, os.RemoveAll(layout.Dir(p.ID)))

	_, err := svc.Fork(p.ID, "")
	require.Error(t, err)

	left, err := store.ListProjects("", false)
	require.NoError(t, err)
	require.Len(t, left, 1)
	require.Equal(t, p.ID, left[0].ID)
	entries, err := os.ReadDir(layout.Root)
	require.NoError(t, err)
	require.Empty(t, entries, "the fork's directory must be cleaned up")
}
