package project

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// NewSession starts a fresh coordinator context while preserving the project,
// main thread, conversation events and accumulated usage. Input admissions and
// recovery cannot cross the rotation boundary.
func (c *Coordinator) NewSession(projectID string, src Source) (*db.Thread, error) {
	c.admission.Lock()
	defer c.admission.Unlock()
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
	if c.Busy(projectID) {
		return nil, fmt.Errorf("coordinator is still running")
	}
	open, err := c.store.HasOpenCoordinatorInputs(projectID)
	if err != nil {
		return nil, err
	}
	if open {
		return nil, fmt.Errorf("coordinator has pending or unresolved input; resolve it first")
	}
	main, err := c.store.GetThread(p.MainThreadID)
	if err != nil {
		return nil, err
	}
	events, err := c.store.ListThreadEvents(main.ID)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.Kind != db.EvDecision {
			continue
		}
		var d Decision
		if err := json.Unmarshal([]byte(e.Payload), &d); err != nil {
			return nil, fmt.Errorf("invalid decision: %w", err)
		}
		if d.Chosen == nil {
			return nil, fmt.Errorf("answer the pending decision before starting a new session")
		}
	}
	if c.live() && main.SessionID != "" {
		if err := c.sessions.engine.Release(main.SessionID); err != nil {
			return nil, fmt.Errorf("cannot close coordinator session: %w", err)
		}
	}
	e := &db.ThreadEvent{}
	src.stamp(e)
	if err := c.store.RotateThreadSession(main.ID, main.SessionID, uuid.NewString(), e); err != nil {
		return nil, err
	}
	c.publishEvent(projectID, e)
	fresh, err := c.store.GetThread(main.ID)
	if err != nil {
		return nil, err
	}
	c.threads.publish(fresh)
	return fresh, nil
}
