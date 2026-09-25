package desk

import (
	"context"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// resident is Desk's handle on one pooled Claude Code process. Its Conductor
// reads everything the process emits for as long as it lives: the turns Desk
// starts, and what happens between them — background task progress, and the
// turn Claude Code starts on its own when a background task finishes after
// the turn that launched it. Reading only during Desk's turns would leave
// that output unread and misattributed to the next message (.design/desk.md
// §3.7).
type resident struct {
	c *agentboot.Conductor
}

// newResident starts conducting a freshly opened process. Call it right
// after Open, before the first turn's events pile up.
func (s *Service) newResident(sessionID string, ps agentboot.PersistentSession) *resident {
	r := &resident{}
	// One converter for the process's life: its dedupe state spans turns
	// and it resets its usage tally at each turn's result.
	conv := newConverter()
	r.c = agentboot.NewConductor(ps, agentboot.ConductorHooks{
		Sink: func(raw any) {
			s.record(sessionID, conv, raw)
			// Activity between turns (a background task reporting in)
			// counts as use: the pool must not evict a process that is
			// still working.
			s.pool.Touch(sessionID)
		},
		OnUnsolicitedTurn: func() (context.Context, agentboot.Prompter) {
			return s.beginUnsolicitedTurn(sessionID)
		},
		OnUnsolicitedTurnComplete: func(_ *agentboot.Result, err error) {
			s.endUnsolicitedTurn(sessionID, err)
		},
		OnTerminated: func(string) {
			s.mu.Lock()
			if s.residents[sessionID] == r {
				delete(s.residents, sessionID)
				// Its background tasks ended with it.
				delete(s.live, sessionID)
			}
			s.mu.Unlock()
		},
	})
	s.mu.Lock()
	s.residents[sessionID] = r
	s.mu.Unlock()
	return r
}

// record turns one agent message into transcript entries.
func (s *Service) record(sessionID string, conv *converter, raw any) {
	if ev, ok := raw.(agentboot.ErrorEvent); ok {
		if ev.Err != nil {
			s.sessions.AppendMessage(sessionID, session.Message{Kind: "error", Content: ev.Err.Error(), Timestamp: time.Now()})
		}
		return
	}
	for _, m := range conv.messages(raw) {
		s.sessions.AppendMessage(sessionID, m)
		s.noteUsage(sessionID, m)
		s.noteTask(sessionID, m)
	}
}

// beginUnsolicitedTurn registers the turn Claude Code started by itself as
// the session's run, so the page shows it working and its approvals, Stop
// and the "busy" check all behave as for any turn.
func (s *Service) beginUnsolicitedTurn(sessionID string) (context.Context, agentboot.Prompter) {
	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	wp := newWebPrompter(sessionID, s.sessions)

	s.mu.Lock()
	if existing, ok := s.runs[sessionID]; ok {
		// A message from the page claimed the session in the same instant;
		// its Send will be refused (the agent's turn came first), so this
		// turn answers approvals through that run's prompter.
		s.mu.Unlock()
		cancel()
		return context.Background(), existing.prompter
	}
	s.runs[sessionID] = &run{cancel: cancel, prompter: wp, done: make(chan struct{}), unsolicited: true}
	s.mu.Unlock()

	s.appendSystem(sessionID, "a background task finished; Claude is following up")
	s.sessions.SetRunning(sessionID)

	var prompter agentboot.Prompter = wp
	if snap, ok := s.sessions.Snapshot(sessionID); ok && autoApproves(snap.PermissionMode) {
		prompter = autoApprovePrompter{inner: wp}
	}
	return ctx, prompter
}

func (s *Service) endUnsolicitedTurn(sessionID string, err error) {
	s.mu.Lock()
	r, ok := s.runs[sessionID]
	if ok && r.unsolicited {
		delete(s.runs, sessionID)
	} else {
		// Adopted by a message's run in the same instant (see
		// beginUnsolicitedTurn): that run cleans itself up, but the status
		// is still this turn's to settle.
		r = nil
	}
	s.mu.Unlock()
	interrupted := false
	if r != nil {
		interrupted = r.stopped.Load()
		r.cancel()
		close(r.done)
	}
	s.pool.Touch(sessionID)

	switch {
	case interrupted:
		s.sessions.Update(sessionID, func(sess *session.Session) {
			sess.Status, sess.Error = session.StatusCompleted, ""
		})
		s.appendSystem(sessionID, "interrupted; send a message to resume")
	case err != nil:
		s.sessions.SetFailed(sessionID, err.Error())
		s.sessions.AppendMessage(sessionID, session.Message{Kind: "error", Content: err.Error(), Timestamp: time.Now()})
	default:
		s.sessions.SetCompleted(sessionID, "")
	}
}

// residentFor returns the Conductor of id's pooled process, if it has one.
func (s *Service) residentFor(id string) (*resident, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.residents[id]
	return r, ok
}
