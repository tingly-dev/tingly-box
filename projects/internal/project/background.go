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
	DefaultHoldMax   = 10 * time.Minute
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

// heldReport is a worker's finished report waiting for its background tasks,
// with the timer that releases it.
type heldReport struct {
	report Report
	timer  *time.Timer
}

// hold keeps a worker's finished report until its background tasks end. The
// follow-up turn replaces it (settle drops it); otherwise it is released a
// moment after the last task ends, or after HoldMax if they never do.
func (m *Threads) hold(r Report) {
	m.mu.Lock()
	m.held[r.Thread.ID] = &heldReport{report: r}
	m.mu.Unlock()
	d := m.holdMax()
	if !m.hasBackground(r.Thread.ID) {
		// The tasks ended after settle looked and before the report was held;
		// their end event found nothing to release.
		d = m.holdGrace()
	}
	m.releaseSoon(r.Thread.ID, d)
}

// dropHeldReport forgets a thread's held report and its release timer.
func (m *Threads) dropHeldReport(threadID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h := m.held[threadID]; h != nil && h.timer != nil {
		h.timer.Stop()
	}
	delete(m.held, threadID)
}

// releaseSoon releases a held report after d, if nothing replaced it by then.
// A held report has one timer: a later call replaces the earlier one, and a
// timer of an older report is stopped with it, so it never releases a newer one.
func (m *Threads) releaseSoon(threadID string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.held[threadID]
	if h == nil {
		return
	}
	if h.timer != nil {
		h.timer.Stop()
	}
	h.timer = time.AfterFunc(d, func() { m.release(threadID) })
}

// release sends a held report as the turn's report, unless a turn is running
// (its end settles the thread and drops the held one) or it is already gone.
func (m *Threads) release(threadID string) {
	m.mu.Lock()
	h := m.held[threadID]
	_, running := m.turns[threadID]
	if h == nil || running {
		m.mu.Unlock()
		return
	}
	delete(m.held, threadID)
	m.mu.Unlock()
	t, err := m.update(threadID, func(t *db.Thread) {
		if t.Status == db.ThreadWorking {
			t.Status = restingStatus(t.CommitsAhead)
		}
	})
	if err != nil || t == nil || t.Status == db.ThreadResolved || t.ArchivedAt != nil {
		return
	}
	r := h.report
	r.Thread = *t
	if m.OnReply != nil {
		m.OnReply(r)
	}
	_ = m.store.TouchProject(t.ProjectID)
	if m.OnReport != nil {
		m.OnReport(r)
	}
}

// restingStatus is where a thread rests once its turn and background tasks
// are over: ready for review if its branch has commits, idle otherwise.
func restingStatus(commitsAhead int) string {
	if commitsAhead > 0 {
		return db.ThreadReady
	}
	return db.ThreadIdle
}
