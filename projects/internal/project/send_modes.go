package project

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// SendMode is how a user's message meets a turn that is already running
// (.design/agentboot-live-control.md §3). With no turn running every mode is
// a plain send. Interrupting a turn is not a way of sending: it is Stop.
type SendMode string

const (
	// SendInsert is the default: the message joins the running turn and the
	// model reads it at its next step (the next model request, after the
	// running tool call returns), or it runs as the next turn if the turn
	// ends first. If the turn cannot take it (the agent reads no input
	// mid-turn, or the turn is still waiting for a project slot), it is held
	// like SendNextTurn.
	SendInsert SendMode = "insert"
	// SendNextTurn holds the message until the turn ends; until then it is a
	// chip the user can take back.
	SendNextTurn SendMode = "next_turn"
)

// legacyModes are the names the API used before insert / next_turn; a page
// loaded before the rename still sends them. stop_and_send has no
// counterpart (stopping is its own call) and stays refused.
var legacyModes = map[SendMode]SendMode{"send": SendInsert, "queue": SendNextTurn}

// Canonical is m with a legacy name replaced by the mode it now has.
func (m SendMode) Canonical() SendMode {
	if c, ok := legacyModes[m]; ok {
		return c
	}
	return m
}

// Valid reports whether m is a known mode; the empty mode means SendInsert.
func (m SendMode) Valid() bool {
	switch m.Canonical() {
	case "", SendInsert, SendNextTurn:
		return true
	}
	return false
}

// SendWith sends a message the way mode says.
func (m *Threads) SendWith(ctx context.Context, threadID, text string, atts []Attachment, src Source, mode SendMode) (*db.Thread, error) {
	t, _, err := m.sendWith(ctx, threadID, text, atts, src, mode)
	return t, err
}

// sendWith is SendWith that also says whether the message joined a running
// turn (so a caller can tell its own user how it was delivered).
func (m *Threads) sendWith(_ context.Context, threadID, text string, atts []Attachment, src Source, mode SendMode) (*db.Thread, bool, error) {
	lock := m.lifecycleLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	if _, err := m.sendable(threadID); err != nil {
		return nil, false, err
	}
	q := queued{text: text, src: src, atts: atts, relayed: src.Kind == db.SourceThread}
	joined := false
	if mode == SendNextTurn || !m.inject(threadID, q) {
		q.held = true
	} else {
		joined = true
	}
	if !joined {
		m.enqueue(threadID, q)
	}
	t, err := m.store.GetThread(threadID)
	return t, joined, err
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
	prompt := m.threadPrompt(q, p, t)
	sid := m.sessionOf(threadID)
	if err := m.engine.Inject(sid, prompt); err != nil {
		switch {
		case !errors.Is(err, engine.ErrNotInjectable):
			m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "could not add the message to the running turn: " + err.Error()})
		case errors.Is(err, engine.ErrAgentReadsBetweenTurns):
			// A turn runs but its agent reads nothing while it works: say so,
			// instead of leaving the user to wonder why it was not read yet.
			m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "this agent cannot take a message while it works; it will read yours after the current turn"})
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
