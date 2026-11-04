package project

import (
	"time"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A worker whose turn ends while its process still runs background tasks (a
// reviewer subagent, a test run) has not finished: Claude starts another turn
// when they end, and that turn's text is the result. Its report waits for
// that turn; the thread card shows it as working in between.

// Defaults for Threads.HoldGrace and Threads.HoldMax.
const (
	DefaultHoldGrace = 15 * time.Second
	DefaultHoldMax   = 30 * time.Minute
)

func (m *Threads) hasBackground(threadID string) bool {
	if m.backgroundCount != nil {
		return m.backgroundCount(threadID) > 0
	}
	return len(m.engine.BackgroundTasks(m.sessionOf(threadID))) > 0
}

func (m *Threads) holdGrace() time.Duration {
	if m.HoldGrace > 0 {
		return m.HoldGrace
	}
	return DefaultHoldGrace
}

func (m *Threads) holdMax() time.Duration {
	if m.HoldMax > 0 {
		return m.HoldMax
	}
	return DefaultHoldMax
}

// hold keeps a worker's finished report until its background tasks end. The
// follow-up turn replaces it (settle drops it); otherwise it is released a
// moment after the last task ends, or after HoldMax if they never do.
func (m *Threads) hold(r Report) {
	m.mu.Lock()
	m.held[r.Thread.ID] = r
	m.mu.Unlock()
	m.releaseSoon(r.Thread.ID, m.holdMax())
}

// releaseSoon releases a held report after d, if nothing replaced it by then.
func (m *Threads) releaseSoon(threadID string, d time.Duration) {
	m.mu.Lock()
	_, ok := m.held[threadID]
	m.mu.Unlock()
	if ok {
		time.AfterFunc(d, func() { m.release(threadID) })
	}
}

// release sends a held report as the turn's report, unless a turn is running
// (its end settles the thread and drops the held one) or it is already gone.
func (m *Threads) release(threadID string) {
	m.mu.Lock()
	r, ok := m.held[threadID]
	_, running := m.turns[threadID]
	if !ok || running {
		m.mu.Unlock()
		return
	}
	delete(m.held, threadID)
	m.mu.Unlock()
	t, err := m.update(threadID, func(t *db.Thread) {
		if t.Status == db.ThreadWorking {
			t.Status = db.ThreadIdle
			if t.CommitsAhead > 0 {
				t.Status = db.ThreadReady
			}
		}
	})
	if err != nil || t == nil || t.Status == db.ThreadResolved || t.ArchivedAt != nil {
		return
	}
	r.Thread = *t
	if m.OnReply != nil {
		m.OnReply(r)
	}
	_ = m.store.TouchProject(t.ProjectID)
	if m.OnReport != nil {
		m.OnReport(r)
	}
}
