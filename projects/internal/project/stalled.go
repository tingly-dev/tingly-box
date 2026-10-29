package project

import (
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// ReportStalled is a report kind for a thread whose turn is still running but
// has shown no activity for a long while.
const ReportStalled = "stalled"

// DefaultStallAfter is how long a running thread may be silent before the
// coordinator hears about it.
const DefaultStallAfter = 20 * time.Minute

// CheckStalled reports each running thread that has shown no activity for
// StallAfter, once per quiet spell: the flag clears when activity resumes.
// Threads waiting on the user are not stalled, they are waiting. The report
// is information for the coordinator, which decides whether the user needs
// to hear it; nothing is stopped or restarted.
func (m *Threads) CheckStalled(now time.Time) {
	after := m.StallAfter
	if after <= 0 {
		after = DefaultStallAfter
	}
	threads, err := m.store.ListThreadsByStatus(db.ThreadWorking)
	if err != nil {
		return
	}
	live := map[string]bool{}
	for _, t := range threads {
		if !m.Running(t.ID) {
			continue
		}
		live[t.ID] = true
		last := t.UpdatedAt
		if events, _, err := m.store.PageThreadEvents(t.ID, db.EventPage{Limit: 1}); err == nil && len(events) > 0 && events[0].CreatedAt.After(last) {
			last = events[0].CreatedAt
		}
		quiet := now.Sub(last)
		m.mu.Lock()
		flagged := m.stalled[t.ID]
		switch {
		case quiet < after:
			delete(m.stalled, t.ID)
		case !flagged:
			if m.stalled == nil {
				m.stalled = map[string]bool{}
			}
			m.stalled[t.ID] = true
		}
		m.mu.Unlock()
		if quiet < after || flagged || m.OnReport == nil {
			continue
		}
		t := t
		m.OnReport(Report{Thread: t, Kind: ReportStalled,
			Text: fmt.Sprintf("No activity for %d minutes; the turn is still running.", int(quiet.Minutes()))})
	}
	m.mu.Lock()
	for id := range m.stalled {
		if !live[id] {
			delete(m.stalled, id)
		}
	}
	m.mu.Unlock()
}
