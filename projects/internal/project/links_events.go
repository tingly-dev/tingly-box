package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// What a thread's PR changing on GitHub tells the coordinator. See
// .design/thread-events.md: only a change counts, the thread records it with the
// facts at that moment, and the coordinator is woken once per project with all
// the changes of one sweep. Nothing here changes a thread's status or sends
// the thread anything.

// ReportGitHub is a report kind for what happened to a thread's PRs on GitHub.
// Like a finished report it is information for the coordinator, whose reply
// is what the chats hear.
const ReportGitHub = "github"

// Kinds of a GitHub event, as the envelope's kind attribute says them. A kind
// for conflicts joins them in its own change: add it to prTransition (what
// counts), prEvent.sentence (what is said) and the prompt.
const (
	EventPRMerged = "pr_merged"
	EventPRClosed = "pr_closed"
)

// prChange is one change of a PR the thread opened: Link is the record as it
// is now.
type prChange struct {
	ThreadID string
	Kind     string
	Link     db.ThreadLink
}

// prTransition is the kind of event a record's move from state before to after
// is, or "" when it is none. Only a PR the thread opened, and only the move
// out of open: a finished record read again is no change.
func prTransition(before string, after *db.ThreadLink) string {
	if after.Kind != db.LinkPR || after.Role != db.LinkOpened || before != db.LinkStateOpen {
		return ""
	}
	switch after.State {
	case db.LinkStateMerged:
		return EventPRMerged
	case db.LinkStateClosed:
		return EventPRClosed
	}
	return ""
}

// changeOf is the change a record's move from state before to the state it now
// has is, or nil.
func changeOf(threadID, before string, after *db.ThreadLink) *prChange {
	kind := prTransition(before, after)
	if kind == "" {
		return nil
	}
	return &prChange{ThreadID: threadID, Kind: kind, Link: *after}
}

// deliver hands changes to whoever tells the coordinator; nils are skipped.
func (l *Links) deliver(changes ...*prChange) {
	var out []prChange
	for _, c := range changes {
		if c != nil {
			out = append(out, *c)
		}
	}
	if len(out) > 0 && l.events != nil {
		l.events(out)
	}
}

// prFacts is what is true of a thread when its PR changed, read once from what
// the system already knows. It says what is so and never what to do about it.
type prFacts struct {
	Status  string
	Running bool
	// Commits are the commits on the PR's local branch that origin does not
	// have; CommitsKnown is false when that could not be read.
	Commits      int
	CommitsKnown bool
	// Others are the thread's other open PRs.
	Others      []openPR
	Uncommitted bool
}

// openPR is a PR of the thread that is still open, and what it is based on.
type openPR struct {
	Number int
	Base   string
}

// factsOf reads the facts of thread t (as stored now) for a change of its PR k.
func factsOf(t *db.Thread, k db.ThreadLink, running bool) prFacts {
	f := prFacts{Status: t.Status, Running: running, Uncommitted: t.Uncommitted}
	for _, o := range t.Links {
		if o.Kind != db.LinkPR || o.State != db.LinkStateOpen || o.Number == k.Number {
			continue
		}
		f.Others = append(f.Others, openPR{o.Number, o.Base})
	}
	if t.RepoPath != "" && k.Head != "" {
		ctx := context.Background()
		if n, err := git(ctx, t.RepoPath, "rev-list", "--count", "refs/remotes/origin/"+k.Head+"..refs/heads/"+k.Head); err == nil {
			_, err = fmt.Sscan(n, &f.Commits)
			f.CommitsKnown = err == nil
		}
	}
	return f
}

// prEvent is one change as the coordinator is told it and as the thread notes
// it. Thread fields say whose PR it is.
type prEvent struct {
	ThreadID string `json:"thread_id"`
	Seq      int    `json:"seq"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	// Item is "owner/repo#number".
	Item string `json:"item"`
	// Said is the sentence about the PR with its title (GitHub's words, so
	// relayed text); Note is the line the thread records: no title, with the
	// facts. Facts is the same facts as the envelope says them.
	Said  string `json:"said"`
	Note  string `json:"note"`
	Facts string `json:"facts"`
}

// sentence is what happened to the PR; withTitle adds its title.
func (c prChange) sentence(withTitle bool) string {
	name := fmt.Sprintf("PR #%d", c.Link.Number)
	if withTitle && strings.TrimSpace(c.Link.Title) != "" {
		name += fmt.Sprintf(" %q", clip(c.Link.Title, 120))
	}
	switch c.Kind {
	case EventPRMerged:
		if c.Link.Base != "" {
			return name + " was merged into " + c.Link.Base + "."
		}
		return name + " was merged."
	}
	return name + " was closed without merging."
}

// since is the facts as the thread's one line says them.
func (f prFacts) since(kind string) string {
	var parts []string
	if f.CommitsKnown {
		switch {
		case kind == EventPRMerged && f.Commits == 1:
			parts = append(parts, "1 new commit")
		case kind == EventPRMerged:
			parts = append(parts, fmt.Sprintf("%d new commits", f.Commits))
		case f.Commits == 1:
			parts = append(parts, "1 unpushed commit")
		default:
			parts = append(parts, fmt.Sprintf("%d unpushed commits", f.Commits))
		}
	}
	for _, o := range f.Others {
		s := fmt.Sprintf("PR #%d still open", o.Number)
		if o.Base != "" {
			s += " (base " + o.Base + ")"
		}
		parts = append(parts, s)
	}
	if f.Uncommitted {
		parts = append(parts, "uncommitted changes")
	} else {
		parts = append(parts, "no uncommitted changes")
	}
	if f.Running {
		parts = append(parts, "a turn is running")
	}
	return "Since then: " + strings.Join(parts, " · ") + "."
}

// state is the facts as the coordinator's envelope says them.
func (f prFacts) state(kind string) string {
	s := "status=" + f.Status
	if f.CommitsKnown {
		key := "unpushed_commits"
		if kind == EventPRMerged {
			key = "commits_after_merge"
		}
		s += fmt.Sprintf(" %s=%d", key, f.Commits)
	}
	var open []string
	for _, o := range f.Others {
		s := fmt.Sprintf("#%d", o.Number)
		if o.Base != "" {
			s += "(base=" + o.Base + ")"
		}
		open = append(open, s)
	}
	prs := "none"
	if len(open) > 0 {
		prs = strings.Join(open, ",")
	}
	return s + " open_prs=" + prs + " uncommitted=" + yesNo(f.Uncommitted) + " running=" + yesNo(f.Running)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// onPRChanges records each change in its thread and tells the coordinator: one
// input per project for all of its changes. A thread already resolved or
// archived only records (the user has wrapped it up); a project that is
// paused or archived has no coordinator to wake, which onReport handles.
func (c *Coordinator) onPRChanges(changes []prChange) {
	byProject := map[string][]prEvent{}
	first := map[string]db.Thread{}
	var order []string
	for _, ch := range changes {
		t, err := c.store.GetThread(ch.ThreadID)
		if err != nil {
			continue
		}
		running := c.threads != nil && c.threads.Running(t.ID)
		f := factsOf(t, ch.Link, running)
		ev := prEvent{
			ThreadID: t.ID, Seq: t.Seq, Title: t.Title, Kind: ch.Kind,
			Item:  fmt.Sprintf("%s#%d", ch.Link.Repo, ch.Link.Number),
			Said:  ch.sentence(true),
			Note:  ch.sentence(false) + " " + f.since(ch.Kind),
			Facts: f.state(ch.Kind),
		}
		c.noteInThread(t, ev, f)
		if t.Status == db.ThreadResolved || t.ResolvedAt != nil || t.ArchivedAt != nil {
			continue
		}
		if _, ok := byProject[t.ProjectID]; !ok {
			order = append(order, t.ProjectID)
			first[t.ProjectID] = *t
		}
		byProject[t.ProjectID] = append(byProject[t.ProjectID], ev)
	}
	for _, id := range order {
		evs := byProject[id]
		var lines []string
		for _, e := range evs {
			lines = append(lines, e.Note)
		}
		c.onReport(Report{Thread: first[id], Kind: ReportGitHub, Text: strings.Join(lines, "\n"), Events: evs})
	}
}

// noteInThread writes the change in the thread's own record as a system event.
// The payload keeps the status it saw, for the invariant that this never moves it.
func (c *Coordinator) noteInThread(t *db.Thread, ev prEvent, f prFacts) {
	e := &db.ThreadEvent{
		ThreadID: t.ID, Kind: db.EvSystem, Content: ev.Note,
		Payload: jsonString(map[string]any{"github": ev.Kind, "item": ev.Item, "status": f.Status, "resolved": t.ResolvedAt != nil}),
	}
	if err := c.store.AddThreadEvent(e); err != nil {
		logrus.WithError(err).Warn("save thread event")
		return
	}
	c.publishEvent(t.ProjectID, e)
}

// renderGitHub is a report of GitHub events as the coordinator is shown it:
// one envelope per event, marked relay (the title is GitHub's text).
func renderGitHub(events []prEvent, at string) []string {
	var out []string
	for _, e := range events {
		out = append(out, envelope(trustRelay, e.Said+"\nThread now: "+e.Facts,
			"source", "github", "kind", e.Kind, "thread", e.Title, "id", short(e.ThreadID), "item", e.Item, "at", at))
	}
	return out
}
