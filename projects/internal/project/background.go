package project

import (
	"fmt"
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

// RunningTasks is how many background tasks the thread's process runs now.
func (m *Threads) RunningTasks(threadID string) int {
	if m.backgroundCount != nil {
		return m.backgroundCount(threadID)
	}
	return len(m.engine.BackgroundTasks(m.sessionOf(threadID)))
}

func (m *Threads) hasBackground(threadID string) bool { return m.RunningTasks(threadID) > 0 }

// republish tells open pages about a thread whose stored row did not change
// (its running-task count is read live), without touching updated_at.
func (m *Threads) republish(threadID string) {
	l := m.rowLock(threadID)
	l.Lock()
	defer l.Unlock()
	if t, err := m.store.GetThread(threadID); err == nil {
		m.publish(t)
	}
}

// AnnotateOne fills in a thread's RunningTasks from the live process and its
// Links from the store. Everything that sends threads out (the API, events, the
// coordinator's view) goes through it or Annotate, so a count means the same
// everywhere.
func (m *Threads) AnnotateOne(t *db.Thread) {
	t.RunningTasks = m.RunningTasks(t.ID)
	t.Links, _ = m.store.ThreadLinks(t.ID)
}

// Annotate is AnnotateOne for a list.
func (m *Threads) Annotate(ts []db.Thread) {
	if len(ts) == 0 {
		return
	}
	ids := make([]string, len(ts))
	for i := range ts {
		ids[i] = ts[i].ID
	}
	links, _ := m.store.ThreadLinksOf(ids)
	for i := range ts {
		ts[i].Links = links[ts[i].ID]
		if m.backgroundCount != nil {
			ts[i].RunningTasks = m.backgroundCount(ts[i].ID)
		} else {
			ts[i].RunningTasks = len(m.engine.BackgroundTasks(ts[i].SessionID))
		}
	}
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
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
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

// tasksNote is the coordinator's marker for a thread that runs background tasks.
func tasksNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" background_tasks=%d", n)
}
