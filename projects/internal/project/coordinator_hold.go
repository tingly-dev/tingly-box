package project

import "github.com/tingly-dev/tingly-box/projects/internal/db"

// coordinatorNeeded reports whether the coordinator process of session sid
// must stay open although it sits idle: a thread of its project is at work,
// so a report is on its way and the coordinator will be asked to act on it.
// Closing it then would make that report resume the session cold in the
// middle of the very work it directs. The pool asks this before it closes a
// coordinator for being idle or to make room (pool.Config.Hold).
//
// "At work" is a thread marked working (a turn started or queued), a thread
// with a turn in flight (one waiting on an approval is still in flight) or a
// thread whose process runs background tasks. A thread that merely waits for
// the user with nothing in flight (interrupted by a restart, say) does not
// pin the coordinator: nothing will come of it without the user, and the
// coordinator then closes after its own idle timeout like any idle process.
func (s *Service) coordinatorNeeded(sid string) bool {
	main, err := s.Store.MainThreadBySession(sid)
	if err != nil {
		return false
	}
	threads, err := s.Store.ListThreads(main.ProjectID)
	if err != nil {
		// Unknown: keep the process, the next sweep asks again.
		return true
	}
	for i := range threads {
		if s.Threads.atWork(&threads[i]) {
			return true
		}
	}
	return false
}

// atWork reports whether t is working in the sense coordinatorNeeded means.
func (m *Threads) atWork(t *db.Thread) bool {
	if t.Status == db.ThreadWorking || m.Running(t.ID) {
		return true
	}
	sid := m.sessionOf(t.ID)
	return sid != "" && len(m.engine.BackgroundTasks(sid)) > 0
}
