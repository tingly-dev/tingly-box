package project

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// NewSession rotates the project coordinator's Claude Code session in place,
// as Threads.NewSession does for a worker: the conversation, its events and
// the project stay, the next input starts a session with no resume and gets
// the project context again. Like a worker it refuses, rather than waits for
// or discards, a turn that is running or queued and an input that needs
// recovery.
func (c *Coordinator) NewSession(projectID string, src Source) (*db.Thread, error) {
	if !c.live() {
		return nil, errNoSessions
	}
	p, err := c.store.GetProject(projectID)
	if err != nil {
		return nil, err
	}
	if p.ArchivedAt != nil {
		return nil, ErrArchived
	}
	if p.Paused {
		return nil, ErrPaused
	}
	main, err := c.store.GetThread(p.MainThreadID)
	if err != nil {
		return nil, err
	}
	// Holding c.mu keeps a message posted meanwhile from waking the queue
	// until the new session is in place; its turn then starts in it.
	c.mu.Lock()
	defer c.mu.Unlock()
	if q, ok := c.queues[projectID]; ok && (q.running || q.woken) {
		return nil, fmt.Errorf("cannot start a new session while the coordinator is working; wait for it or stop it first: %w", ErrBusy)
	}
	if pending, err := c.store.PendingCoordinatorInputs(projectID); err != nil {
		return nil, err
	} else if pending != 0 {
		return nil, fmt.Errorf("cannot start a new session while messages are waiting for the coordinator: %w", ErrBusy)
	}
	if id, err := c.store.UncertainCoordinatorInput(projectID); err != nil {
		return nil, err
	} else if id != "" {
		return nil, fmt.Errorf("cannot start a new session while an interrupted message needs recovery; retry or skip it first: %w", ErrBusy)
	}
	old := main.SessionID
	if old != "" {
		if err := c.sessions.engine.Release(old); err != nil {
			if errors.Is(err, engine.ErrBusy) {
				return nil, fmt.Errorf("cannot start a new session while work is running; finish or stop it first: %w", ErrBusy)
			}
			return nil, fmt.Errorf("could not close the current session: %w", err)
		}
	}
	event := &db.ThreadEvent{}
	src.stamp(event)
	if err := c.store.RotateThreadSession(main.ID, old, uuid.NewString(), event); err != nil {
		return nil, err
	}
	c.sessions.mu.Lock()
	delete(c.sessions.stopReq, old)
	c.sessions.mu.Unlock()
	fresh, err := c.store.GetThread(main.ID)
	if err != nil {
		return nil, err
	}
	c.publishEvent(projectID, event)
	c.threads.publish(fresh)
	return fresh, nil
}
