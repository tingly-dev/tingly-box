package project

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// NewSession rotates the agent session in place, without stopping active work
// or removing the thread's history and workspace.
func (m *Threads) NewSession(threadID string, src Source) (*db.Thread, error) {
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	t, err := m.sendable(threadID)
	if err != nil {
		return nil, err
	}
	prompter := m.prompter(threadID)
	prompter.mu.Lock()
	pending := len(prompter.pending) != 0
	prompter.mu.Unlock()
	m.mu.Lock()
	heldReport := m.held[threadID] != nil
	m.mu.Unlock()
	if pending || heldReport || m.Running(threadID) || len(m.engine.BackgroundTasks(t.SessionID)) != 0 {
		return nil, fmt.Errorf("cannot start a new session while work or a request is pending; finish or stop it first: %w", ErrBusy)
	}
	if err := m.engine.Release(t.SessionID); err != nil {
		if err == engine.ErrBusy {
			return nil, fmt.Errorf("cannot start a new session while work is running; finish or stop it first: %w", ErrBusy)
		}
		return nil, fmt.Errorf("could not close the current session: %w", err)
	}
	row := m.rowLock(threadID)
	row.Lock()
	defer row.Unlock()
	fresh, err := m.sendable(threadID)
	if err != nil {
		return nil, err
	}
	if fresh.SessionID != t.SessionID {
		return nil, fmt.Errorf("session changed while starting a new session")
	}
	event := &db.ThreadEvent{}
	src.stamp(event)
	newID := uuid.NewString()
	if err := m.store.RotateThreadSession(threadID, t.SessionID, newID, event); err != nil {
		return nil, err
	}
	m.mu.Lock()
	delete(m.threadOfSession, t.SessionID)
	m.sessionOfThread[threadID], m.threadOfSession[newID] = newID, threadID
	delete(m.allowed, threadID)
	m.mu.Unlock()
	fresh, err = m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: event})
	m.publish(fresh)
	return fresh, nil
}
