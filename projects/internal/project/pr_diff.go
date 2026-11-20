package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A PR's diff is read from this checkout's refs when they hold both of its
// branches (no network, no token, and it is fast). When they do not (a branch
// another machine pushed, one that was deleted) GitHub's own list of the PR's
// files can stand in, but that is switched off (Threads.remoteDiff): it costs
// requests, so for now the PR says its branches are not here. See
// .design/github-tools.md, "A PR's diff".

// GitHubFile is one file of a PR as GitHub lists it.
type GitHubFile struct {
	Path, OldPath string
	// Status is GitHub's word: added, removed, modified, renamed, copied, changed.
	Status               string
	Additions, Deletions int
	// Patch is the file's hunks; GitHub leaves it out for a binary file and for
	// one that is too large.
	Patch string
}

const (
	// prFilesCacheFor is how long the files of one PR are kept, so that opening
	// its files one after the other asks GitHub once.
	prFilesCacheFor = 2 * time.Minute
	// prFilesMax is GitHub's own ceiling on the files of a PR it lists.
	prFilesMax = 3000
)

type prFilesEntry struct {
	at    time.Time
	files []GitHubFile
}

// PRFiles lists a PR's files from GitHub, kept for prFilesCacheFor.
func (l *Links) PRFiles(ctx context.Context, t *db.Thread, number int) ([]GitHubFile, error) {
	lc, err := l.resolve(ctx, t)
	if err != nil {
		return nil, err
	}
	key := lc.repo.Slug() + "#" + fmt.Sprint(number)
	l.filesMu.Lock()
	e, ok := l.files[key]
	l.filesMu.Unlock()
	if ok && l.now().Sub(e.at) < prFilesCacheFor {
		return e.files, nil
	}
	files, err := l.gh.PRFiles(ctx, lc.repo, number)
	if err != nil {
		return nil, err
	}
	l.filesMu.Lock()
	l.files[key] = prFilesEntry{at: l.now(), files: files}
	l.filesMu.Unlock()
	return files, nil
}

func githubStatus(s string) string {
	switch s {
	case "added", "renamed", "copied":
		return s
	case "removed":
		return "deleted"
	}
	return "modified"
}

// remotePRDiff is the summary of a PR from GitHub's list of its files. A file
// without a patch and without changed lines is binary (or only moved).
func remotePRDiff(number int, base, head string, files []GitHubFile) Diff {
	d := Diff{Number: number, Base: base, Head: head, Source: "github"}
	list := make([]DiffFile, 0, len(files))
	for _, f := range files {
		list = append(list, DiffFile{
			Path: f.Path, OldPath: f.OldPath, Status: githubStatus(f.Status),
			Additions: f.Additions, Deletions: f.Deletions,
			Binary:    f.Patch == "" && f.Additions+f.Deletions == 0 && f.Status != "renamed" && f.Status != "copied",
			Generated: generatedPath(f.Path),
		})
	}
	d.setFiles(list)
	return d
}

// remotePRFileDiff is one file's patch from GitHub's list. GitHub sends the
// hunks only, so the header git would have printed is put back for the viewer.
func remotePRFileDiff(files []GitHubFile, path string) (FileDiff, error) {
	for _, f := range files {
		if f.Path != path {
			continue
		}
		d := FileDiff{Path: path}
		switch {
		case f.Patch == "" && f.Additions+f.Deletions == 0:
			d.Binary = true
		case f.Patch == "" || len(f.Patch) > maxFilePatch:
			d.TooLarge = true
		default:
			old := f.OldPath
			if old == "" {
				old = f.Path
			}
			header := fmt.Sprintf("diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", old, f.Path, old, f.Path)
			d.Patch, d.LinesCut = cutLongLines(header + f.Patch + "\n")
		}
		return d, nil
	}
	return FileDiff{}, fmt.Errorf("%s is not one of the PR's files", path)
}

// prOf is the PR a thread holds a link to, and only an open one: its diff is
// what is being reviewed; a merged or closed PR has nothing left to review.
func prOf(t *db.Thread, number int) (db.ThreadLink, error) {
	for _, k := range t.Links {
		if k.Kind == db.LinkPR && k.Number == number {
			if k.State != db.LinkStateOpen {
				return k, fmt.Errorf("PR #%d is %s: only an open PR's changes are shown", number, k.State)
			}
			if k.Head == "" || k.Base == "" {
				return k, fmt.Errorf("PR #%d has not been read from GitHub yet: refresh it and try again", number)
			}
			return k, nil
		}
	}
	return db.ThreadLink{}, fmt.Errorf("#%d is not one of this thread's PRs", number)
}

func (m *Threads) linksOrNil() *Links {
	if m.links == nil {
		return nil
	}
	return m.links()
}

// prDiffSource finds one of a thread's open PRs and where its diff is read:
// the checkout when it holds both branches (dir), else GitHub when that read
// is switched on (links), else errNoLocalRefs.
func (m *Threads) prDiffSource(ctx context.Context, threadID string, number int) (t *db.Thread, k db.ThreadLink, links *Links, err error) {
	if t, err = m.store.GetThread(threadID); err != nil {
		return nil, k, nil, err
	}
	if k, err = prOf(t, number); err != nil {
		return nil, k, nil, err
	}
	if t.Cwd != "" {
		if _, err = localPRRange(ctx, t.Cwd, k.Base, k.Head); err == nil {
			return t, k, nil, nil
		}
	}
	if links = m.linksOrNil(); links == nil || !m.remoteDiff {
		return nil, k, nil, errNoLocalRefs
	}
	return t, k, links, nil
}

// PRDiff returns the changes of one of a thread's open PRs.
func (m *Threads) PRDiff(ctx context.Context, threadID string, number int) (Diff, error) {
	t, k, links, err := m.prDiffSource(ctx, threadID, number)
	if err != nil {
		return Diff{}, err
	}
	if links == nil {
		return LocalPRDiff(ctx, t.Cwd, number, k.Base, k.Head)
	}
	files, err := links.PRFiles(ctx, t, number)
	if err != nil {
		return Diff{}, err
	}
	return remotePRDiff(number, k.Base, k.Head, files), nil
}

// PRDiffFile returns one file of one of a thread's open PRs.
func (m *Threads) PRDiffFile(ctx context.Context, threadID string, number int, path, oldPath string) (FileDiff, error) {
	t, k, links, err := m.prDiffSource(ctx, threadID, number)
	if err != nil {
		return FileDiff{}, err
	}
	if links == nil {
		return LocalPRFileDiff(ctx, t.Cwd, k.Base, k.Head, path, oldPath)
	}
	files, err := links.PRFiles(ctx, t, number)
	if err != nil {
		return FileDiff{}, err
	}
	return remotePRFileDiff(files, path)
}

// MergeDraft is what the merge dialog starts from: the words of the PR, as
// `gh pr create --fill` and `gh pr merge` take them.
type MergeDraft struct {
	// Number is the open PR of the thread's branch, 0 when there is none yet.
	Number int `json:"number"`
	// Source is "pr" (the open PR's own title and description, read from
	// GitHub now), "commit" (the branch's one commit) or "commits" (several).
	Source string `json:"source"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// MergeDraft reads the title and description a merge starts from.
func (m *Threads) MergeDraft(ctx context.Context, threadID string) (MergeDraft, error) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return MergeDraft{}, err
	}
	if t.Branch == "" {
		return MergeDraft{}, errors.New("this thread has no worktree branch to merge")
	}
	for _, k := range t.Links {
		if k.Kind == db.LinkPR && k.State == db.LinkStateOpen && k.Head == t.Branch {
			// The description is not recorded, so GitHub is asked, unless its budget is spent.
			if links := m.linksOrNil(); links != nil && links.pausedUntil().IsZero() {
				if repo, err := repoOfLink(&k); err == nil {
					if _, it, err := links.read(ctx, repo, k.Number); err == nil && it.IsPR {
						return MergeDraft{Number: k.Number, Source: "pr", Title: it.Title, Body: strings.TrimSpace(it.Body)}, nil
					}
				}
			}
			// GitHub could not be read: the record's own title is still the PR's.
			return MergeDraft{Number: k.Number, Source: "pr", Title: k.Title}, nil
		}
	}
	title, body, source := fillPR(ctx, t)
	return MergeDraft{Source: source, Title: title, Body: body}, nil
}

// fillPR takes a new PR's title and description from the branch's commits the
// way `gh pr create --fill` does: one commit gives its subject and body; several
// give the branch name, spelled out, and a list of their subjects.
func fillPR(ctx context.Context, t *db.Thread) (title, body, source string) {
	if t.BaseBranch == "" {
		return t.Title, "", "commits"
	}
	var subjects, bodies []string
	// The base may only be on origin (a checkout that never made it locally).
	for _, ref := range []string{t.BaseBranch, "origin/" + t.BaseBranch} {
		out, err := git(ctx, t.Cwd, "log", "--reverse", "--format=%s%x1f%b%x1e", ref+"..refs/heads/"+t.Branch)
		if err != nil {
			continue
		}
		for _, rec := range strings.Split(out, "\x1e") {
			if rec = strings.Trim(rec, "\n"); rec != "" {
				s, b, _ := strings.Cut(rec, "\x1f")
				subjects, bodies = append(subjects, strings.TrimSpace(s)), append(bodies, strings.TrimSpace(b))
			}
		}
		break
	}
	switch len(subjects) {
	case 0:
		return t.Title, "", "commits"
	case 1:
		return subjects[0], bodies[0], "commit"
	}
	var b strings.Builder
	for _, s := range subjects {
		b.WriteString("- " + s + "\n")
	}
	return humanBranch(t.Branch), strings.TrimSpace(b.String()), "commits"
}

// humanBranch spells a branch name out as gh does: separators become spaces
// and the first letter is a capital.
func humanBranch(name string) string {
	s := strings.NewReplacer("-", " ", "_", " ").Replace(name)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
