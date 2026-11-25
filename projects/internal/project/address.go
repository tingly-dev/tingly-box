package project

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Addressing a thread from an IM chat (.design/projects-im.md, "在 IM 里点名线程"):
// "#N text" sends one message to thread #N,
// /session N or /session <title> binds the chat to it (#0: the project
// conversation), /session lists them.

// ErrNoSuchThread is for a thread number the project does not have.
var ErrNoSuchThread = errors.New("no such thread in this project")

// A message addresses a thread only by "#N" at its very start followed by
// whitespace and then the message; "#N" anywhere else, or glued to other
// characters ("#3rd", "#3,"), is plain text.
var mentionRe = regexp.MustCompile(`^#(\d+)\s+(?s:(.+))$`)

// parseMention reads "#2 text": the thread number and the message. ok is
// false for anything else, "@2 text" and "@opus ..." included.
func parseMention(text string) (n int, rest string, ok bool) {
	m := mentionRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return 0, "", false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, "", false
	}
	return n, strings.TrimSpace(m[2]), true
}

// threadBySeq finds thread #n of a project.
func (s *Service) threadBySeq(projectID string, n int) (*db.Thread, error) {
	threads, err := s.Store.ListThreads(projectID)
	if err != nil {
		return nil, err
	}
	for i := range threads {
		if threads[i].Seq == n {
			return &threads[i], nil
		}
	}
	return nil, fmt.Errorf("%w: #%d (/session lists the sessions here)", ErrNoSuchThread, n)
}

// threadByRef finds the thread a command argument names: "2" or "#2" by
// number ("0" is a project's conversation), otherwise by title: exact, then
// prefix, then part of it, ignoring case, among the sessions that are not
// archived. Several matches are listed for the user to choose from.
func (s *Service) threadByRef(projectID, ref string) (*db.Thread, error) {
	ref = strings.TrimSpace(ref)
	num := strings.TrimPrefix(ref, "#")
	if n, err := strconv.Atoi(num); err == nil && n >= 0 {
		if n == 0 {
			t, err := s.Store.GetMainThread(projectID)
			if errors.Is(err, db.ErrNotFound) {
				return nil, fmt.Errorf("%w: #0 (a folder has no project conversation)", ErrNoSuchThread)
			}
			return t, err
		}
		return s.threadBySeq(projectID, n)
	}
	if ref == "" {
		return nil, fmt.Errorf("name a session by its number or title, like #2")
	}
	threads, err := s.Store.ListThreads(projectID)
	if err != nil {
		return nil, err
	}
	var open []db.Thread
	for _, t := range threads {
		if t.ArchivedAt == nil && t.Seq > 0 {
			open = append(open, t)
		}
	}
	got := matchThreadTitles(open, strings.TrimPrefix(ref, "#"))
	switch len(got) {
	case 0:
		return nil, fmt.Errorf("%w: %q (/session lists the sessions here)", ErrNoSuchThread, ref)
	case 1:
		return &got[0], nil
	}
	lines := []string{fmt.Sprintf("More than one session matches %q:", ref)}
	for _, t := range got {
		lines = append(lines, fmt.Sprintf("#%d %s", t.Seq, t.Title))
	}
	lines = append(lines, "Say which: /session N.")
	return nil, errors.New(strings.Join(lines, "\n"))
}

// matchThreadTitles narrows threads to the best kind of title match: exact,
// else prefix, else substring, ignoring case.
func matchThreadTitles(threads []db.Thread, q string) []db.Thread {
	q = strings.ToLower(strings.TrimSpace(q))
	for _, match := range []func(title string) bool{
		func(t string) bool { return t == q },
		func(t string) bool { return strings.HasPrefix(t, q) },
		func(t string) bool { return strings.Contains(t, q) },
	} {
		var got []db.Thread
		for _, t := range threads {
			if match(strings.ToLower(t.Title)) {
				got = append(got, t)
			}
		}
		if len(got) > 0 {
			return got
		}
	}
	return nil
}

// placeName is how a reply names what a chat talks to.
func placeName(t *db.Thread, p *db.Project) string {
	if t.Agent == db.AgentLLM {
		return "the project conversation of " + p.Name
	}
	return "[" + threadTag(t.Seq, t.Title) + "]"
}

// focus binds a chat to thread #N of the project it is in.
func (s *Service) focus(b *db.ChannelBinding, p *db.Project, ref string) (string, error) {
	t, err := s.threadByRef(p.ID, ref)
	if err != nil {
		return "", err
	}
	if t.Status == db.ThreadResolved {
		return fmt.Sprintf("[%s] is done; reopen it in Projects on the web to talk to it.", threadTag(t.Seq, t.Title)), nil
	}
	if err := s.Store.UpsertChannelBinding(&db.ChannelBinding{Platform: b.Platform, ChatID: b.ChatID, ThreadID: t.ID, TargetProjectID: p.ID, TargetThreadID: b.TargetThreadID}); err != nil {
		return "", err
	}
	reply := "This chat now talks to " + placeName(t, p) + "."
	if t.ParentID != "" {
		if parent, err := s.Store.GetThread(t.ParentID); err == nil {
			reply += fmt.Sprintf(" /session %d returns to %s.", parent.Seq, placeName(parent, p))
		}
	}
	return reply, nil
}

// threadListMax caps /session; what waits on the user comes first.
const threadListMax = 15

// threadList lists the open sessions of the chat's project or folder, the
// ones waiting on the user first (a project's conversation, #0, leads), and
// marks the one the chat is on.
func (s *Service) threadList(cur *db.Thread, p *db.Project) (string, error) {
	threads, err := s.Store.ListThreads(p.ID)
	if err != nil {
		return "", err
	}
	rank := map[string]int{db.ThreadWaiting: 0, db.ThreadFailed: 0, db.ThreadWorking: 1}
	var open []db.Thread
	done := 0
	for _, t := range threads {
		if t.ArchivedAt != nil {
			continue
		}
		if t.Status == db.ThreadResolved {
			done++
			continue
		}
		open = append(open, t)
	}
	sort.SliceStable(open, func(i, j int) bool {
		ri, iok := rank[open[i].Status]
		rj, jok := rank[open[j].Status]
		if !iok {
			ri = 2
		}
		if !jok {
			rj = 2
		}
		if ri != rj {
			return ri < rj
		}
		return open[i].Seq < open[j].Seq
	})
	var lines []string
	if p.Type != db.ProjectFolder {
		line := "#0 the project conversation"
		if cur.Agent == db.AgentLLM {
			line += " (this chat)"
		}
		lines = append(lines, line)
	}
	if len(open) == 0 {
		lines = append(lines, fmt.Sprintf("%s has no open sessions (%d done).", p.Name, done))
		return strings.Join(lines, "\n"), nil
	}
	for i, t := range open {
		if i == threadListMax {
			lines = append(lines, fmt.Sprintf("… and %d more on the web", len(open)-threadListMax))
			break
		}
		line := fmt.Sprintf("#%d %s: %s", t.Seq, t.Title, threadStatusWord(t.Status))
		if t.ID == cur.ID {
			line += " (this chat)"
		}
		lines = append(lines, line)
	}
	if done > 0 {
		lines = append(lines, fmt.Sprintf("(%d done)", done))
	}
	lines = append(lines, "/session N (or part of a title) switches to one; #N <message> talks to it once.")
	return strings.Join(lines, "\n"), nil
}

func threadStatusWord(status string) string {
	switch status {
	case db.ThreadWaiting:
		return "needs you"
	case db.ThreadFailed:
		return "failed"
	case db.ThreadWorking:
		return "working"
	case db.ThreadReady:
		return "ready to merge"
	default:
		return "idle"
	}
}
