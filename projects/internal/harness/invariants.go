package harness

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Invariants hold at the end of every scenario, whatever it did. Each is a
// rule of the Projects workflow, not of one test.
//
//	trust.report-labelled  a thread's report reaches the coordinator in an
//	                       envelope marked trust="relay", never as the user's words
//	trust.envelope-intact  text inside an event cannot open or close envelopes
//	wait.has-reason        a thread waiting on the user says what for
//	status.ready-has-commits  a thread is only "ready for review" when its
//	                       branch has commits to review
//	status.resolved-consistent  a thread is resolved exactly when it has a
//	                       resolved time, so reopening cannot leave one behind
//	memory.index-resolves  every link in MEMORY.md names a file that exists
func (w *World) CheckInvariants() {
	t := w.T
	t.Helper()

	open := regexp.MustCompile(`<event[^>]*>`)
	for _, req := range w.Requests() {
		events := withoutContext(req.Prompt)
		opens := open.FindAllString(events, -1)
		if len(opens) != strings.Count(events, "</event>") {
			t.Errorf("trust.envelope-intact: an event envelope was opened or closed by text inside one:\n%s", events)
		}
		for _, o := range opens {
			if strings.Contains(o, `source="thread"`) && !strings.Contains(o, `trust="relay"`) {
				t.Errorf("trust.report-labelled: a thread's report is not marked as relayed: %s", o)
			}
		}
	}

	for _, th := range w.ThreadList() {
		if th.Status == db.ThreadWaiting && strings.TrimSpace(th.WaitReason) == "" {
			t.Errorf("wait.has-reason: thread %q is waiting with no reason", th.Title)
		}
	}

	for _, th := range w.ThreadList() {
		if th.Status == db.ThreadReady && th.Branch != "" && th.CommitsAhead == 0 {
			t.Errorf("status.ready-has-commits: thread %q is ready with no commits", th.Title)
		}
	}

	for _, th := range w.ThreadList() {
		if (th.Status == db.ThreadResolved) != (th.ResolvedAt != nil) {
			t.Errorf("status.resolved-consistent: thread %q is %s with resolved time %v", th.Title, th.Status, th.ResolvedAt)
		}
	}

	idx := w.Memory("MEMORY.md")
	for _, m := range regexp.MustCompile(`\]\(([^)]+\.md)\)|\[\[([^\]]+\.md)\]\]`).FindAllStringSubmatch(idx, -1) {
		name := m[1]
		if name == "" {
			name = m[2]
		}
		if _, err := os.Stat(filepath.Join(w.Layout.MemoryDir(w.Project.ID), name)); err != nil {
			t.Errorf("memory.index-resolves: MEMORY.md links %s, which is missing", name)
		}
	}
}

// withoutContext drops the project context a turn's prompt may carry (the
// coordinator's rules talk about <event> envelopes in prose), leaving the
// conversation's events.
func withoutContext(prompt string) string {
	start := strings.Index(prompt, "\nCurrent project context:\n")
	if start < 0 {
		return prompt
	}
	end := strings.Index(prompt[start:], "</threads>\n")
	if end < 0 {
		return prompt[:start]
	}
	return prompt[:start] + prompt[start+end+len("</threads>\n"):]
}
