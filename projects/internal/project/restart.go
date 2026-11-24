package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

var ErrRestarting = errors.New("the session is restarting")

// Restart closes the agent process even when a turn is stuck. The next
// message resumes the same session in a new process.
func (m *Threads) Restart(threadID string) (*db.Thread, error) {
	m.startMu.Lock()
	t, err := m.restartable(threadID)
	if err != nil {
		m.startMu.Unlock()
		return nil, err
	}
	m.mu.Lock()
	m.restarting[threadID] = true
	if a := m.active[threadID]; a != nil {
		a.ending = true
	}
	m.mu.Unlock()
	m.stopQueued(threadID, Source{})
	m.startMu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.restarting, threadID)
		m.mu.Unlock()
	}()

	// A tool's HTTP call can be waiting independently of the agent turn.
	// Cancel those requests explicitly and reject new ones until teardown.
	p := m.prompter(threadID)
	pending := p.pauseRequests()
	defer p.resumeRequests()

	if err := m.engine.Restart(t.SessionID); err != nil {
		return nil, fmt.Errorf("could not stop the session process: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentboot.SessionCloseTimeout)
	defer cancel()
	for _, pd := range pending {
		select {
		case <-pd.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	m.next(threadID)
	m.dropHeldReport(threadID)
	m.revokeTools(threadID)
	if err := m.expireRequests(t); err != nil {
		return nil, err
	}
	restarted := false
	t, err = m.update(threadID, func(cur *db.Thread) {
		if cur.Status == db.ThreadResolved || cur.ArchivedAt != nil || cur.SessionID != t.SessionID {
			return
		}
		cur.Status, cur.WaitReason = db.RestingStatus(cur), ""
		restarted = true
	})
	if err == nil && restarted {
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "Session stopped for restart. Send a message to continue."})
	}
	return t, err
}

func (m *Threads) restartable(threadID string) (*db.Thread, error) {
	m.mu.Lock()
	restarting := m.restarting[threadID]
	m.mu.Unlock()
	if restarting {
		return nil, ErrRestarting
	}
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	switch {
	case t.Agent == db.AgentLLM:
		return nil, ErrMainThread
	case t.ArchivedAt != nil:
		return nil, ErrThreadArchived
	case t.Status == db.ThreadResolved:
		return nil, ErrResolved
	default:
		return t, nil
	}
}

func (p *threadPrompter) pauseRequests() []*pending {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = true
	pending := make([]*pending, 0, len(p.pending))
	for _, pd := range p.pending {
		pending = append(pending, pd)
		pd.cancel()
	}
	return pending
}

func (p *threadPrompter) resumeRequests() {
	p.mu.Lock()
	p.paused = false
	p.mu.Unlock()
}

func (m *Threads) expireRequests(t *db.Thread) error {
	events, err := m.store.ListRequestEvents(t.ID)
	if err != nil {
		return err
	}
	for i := range events {
		ev := &events[i]
		var payload requestPayload
		if json.Unmarshal([]byte(ev.Payload), &payload) != nil || payload.State != "pending" {
			continue
		}
		payload.State, payload.AutoAt = "expired", nil
		ev.Payload = jsonString(payload)
		if err := m.store.UpdateThreadEvent(ev); err != nil {
			return err
		}
		m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: ev})
	}
	return nil
}
