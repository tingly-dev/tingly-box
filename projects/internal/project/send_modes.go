package project

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// SendMode is how a user's message meets a turn that is already running
// (.design/agentboot-live-control.md §3). With no turn running every mode is
// a plain send.
type SendMode string

const (
	// SendNow is the default, as in Claude Code: the message joins the running
	// turn and is read at Claude's next step, or runs as the next turn if the
	// turn ends first. If it cannot join, it is held like SendQueue.
	SendNow SendMode = "send"
	// SendQueue holds the message until the turn ends; until then it is a
	// chip the user can take back.
	SendQueue SendMode = "queue"
	// SendStopAndSend interrupts the turn and sends the message after it.
	SendStopAndSend SendMode = "stop_and_send"
)

// Valid reports whether m is a known mode; the empty mode means SendNow.
func (m SendMode) Valid() bool {
	switch m {
	case "", SendNow, SendQueue, SendStopAndSend:
		return true
	}
	return false
}

// SendWith sends a user's message the way mode says.
func (m *Threads) SendWith(ctx context.Context, threadID, text string, atts []Attachment, src Source, mode SendMode) (*db.Thread, error) {
	if _, err := m.sendable(threadID); err != nil {
		return nil, err
	}
	q := queued{text: text, src: src, atts: atts}
	switch mode {
	case SendQueue:
		q.held = true
	case SendStopAndSend:
		if done := m.stop(threadID); done != nil {
			select {
			case <-done:
			case <-time.After(agentboot.SessionCloseTimeout):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	default:
		if m.inject(threadID, q) {
			return m.store.GetThread(threadID)
		}
		q.held = true
	}
	m.enqueue(threadID, q)
	return m.store.GetThread(threadID)
}

// inject adds q to the turn in flight. False means it could not join (no
// turn running, or the agent cannot take it) and the caller holds it.
func (m *Threads) inject(threadID string, q queued) bool {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return false
	}
	p, err := m.store.GetProject(t.ProjectID)
	if err != nil {
		return false
	}
	prompt := withAttachments(q.text, m.layout.LibraryDir(UploadOwner(p, t)), q.atts)
	if err := m.engine.Inject(m.sessionOf(threadID), prompt); err != nil {
		if !errors.Is(err, engine.ErrNotInjectable) {
			m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "could not add the message to the running turn: " + err.Error()})
		}
		return false
	}
	ev := &db.ThreadEvent{Kind: db.EvUser, Content: q.text, Payload: messagePayload(q.atts, true)}
	q.src.stamp(ev)
	m.addEvent(t, ev)
	return true
}

// queuedPayload is a chip event's payload.
type queuedPayload struct {
	State       string       `json:"state"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// messagePayload records a user message's attachments, and that it joined a
// running turn rather than starting one (the page marks it until Claude has
// read it).
func messagePayload(atts []Attachment, injected bool) string {
	if !injected {
		return AttachmentsPayload(atts)
	}
	return jsonString(map[string]any{"injected": true, "attachments": atts})
}

// holdMessage records q as a chip above the composer and returns its id.
func (m *Threads) holdMessage(t *db.Thread, q queued) string {
	ev := &db.ThreadEvent{Kind: db.EvQueued, Content: q.text, Payload: jsonString(queuedPayload{State: "held", Attachments: q.atts})}
	q.src.stamp(ev)
	m.addEvent(t, ev)
	return ev.ID
}

// setChip moves a chip to state, publishing the change.
func (m *Threads) setChip(t *db.Thread, eventID, state string) (*db.ThreadEvent, bool) {
	events, err := m.store.ListThreadEvents(t.ID)
	if err != nil {
		return nil, false
	}
	for i := range events {
		e := &events[i]
		if e.ID != eventID || e.Kind != db.EvQueued {
			continue
		}
		var pl queuedPayload
		_ = json.Unmarshal([]byte(e.Payload), &pl)
		if pl.State != "held" {
			return e, false
		}
		pl.State = state
		e.Payload = jsonString(pl)
		_ = m.store.UpdateThreadEvent(e)
		m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: e})
		return e, true
	}
	return nil, false
}

// releaseHeld is called when q's turn comes: a held message becomes the
// user's message in the transcript and its chip goes away.
func (m *Threads) releaseHeld(t *db.Thread, q *queued) {
	if !q.held {
		return
	}
	q.held = false
	m.setChip(t, q.eventID, "sent")
	ev := &db.ThreadEvent{Kind: db.EvUser, Content: q.text, Payload: AttachmentsPayload(q.atts)}
	q.src.stamp(ev)
	m.addEvent(t, ev)
}

// dropHeld removes the chips of messages that will not be sent (Stop).
func (m *Threads) dropHeld(threadID string, qs []queued) {
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return
	}
	for _, q := range qs {
		if q.held {
			m.setChip(t, q.eventID, "removed")
		}
	}
}

// RemoveQueued takes a held message back before it is sent. False means it
// was already sent or removed.
func (m *Threads) RemoveQueued(threadID, eventID string) bool {
	m.mu.Lock()
	removed := false
	if a := m.active[threadID]; a != nil {
		for i, q := range a.queue {
			if q.held && q.eventID == eventID {
				a.queue = append(a.queue[:i:i], a.queue[i+1:]...)
				removed = true
				break
			}
		}
	}
	m.mu.Unlock()
	if !removed {
		return false
	}
	t, err := m.store.GetThread(threadID)
	if err != nil {
		return true
	}
	m.setChip(t, eventID, "removed")
	return true
}
