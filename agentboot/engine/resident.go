package engine

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// resident is the engine's handle on one pooled Claude Code process. Its Conductor
// reads everything the process emits for as long as it lives: the turns the host
// starts, and what happens between them — background task progress, and the
// turn Claude Code starts on its own when a background task finishes after
// the turn that launched it. Reading only during the host's turns would leave
// that output unread and misattributed to the next message (.design/desk.md
// §3.7).
type resident struct {
	c *agentboot.Conductor
}

// newResident starts conducting a freshly opened process. Call it right
// after Open, before the first turn's events pile up.
func (e *Engine) newResident(sessionID string, ps agentboot.PersistentSession) *resident {
	r := &resident{}
	// One converter for the process's life: its dedupe state spans turns
	// and it resets its usage tally at each turn's result.
	conv := newConverter()
	r.c = agentboot.NewConductor(ps, agentboot.ConductorHooks{
		Sink: func(raw any) {
			e.record(sessionID, conv, raw)
			// Activity between turns (a background task reporting in)
			// counts as use: the pool must not evict a process that is
			// still working.
			e.pool.Touch(sessionID)
		},
		OnUnsolicitedTurn: func() (context.Context, agentboot.Prompter) {
			return e.beginUnsolicitedTurn(sessionID)
		},
		OnUnsolicitedTurnComplete: func(_ *agentboot.Result, err error) {
			e.endUnsolicitedTurn(sessionID, err)
		},
		OnTerminated: func(string) {
			e.mu.Lock()
			if e.residents[sessionID] == r {
				delete(e.residents, sessionID)
				// Its background tasks ended with it.
				delete(e.live, sessionID)
			}
			e.mu.Unlock()
		},
	})
	e.mu.Lock()
	e.residents[sessionID] = r
	e.mu.Unlock()
	return r
}

// record turns one agent message into transcript entries.
func (e *Engine) record(sessionID string, conv *converter, raw any) {
	if ev, ok := raw.(agentboot.ErrorEvent); ok {
		if ev.Err != nil {
			e.appendError(sessionID, ev.Err.Error())
		}
		return
	}
	for _, m := range conv.messages(raw) {
		e.store.Append(sessionID, m)
		e.noteTask(sessionID, m)
	}
}

// beginUnsolicitedTurn registers the turn Claude Code started by itself as
// the session's run, so the page shows it working and its approvals, Stop
// and the "busy" check all behave as for any turn.
func (e *Engine) beginUnsolicitedTurn(sessionID string) (context.Context, agentboot.Prompter) {
	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	wp := newWebPrompter(sessionID, e.store)

	e.mu.Lock()
	if existing, ok := e.runs[sessionID]; ok {
		// A message from the host claimed the session in the same instant;
		// its Send will be refused (the agent's turn came first), so this
		// turn answers approvals through that run's prompter.
		e.mu.Unlock()
		cancel()
		return context.Background(), existing.prompter
	}
	e.runs[sessionID] = &run{cancel: cancel, prompter: wp, done: make(chan struct{}), unsolicited: true}
	e.mu.Unlock()

	e.appendSystem(sessionID, "a background task finished; Claude is following up")
	e.store.SetRunning(sessionID)

	var prompter agentboot.Prompter = wp
	if e.permissionMode != nil && autoApproves(e.permissionMode(sessionID)) {
		prompter = autoApprovePrompter{inner: wp}
	}
	return ctx, prompter
}

func (e *Engine) endUnsolicitedTurn(sessionID string, err error) {
	e.mu.Lock()
	r, ok := e.runs[sessionID]
	if ok && r.unsolicited {
		delete(e.runs, sessionID)
	} else {
		// Adopted by a message's run in the same instant (see
		// beginUnsolicitedTurn): that run cleans itself up, but the status
		// is still this turn's to settle.
		r = nil
	}
	e.mu.Unlock()
	interrupted := false
	if r != nil {
		interrupted = r.stopped.Load()
		r.cancel()
		close(r.done)
	}
	e.pool.Touch(sessionID)

	switch {
	case interrupted:
		e.store.SetInterrupted(sessionID)
		e.appendSystem(sessionID, "interrupted; send a message to resume")
	case err != nil:
		e.store.SetFailed(sessionID, err.Error())
		e.appendError(sessionID, err.Error())
	default:
		e.store.SetCompleted(sessionID, "")
	}
}

// residentFor returns the Conductor of id's pooled process, if it has one.
func (e *Engine) residentFor(id string) (*resident, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.residents[id]
	return r, ok
}
