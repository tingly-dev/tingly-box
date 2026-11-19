package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// archiveWithThread starts a thread on a fresh fixture, waits for it to go
// idle, drops a scratch file (with a read-only directory, as a Go module
// cache leaves behind) into its TMPDIR and archives it.
func archiveWithThread(t *testing.T) (*testFixture, *db.Thread) {
	t.Helper()
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	th, err := f.threads.Start(t.Context(), f.project, StartSpec{Title: "Add feature", Task: "x", Origin: "coordinator"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	tmp := f.threads.layout.TmpDir(f.project.ID, th.ID)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "go", "pkg", "mod", "locked"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go", "pkg", "mod", "locked", "cache.bin"), []byte("cache"), 0o444))
	require.NoError(t, os.Chmod(filepath.Join(tmp, "go", "pkg", "mod", "locked"), 0o555))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "notes.txt"), []byte("scratch"), 0o644))
	cache := filepath.Join(f.threads.layout.LibraryDir(f.project.ID), ".cache", "go-mod", "cache.bin")
	require.NoError(t, os.MkdirAll(filepath.Dir(cache), 0o755))
	require.NoError(t, os.WriteFile(cache, []byte("cache"), 0o644))
	archived, err := f.threads.Archive(th.ID)
	require.NoError(t, err)
	return f, archived
}

func TestArchiveFreesTheWorktreeAndKeepsTheBranch(t *testing.T) {
	f, archived := archiveWithThread(t)

	// The worktree is gone, from disk and from git's registration.
	assert.NoDirExists(t, archived.Cwd)
	assert.NotContains(t, run(t, f.repo, "git", "-C", f.repo, "worktree", "list", "--porcelain"), archived.Cwd)
	// The branch stays.
	assert.True(t, BranchExists(t.Context(), f.repo, archived.Branch), "the branch is kept")
	// The scratch went too, read-only cache included.
	assert.NoDirExists(t, f.threads.layout.TmpDir(f.project.ID, archived.ID))
	// The project's shared cache is not the thread's to delete.
	assert.FileExists(t, filepath.Join(f.threads.layout.LibraryDir(f.project.ID), ".cache", "go-mod", "cache.bin"))
	// The thread is resolved and archived, with the freed worktree marked.
	assert.NotNil(t, archived.ResolvedAt)
	assert.NotNil(t, archived.ArchivedAt)
	assert.NotNil(t, archived.WorktreeFreedAt)
	assert.Equal(t, db.ThreadResolved, archived.Status)
	// The transcript says what happened.
	events, err := f.store.ListThreadEvents(archived.ID)
	require.NoError(t, err)
	last := events[len(events)-1]
	assert.Equal(t, db.EvSystem, last.Kind)
	assert.Contains(t, last.Content, archived.Branch)
}

func TestRestoreRebuildsTheWorktreeAtTheSamePath(t *testing.T) {
	f, archived := archiveWithThread(t)
	restored, err := f.threads.Restore(archived.ID)
	require.NoError(t, err)
	assert.Nil(t, restored.ArchivedAt)
	assert.Nil(t, restored.WorktreeFreedAt)
	assert.DirExists(t, restored.Cwd)
	assert.Equal(t, archived.Cwd, restored.Cwd, "the same path, so the transcript and --resume keep working")
	assert.Contains(t, run(t, f.repo, "git", "-C", f.repo, "worktree", "list", "--porcelain"), restored.Cwd)
	// A second restore is a no-op.
	again, err := f.threads.Restore(restored.ID)
	require.NoError(t, err)
	assert.Nil(t, again.ArchivedAt)
}

func TestRestoreWithTheBranchGoneRestoresTheRecordOnly(t *testing.T) {
	f, archived := archiveWithThread(t)
	run(t, f.repo, "git", "-C", f.repo, "branch", "-D", archived.Branch)
	restored, err := f.threads.Restore(archived.ID)
	require.NoError(t, err)
	assert.Nil(t, restored.ArchivedAt)
	assert.NoDirExists(t, restored.Cwd)
	events, _ := f.store.ListThreadEvents(restored.ID)
	last := events[len(events)-1]
	assert.Contains(t, last.Content, "no longer exists")
}

func TestArchiveRefusesADirtyWorktree(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	th, err := f.threads.Start(t.Context(), f.project, StartSpec{Title: "Add feature", Task: "x", Origin: "coordinator"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	require.NoError(t, os.WriteFile(filepath.Join(th.Cwd, "draft.txt"), []byte("half done"), 0o644))
	tmp := f.threads.layout.TmpDir(f.project.ID, th.ID)
	require.NoError(t, os.MkdirAll(tmp, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "scratch.txt"), []byte("s"), 0o644))

	_, err = f.threads.Archive(th.ID)
	var blocked *BlockedArchive
	require.ErrorAs(t, err, &blocked)
	assert.Contains(t, blocked.Blockers[0], "uncommitted")
	assert.DirExists(t, th.Cwd, "the worktree is untouched")
	saved, _ := f.store.GetThread(th.ID)
	assert.Nil(t, saved.ArchivedAt)
	// The report says the same, ahead of time.
	report, err := f.threads.Cleanup(th.ID)
	require.NoError(t, err)
	assert.False(t, report.CanArchive)
	assert.NotEmpty(t, report.Blockers)
	assert.Greater(t, report.WorktreeBytes, int64(0))
	assert.Greater(t, report.TmpBytes, int64(0))
}

func TestArchiveWarnsAboutUnpushedWork(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	th, err := f.threads.Start(t.Context(), f.project, StartSpec{Title: "Add feature", Task: "x", Origin: "coordinator"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	require.NoError(t, os.WriteFile(filepath.Join(th.Cwd, "feature.go"), []byte("package main\n"), 0o644))
	run(t, th.Cwd, "git", "-C", th.Cwd, "add", ".")
	run(t, th.Cwd, "git", "-C", th.Cwd, "commit", "-qm", "wip")
	report, err := f.threads.Cleanup(th.ID)
	require.NoError(t, err)
	assert.True(t, report.CanArchive, "unpushed work warns, it does not block: %v", report.Blockers)
	found := false
	for _, w := range report.Warnings {
		found = found || strings.Contains(w, "never pushed")
	}
	assert.True(t, found, "warnings: %v", report.Warnings)
	assert.Contains(t, report.Kept[len(report.Kept)-1], "branch tp/")
}

func TestDeleteNeedsAnArchiveAndAMergedBranch(t *testing.T) {
	f, archived := archiveWithThread(t)

	// Not archived is refused outright.
	fresh := newFixture(t, func(*enginetest.Turn) error { return nil })
	th, err := fresh.threads.Start(t.Context(), fresh.project, StartSpec{Title: "Second", Task: "y"})
	require.NoError(t, err)
	waitStatus(t, fresh.store, th.ID, db.ThreadIdle)
	require.ErrorIs(t, fresh.threads.Delete(th.ID, DeleteSpec{}), ErrNotArchived)

	// Archived, but the branch has no merged PR: refused without force.
	err = f.threads.Delete(archived.ID, DeleteSpec{})
	var blocked *BlockedArchive
	require.ErrorAs(t, err, &blocked)
	assert.Contains(t, blocked.Blockers[0], "no merged PR")

	// Wrong confirmation is still refused.
	require.ErrorAs(t, f.threads.Delete(archived.ID, DeleteSpec{Force: true, ConfirmBranch: "wrong"}), &blocked)

	// Force with the branch name deletes everything but the library.
	outputs := filepath.Join(f.threads.layout.LibraryDir(f.project.ID), "outputs", "add-feature")
	require.NoError(t, os.MkdirAll(outputs, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outputs, "report.md"), []byte("# r"), 0o644))
	require.NoError(t, f.threads.Delete(archived.ID, DeleteSpec{Force: true, ConfirmBranch: archived.Branch}))
	_, err = f.store.GetThread(archived.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)
	events, _ := f.store.ListThreadEvents(archived.ID)
	assert.Empty(t, events)
	assert.False(t, BranchExists(t.Context(), f.repo, archived.Branch))
	assert.DirExists(t, outputs, "the library's outputs are not the thread's to delete")
	assert.FileExists(t, filepath.Join(f.threads.layout.LibraryDir(f.project.ID), ".cache", "go-mod", "cache.bin"),
		"the shared cache belongs to the project, not the thread")
	_, err = os.Stat(f.threads.layout.SettingsFile(f.project.ID, archived.ID))
	assert.True(t, os.IsNotExist(err), "the settings file went with the thread")
}

func TestDeleteWithAMergedPRNeedsNoForce(t *testing.T) {
	f, archived := archiveWithThread(t)
	require.NoError(t, f.store.UpdateThreadLinks(archived.ID, func(links []db.ThreadLink) []db.ThreadLink {
		return append(links, db.ThreadLink{Kind: db.LinkPR, Repo: "t/r", Number: 7, State: db.LinkStateMerged, Head: archived.Branch})
	}))
	require.NoError(t, f.threads.Delete(archived.ID, DeleteSpec{}))
	assert.False(t, BranchExists(t.Context(), f.repo, archived.Branch))
	_, err := f.store.GetThread(archived.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestDeleteKeepsTheBranchUnderATrashRef(t *testing.T) {
	f, archived := archiveWithThread(t)
	require.NoError(t, f.threads.Delete(archived.ID, DeleteSpec{Force: true, ConfirmBranch: archived.Branch}))
	out := run(t, f.repo, "git", "-C", f.repo, "for-each-ref", "refs/tingly/trash/", "--format=%(refname)")
	assert.Contains(t, out, "refs/tingly/trash/"+archived.Branch)
}
