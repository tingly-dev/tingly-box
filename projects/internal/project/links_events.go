package project

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

// Kinds of a GitHub event, as the envelope's kind attribute says them.
// EventPRConflicts is the one that needs the user (needs_user="true");
// EventPRConflictsGone is only ever recorded in the thread, never sent.
const (
	EventPRMerged        = "pr_merged"
	EventPRClosed        = "pr_closed"
	EventPRConflicts     = "pr_conflicts"
	EventPRConflictsGone = "pr_conflicts_gone"
)

// conflictWakeGap is how soon after waking the coordinator about a PR's
// conflicts it may be woken about the same PR again, when it left them and
// came back; the count is kept in memory, so a restart forgets it. A conflict
// that stays is one wake whatever this says: the record keeps its merge state
// through unknown readings (keepMergeState).
const conflictWakeGap = 30 * time.Minute

// prChange is one change of a PR the thread opened: Link is the record as it
// is now. Quiet is set for a change that is only recorded in the thread.
type prChange struct {
	ThreadID string
	Kind     string
	Link     db.ThreadLink
	Quiet    bool
}

// conflicting is a merge state of an open PR that cannot be merged as it is.
func conflicting(k *db.ThreadLink) bool {
	return k.State == db.LinkStateOpen && k.MergeState == "conflicts"
}

// prTransition is the kind of event a record's move from before to after is,
// or "" when it is none. Only an open PR the thread opened counts (a new
// record starts open, so its first reading counts too), and only a move out
// of open, into conflicts from any other merge state, or out of them to a
// state GitHub has settled on: a finished record read again, or a conflict
// that is still there, is no change.
func prTransition(before db.ThreadLink, after *db.ThreadLink) string {
	if after.Kind != db.LinkPR || after.Role != db.LinkOpened || before.State != db.LinkStateOpen {
		return ""
	}
	switch after.State {
	case db.LinkStateMerged:
		return EventPRMerged
	case db.LinkStateClosed:
		return EventPRClosed
	}
	switch {
	case conflicting(after) && !conflicting(&before):
		return EventPRConflicts
	case conflicting(&before) && after.State == db.LinkStateOpen && after.MergeState != "" && after.MergeState != "unknown" && after.MergeState != "conflicts":
		return EventPRConflictsGone
	}
	return ""
}

// changeOf is the change a record's move from before to the state it now has
// is, or nil. A change of conflicts is recorded only (Quiet) when it is the
// leaving of them.
func changeOf(threadID string, before db.ThreadLink, after *db.ThreadLink) *prChange {
	kind := prTransition(before, after)
	if kind == "" {
		return nil
	}
	return &prChange{ThreadID: threadID, Kind: kind, Link: *after, Quiet: kind == EventPRConflictsGone}
}

// capConflicts makes a change of entering conflicts quiet when the coordinator
// was already woken about the same PR within conflictWakeGap. The caller holds
// l.transitions.
func (l *Links) capConflicts(ch *prChange) {
	if ch == nil || ch.Kind != EventPRConflicts {
		return
	}
	key := fmt.Sprintf("%s|%s#%d", ch.ThreadID, ch.Link.Repo, ch.Link.Number)
	now := l.now()
	if last, ok := l.conflictWoke[key]; ok && now.Sub(last) < conflictWakeGap {
		ch.Quiet = true
		return
	}
	if l.conflictWoke == nil {
		l.conflictWoke = map[string]time.Time{}
	}
	l.conflictWoke[key] = now
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
	// Line is the sentence alone, for the chat's fallback line.
	Line string `json:"line"`
	// NeedsUser is set for conflicts: the user decides, so the coordinator
	// must ask.
	NeedsUser bool `json:"needs_user,omitempty"`
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
	case EventPRConflicts:
		if c.Link.Base != "" {
			return name + " has merge conflicts with " + c.Link.Base + "."
		}
		return name + " has merge conflicts."
	case EventPRConflictsGone:
		return fmt.Sprintf("Conflicts on PR #%d are resolved.", c.Link.Number)
	}
	return name + " was closed without merging."
}

// now is the facts as the thread's one line says them for a PR that needs the
// user's choice: where the thread stands, not what happened since.
func (f prFacts) now() string {
	parts := []string{f.Status}
	if f.Running {
		parts = append(parts, "a turn is running")
	} else {
		parts = append(parts, "no turn running")
	}
	if f.CommitsKnown {
		parts = append(parts, fmt.Sprintf("%d unpushed %s", f.Commits, plural(f.Commits, "commit")))
	}
	if f.Uncommitted {
		parts = append(parts, "uncommitted changes")
	} else {
		parts = append(parts, "no uncommitted changes")
	}
	return "Thread: " + strings.Join(parts, ", ") + "."
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
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
			Line:  ch.sentence(false),
			Facts: f.state(ch.Kind),
			Note:  ch.sentence(false) + " " + f.since(ch.Kind),

			NeedsUser: ch.Kind == EventPRConflicts,
		}
		switch ch.Kind {
		case EventPRConflicts:
			ev.Note = ev.Line + " " + f.now()
		case EventPRConflictsGone:
			ev.Note = ev.Line
		}
		c.noteInThread(t, ev, f)
		if ch.Quiet || t.Status == db.ThreadResolved || t.ResolvedAt != nil || t.ArchivedAt != nil {
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
		needs := ""
		if e.NeedsUser {
			needs = "true"
		}
		out = append(out, envelope(trustRelay, e.Said+"\nThread now: "+e.Facts,
			"source", "github", "kind", e.Kind, "needs_user", needs, "thread", e.Title, "id", short(e.ThreadID), "item", e.Item, "at", at))
	}
	return out
}

// needsUserOf are the events of a github report that the user decides.
func needsUserOf(e *db.ThreadEvent) []prEvent {
	var pl struct {
		Kind   string    `json:"kind"`
		Events []prEvent `json:"events"`
	}
	if json.Unmarshal([]byte(e.Payload), &pl) != nil || pl.Kind != ReportGitHub {
		return nil
	}
	var out []prEvent
	for _, ev := range pl.Events {
		if ev.NeedsUser {
			out = append(out, ev)
		}
	}
	return out
}

// foldNeedsUser tells the chats, as one folded line each, about events that
// need the user when the coordinator cannot be counted on to ask: it is
// offline, or answered with nothing.
func (c *Coordinator) foldNeedsUser(projectID string, events []prEvent) {
	p, err := c.store.GetProject(projectID)
	if err != nil {
		return
	}
	for _, ev := range events {
		if !ev.NeedsUser {
			continue
		}
		c.final(&db.ThreadEvent{
			ThreadID: p.MainThreadID, Kind: db.EvThreadUpdate, Content: ev.Line,
			Payload: jsonString(map[string]any{"thread_id": ev.ThreadID, "seq": ev.Seq, "title": ev.Title, "kind": ReportGitHub, "events": []prEvent{ev}}),
		})
	}
}

// answeredNeedsUser reports whether a coordinator turn on a needs_user input
// said something to the user: a reply (text, when the turn was not ended with
// no_update) or a decision card posted since the input.
func (c *Coordinator) answeredNeedsUser(mainThreadID string, input *db.ThreadEvent, replied bool) bool {
	if replied {
		return true
	}
	recent, err := c.store.ListRecentThreadEvents(mainThreadID, 50)
	if err != nil {
		return true // cannot tell: do not say it twice
	}
	for _, e := range recent {
		if e.Kind == db.EvDecision && !e.CreatedAt.Before(input.CreatedAt) {
			return true
		}
	}
	return false
}
