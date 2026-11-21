package project

// Cleanup closes a thread's workspace down in stages: Archive frees the
// worktree but keeps the branch, the record and the transcript (Restore can
// rebuild the worktree from the branch); Delete removes what is left of an
// archived thread — the branch (only provably merged, or forced with a
// confirmation and a trash ref), its scratch and settings files, and its
// rows. The library is never touched: uploads and outputs are the project's,
// not the thread's. Both actions are guarded by Cleanup, one report used by
// the preview and re-checked at execution time.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// ErrNotArchived is a delete asked of a thread that is not archived: deleting
// is permanent, so it sits behind an archive.
var ErrNotArchived = errors.New("archive the thread first; delete is permanent")

// CleanupReport is what archive or delete would do to a thread right now:
// sizes, what blocks it, what is only a warning, and what is kept.
type CleanupReport struct {
	ThreadID string `json:"thread_id"`
	// WorktreeBytes and TmpBytes are what archive would free (best effort;
	// a worktree already freed or a missing directory counts as 0).
	WorktreeBytes int64    `json:"worktree_bytes"`
	TmpBytes      int64    `json:"tmp_bytes"`
	Blockers      []string `json:"blockers,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	// Kept says what survives, so "archive" is not read as "throw away".
	Kept       []string `json:"kept"`
	CanArchive bool     `json:"can_archive"`
	CanDelete  bool     `json:"can_delete"`
}

// BlockedArchive refuses an archive or a delete that has blockers. It is a
// 409 at the API layer, with the blockers listed in the body.
type BlockedArchive struct{ Blockers []string }

func (e *BlockedArchive) Error() string { return strings.Join(e.Blockers, "; ") }

// Cleanup computes the report for a thread: what archive (or delete, once
// archived) would free and what stands in the way. It never changes anything.
func (m *Threads) Cleanup(threadID string) (*CleanupReport, error) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if t.Agent == db.AgentLLM {
		return nil, ErrNotWorker
	}
	r := &CleanupReport{ThreadID: threadID}
	// Sizes: what archive frees. Kept is unconditional — archive and delete
	// keep the transcript, and delete keeps the project's library.
	r.Kept = append(r.Kept,
		"the conversation (every event)",
		"the library's uploads and outputs")
	if t.Branch != "" {
		r.Kept = append(r.Kept, "branch "+t.Branch+" (until the thread is deleted)")
	}
	// Only a registered worktree of the thread's repository counts as
	// freeable: a folder session's cwd is the user's own folder, and its
	// size is nobody's business here.
	if t.RepoPath != "" && t.Cwd != "" && registeredWorktree(t.RepoPath, t.Cwd) {
		r.WorktreeBytes = dirSize(t.Cwd)
	}
	r.TmpBytes = dirSize(m.layout.TmpDir(t.ProjectID, t.ID))
	if t.ArchivedAt != nil {
		r.CanDelete = true
		if t.WorktreeFreedAt != nil {
			r.WorktreeBytes = 0
		}
		r.Blockers = m.deleteBlockers(t)
		r.CanDelete = len(r.Blockers) == 0
		return r, nil
	}
	r.CanArchive = true
	r.Blockers = m.archiveBlockers(t)
	r.Warnings = m.cleanupWarnings(t)
	r.CanArchive = len(r.Blockers) == 0
	return r, nil
}

// archiveBlockers refuses an archive that would lose work or tear a live
// thread down: a running turn, a message waiting for a slot, or uncommitted
// changes in the worktree (the branch is kept, but uncommitted work lives
// only in the directory).
func (m *Threads) archiveBlockers(t *db.Thread) []string {
	var out []string
	if m.Running(t.ID) {
		out = append(out, "the thread is working; stop it first")
	}
	if t.Queued {
		out = append(out, "a message is queued for this thread")
	}
	if t.Cwd != "" && t.RepoPath != "" && t.WorktreeFreedAt == nil {
		if !registeredWorktree(t.RepoPath, t.Cwd) {
			out = append(out, "the working directory is not this repository's worktree; free it by hand")
		} else if s := Stats(context.Background(), t.Cwd, t.BaseBranch); s.Uncommitted {
			out = append(out, "the worktree has uncommitted changes; commit them first")
		}
	}
	return out
}

// cleanupWarnings says what the user should know but need not act on: the
// branch keeps committed work, so unpushed or unmerged commits and an open
// PR are fine to archive with.
func (m *Threads) cleanupWarnings(t *db.Thread) []string {
	var out []string
	if t.Branch != "" && t.RepoPath != "" && !branchPushed(t.RepoPath, t.Branch) {
		out = append(out, "the branch has commits that were never pushed; they stay on the local branch "+t.Branch)
	}
	if t.CommitsAhead > 0 && t.MergedAt == nil && !hasMergedPR(t) {
		out = append(out, "the branch is not merged yet; archive keeps the branch, the work is not lost")
	}
	for _, k := range t.Links {
		if k.Kind == db.LinkPR && k.State == db.LinkStateOpen && k.Head == t.Branch {
			out = append(out, fmt.Sprintf("PR #%d is still open on GitHub; archiving does not close it", k.Number))
			break
		}
	}
	return out
}

// deleteBlockers refuses a delete that would destroy the only copy of work:
// unmerged commits need force, and a branch checked out in another worktree
// cannot be deleted at all.
func (m *Threads) deleteBlockers(t *db.Thread) []string {
	var out []string
	if t.Branch == "" || t.RepoPath == "" || !BranchExists(context.Background(), t.RepoPath, t.Branch) {
		return out
	}
	if where := branchCheckedOut(t.RepoPath, t.Branch); where != "" {
		out = append(out, "the branch is checked out in "+where+"; remove that worktree first")
	}
	return out
}

// DeleteSpec says how hard a delete pushes: Force with ConfirmBranch names
// the branch, deleting one that no merged PR proves merged. The trash ref
// keeps the commits reachable for a while.
type DeleteSpec struct {
	Force         bool
	ConfirmBranch string
}

// Delete removes an archived thread for good: the branch (guarded), its
// scratch and settings files, and its rows (events, bindings, delivery
// state). The project's library is not touched.
func (m *Threads) Delete(threadID string, spec DeleteSpec) error {
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return err
	}
	if t.Agent == db.AgentLLM {
		return ErrNotWorker
	}
	if t.ArchivedAt == nil {
		return ErrNotArchived
	}
	if m.Running(t.ID) {
		return &BlockedArchive{Blockers: []string{"the thread is working; stop it first"}}
	}
	if t.Branch != "" && t.RepoPath != "" && BranchExists(context.Background(), t.RepoPath, t.Branch) {
		if err := m.deleteBranch(t, spec); err != nil {
			return err
		}
	}
	// The chat bindings move to the parent first (as on resolve); whatever
	// still names this thread — a folder session's chat, a target binding —
	// goes with the row, and the chat starts a fresh session next time.
	m.releaseChats(t)
	if err := m.store.DeleteThreadCascade(threadID); err != nil {
		return err
	}
	m.cleanTmp(t)
	for _, path := range []string{
		m.layout.SettingsFile(t.ProjectID, threadID),
		m.layout.SessionSettingsFile(threadID),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logrus.WithError(err).WithField("thread_id", threadID).Warn("projects: cannot remove the thread's settings file")
		}
	}
	logrus.WithField("thread_id", threadID).Info("projects: thread deleted")
	return nil
}

// deleteBranch removes the thread's branch. A merged PR (in the thread's
// links, or a squash that left no ancestor relation) is proof enough; an
// unmerged branch needs Force and the branch name typed back, and keeps its
// commits under refs/tingly/trash/<branch> so nothing is truly lost.
func (m *Threads) deleteBranch(t *db.Thread, spec DeleteSpec) error {
	if where := branchCheckedOut(t.RepoPath, t.Branch); where != "" {
		return &BlockedArchive{Blockers: []string{"the branch is checked out in " + where + "; remove that worktree first"}}
	}
	merged := t.MergedAt != nil || hasMergedPR(t)
	if !merged && (!spec.Force || spec.ConfirmBranch != t.Branch) {
		return &BlockedArchive{Blockers: []string{fmt.Sprintf(
			"branch %s has no merged PR; to delete it anyway, confirm with the branch name — its commits stay under refs/tingly/trash/%s",
			t.Branch, t.Branch)}}
	}
	if !merged {
		// A cheap pointer, not a backup: the commits stay reachable until the
		// repository's gc prunes unreferenced refs.
		if _, err := git(context.Background(), t.RepoPath, "update-ref", "refs/tingly/trash/"+t.Branch, "refs/heads/"+t.Branch); err != nil {
			return fmt.Errorf("keeping the trash ref: %w", err)
		}
	}
	// -D, not -d: this repository squash-merges, so even a provably merged
	// branch is not an ancestor of the base and -d would refuse it. Nothing
	// is lost — merged means GitHub holds the work, otherwise the trash ref
	// above does.
	if _, err := git(context.Background(), t.RepoPath, "branch", "-D", t.Branch); err != nil {
		return err
	}
	return nil
}

// Archive closes a thread down and frees its worktree: it resolves the
// thread (stopping the turn), removes the worktree directory and its git
// registration, and deletes the scratch. The branch, the record and the
// transcript stay; Restore rebuilds the worktree from the branch at the same
// path. A folder session has no worktree: archiving it is the old flag-only
// behavior.
func (m *Threads) Archive(threadID string) (*db.Thread, error) {
	// A folder session keeps its own archive: the status stays (a failed
	// session stays failed), nothing is resolved and nothing is freed, so
	// restoring puts it back exactly as it was. Its archive flag only hides
	// it from active work.
	if t, err := m.store.GetThread(threadID); err == nil {
		if p, err := m.store.GetProject(t.ProjectID); err == nil && p.Type == db.ProjectFolder {
			return m.ArchiveSession(threadID)
		}
	}
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if t.Agent == db.AgentLLM {
		return nil, ErrNotWorker
	}
	if blockers := m.archiveBlockers(t); len(blockers) > 0 {
		return nil, &BlockedArchive{Blockers: blockers}
	}
	// As in Resolve: let the turn take its interrupt before the process is
	// closed, then evict; nothing can start a session, the lock is held.
	if done := m.stopEnding(threadID); done != nil {
		select {
		case <-done:
		case <-time.After(agentboot.SessionCloseTimeout):
		}
	}
	m.engine.Evict(m.sessionOf(threadID))
	// The worktree goes, the branch stays. A dirty worktree was refused
	// above; --force is for submodules and ignored files (node_modules).
	if t.RepoPath != "" && t.Cwd != "" && t.WorktreeFreedAt == nil && registeredWorktree(t.RepoPath, t.Cwd) {
		if _, err := git(context.Background(), t.RepoPath, "worktree", "remove", "--force", t.Cwd); err != nil {
			return nil, fmt.Errorf("freeing the worktree: %w", err)
		}
		_, _ = git(context.Background(), t.RepoPath, "worktree", "prune")
	}
	t, err = m.update(threadID, func(t *db.Thread) {
		now := time.Now()
		if t.ResolvedAt == nil {
			t.ResolvedAt = &now
			t.Status, t.WaitReason = db.ThreadResolved, ""
		}
		if t.ArchivedAt == nil {
			t.ArchivedAt = &now
		}
		if t.RepoPath != "" && t.WorktreeFreedAt == nil {
			t.WorktreeFreedAt = &now
		}
	})
	if err == nil {
		m.releaseChats(t)
		m.cleanTmp(t)
		// After the save, as in Resolve.
		m.revokeTools(threadID)
		kept := "the branch stays"
		if t.Branch != "" {
			kept = "branch " + t.Branch + " stays"
		}
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "Archived; the worktree was freed, " + kept + ". Restore brings the worktree back."})
	}
	return t, err
}

// Restore returns an archived thread to the list and rebuilds its worktree
// from the branch, at the same path (so the transcript and `--resume` keep
// working). A missing branch restores the record only and says so.
func (m *Threads) Restore(threadID string) (*db.Thread, error) {
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if t.ArchivedAt == nil && t.WorktreeFreedAt == nil {
		return t, nil // not archived; nothing to do
	}
	note := ""
	if t.WorktreeFreedAt != nil && t.RepoPath != "" && t.Cwd != "" && t.Branch != "" {
		switch {
		case !BranchExists(context.Background(), t.RepoPath, t.Branch):
			note = "the branch " + t.Branch + " no longer exists; the thread is restored as a record only"
		case dirExists(t.Cwd):
			return nil, fmt.Errorf("cannot restore: %s already exists", t.Cwd)
		default:
			if _, err := git(context.Background(), t.RepoPath, "worktree", "add", t.Cwd, t.Branch); err != nil {
				return nil, fmt.Errorf("rebuilding the worktree: %w", err)
			}
		}
	}
	t, err = m.update(threadID, func(t *db.Thread) {
		t.ArchivedAt = nil
		t.WorktreeFreedAt = nil
	})
	if err == nil {
		m.publish(t)
		if note != "" {
			m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: note})
		}
	}
	return t, err
}

// hasMergedPR reports whether the thread's links hold a merged PR whose head
// is the thread's branch. It is the only merge proof this machine can check
// by itself; the repository squash-merges, so ancestry proves nothing.
func hasMergedPR(t *db.Thread) bool {
	for _, k := range t.Links {
		if k.Kind == db.LinkPR && k.State == db.LinkStateMerged && (t.Branch == "" || k.Head == t.Branch) {
			return true
		}
	}
	return false
}

// registeredWorktree reports whether dir is a worktree git registers for
// repo — the safety net that keeps every removal on real worktrees, never an
// arbitrary path.
func registeredWorktree(repo, dir string) bool {
	out, err := git(context.Background(), repo, "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") && strings.TrimPrefix(line, "worktree ") == dir {
			return true
		}
	}
	return false
}

// branchCheckedOut returns the worktree dir that has the branch checked out,
// or "" when none does.
func branchCheckedOut(repo, branch string) string {
	out, err := git(context.Background(), repo, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	dir := ""
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			dir = strings.TrimPrefix(line, "worktree ")
		case line == "branch refs/heads/"+branch && dir != "":
			return dir
		case line == "":
			dir = ""
		}
	}
	return ""
}

// branchPushed reports whether the local branch exists on the remote at the
// same commit. An absent remote tracking branch counts as not pushed.
func branchPushed(repo, branch string) bool {
	local, err := git(context.Background(), repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		return false
	}
	remote, err := git(context.Background(), repo, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	return err == nil && local == remote
}

// dirSize walks dir and sums the regular files. Missing and unreadable paths
// count as what could be measured; 0 is the honest answer for "nothing to
// free".
func dirSize(dir string) int64 {
	if dir == "" {
		return 0
	}
	var total int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	_ = err
	return total
}
