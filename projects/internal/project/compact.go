package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// /compact for a thread (.design/slash-commands.md): the user's line is a
// message like any other, then a compaction runs as the thread's turn and
// leaves one card in the transcript.

// commandPayload is the payload of an EvCommand event.
type commandPayload struct {
	Name         string `json:"name"`
	Args         string `json:"args,omitempty"`
	State        string `json:"state"` // running, ok, failed
	Trigger      string `json:"trigger,omitempty"`
	TokensBefore int64  `json:"tokens_before,omitempty"`
	TokensAfter  int64  `json:"tokens_after,omitempty"`
	// TokensReported says the two figures above are the agent's; when false
	// they are unknown, not zero.
	TokensReported bool   `json:"tokens_reported,omitempty"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
	Summary        string `json:"summary,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// Compact asks the thread's agent to compact its conversation. Like any
// message it runs after the turn in flight; the user's /compact line is in
// the transcript at once (or when its turn comes, if it had to wait).
func (m *Threads) Compact(ctx context.Context, threadID, instructions string, src Source) (*db.Thread, error) {
	t, err := m.sendable(threadID)
	if err != nil {
		return nil, err
	}
	// Before its first turn has ended a thread has no conversation to compact,
	// unless that turn is still running: the request then waits behind it.
	if t.Turns == 0 && !m.Running(threadID) {
		return nil, ErrNothingToCompact
	}
	m.enqueue(threadID, queued{text: compactText(instructions), src: src, held: true, compact: &compactRequest{instructions: instructions}})
	return m.store.GetThread(threadID)
}

// compactReturned finishes the begin of a compaction turn. When the turn ran,
// turnEnded has already settled the thread (and, for a turn the agent
// started first, re-queued the request); the context figures are refreshed
// here. When Engine.Compact never started a turn, the request is put back or
// dropped.
func (m *Threads) compactReturned(t *db.Thread, a *activeThread, slot *limiter, res agentboot.CompactResult, err error) {
	switch {
	case errors.Is(err, engine.ErrBusy):
		// A turn of the agent's own holds the session: this waits for it.
		m.mu.Lock()
		delete(m.turns, t.ID)
		a.queue = append([]queued{*a.current}, a.queue...)
		a.current, a.slot = nil, nil
		m.mu.Unlock()
		slot.release()
		m.next(t.ID)
	case errors.Is(err, agentboot.ErrCompactUnsupported):
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: "This agent cannot compact its conversation."})
		m.mu.Lock()
		delete(m.turns, t.ID)
		a.slot = nil
		m.mu.Unlock()
		slot.release()
		m.giveUp(t.ID, a)
	case err == nil:
		m.refreshContext(t.ID, res)
	}
}

// refreshContext sets the thread's context figures after a compaction: a
// compaction turn makes no model call, so no usage entry would update them.
// The live process knows exactly; failing that, the compaction's own figure.
func (m *Threads) refreshContext(threadID string, res agentboot.CompactResult) {
	used, window := int64(0), int64(0)
	if u, err := m.engine.ContextUsage(context.Background(), m.sessionOf(threadID)); err == nil {
		used, window = u.Used, u.Max
	} else if res.Reported.Tokens {
		used = res.TokensAfter
	} else {
		return
	}
	_, _ = m.update(threadID, func(t *db.Thread) {
		t.ContextTokens = used
		if window > 0 {
			t.ContextWindow = int(window)
		}
	})
}

// openingPayload is the card of a compaction that has just started.
func openingPayload(manual bool) commandPayload {
	trigger := string(agentboot.CompactAuto)
	if manual {
		trigger = string(agentboot.CompactManual)
	}
	return commandPayload{Name: "compact", State: "running", Trigger: trigger}
}

// settled is the card after the agent reported how the compaction ended.
func (pl commandPayload) settled(ce agentboot.CompactEvent) commandPayload {
	switch ce.Phase {
	case agentboot.CompactDone:
		r := ce.Result
		pl.State, pl.DurationMS, pl.Summary = "ok", r.DurationMS, r.Summary
		if r.Trigger != "" {
			pl.Trigger = string(r.Trigger)
		}
		if r.Reported.Tokens {
			pl.TokensBefore, pl.TokensAfter, pl.TokensReported = r.TokensBefore, r.TokensAfter, true
		}
	case agentboot.CompactFailed:
		pl.State, pl.Reason = "failed", ce.Reason
	}
	return pl
}

// stepCard applies a compaction event to the compaction card: the compaction
// starting (or an event with no card open, as for an automatic compaction)
// opens a card through add, and its end settles it through update. open is the
// card still open afterwards, and settled the final payload when ce ended it.
func stepCard(card *db.ThreadEvent, ce agentboot.CompactEvent, manual bool, add, update func(*db.ThreadEvent)) (open *db.ThreadEvent, settled *commandPayload) {
	if ce.Phase == agentboot.CompactStarted || card == nil {
		card = &db.ThreadEvent{Kind: db.EvCommand, Content: "compact", Payload: jsonString(openingPayload(manual))}
		add(card)
		if ce.Phase == agentboot.CompactStarted {
			return card, nil
		}
	}
	var pl commandPayload
	_ = json.Unmarshal([]byte(card.Payload), &pl)
	pl = pl.settled(ce)
	card.Payload = jsonString(pl)
	update(card)
	return nil, &pl
}

// noteCompact keeps the thread's compaction card: one event, created when the
// compaction starts and updated in place when it ends. A compaction nobody
// asked for (the agent's own) gets a card the same way. A requested one also
// becomes the turn's text, which the chats bound to the thread hear.
func (m *Threads) noteCompact(t *db.Thread, e engine.Entry) {
	var ce agentboot.CompactEvent
	if json.Unmarshal(e.Payload, &ce) != nil {
		return
	}
	m.mu.Lock()
	card, tally := m.cards[t.ID], m.turns[t.ID]
	m.mu.Unlock()
	manual := tally != nil && tally.compact

	open, settled := stepCard(card, ce, manual,
		func(c *db.ThreadEvent) { m.addEvent(t, c) },
		func(c *db.ThreadEvent) {
			_ = m.store.UpdateThreadEvent(c)
			m.hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: c})
		})
	m.mu.Lock()
	defer m.mu.Unlock()
	if open != nil {
		m.cards[t.ID] = open
		return
	}
	delete(m.cards, t.ID)
	if settled != nil && manual {
		tally.text = compactLine(*settled)
	}
}

// compactLine is the one-line outcome a chat hears.
func compactLine(pl commandPayload) string {
	if pl.State == "failed" {
		if pl.Reason == "" {
			return "Could not compact the conversation."
		}
		return "Could not compact: " + strings.TrimRight(pl.Reason, ".") + "."
	}
	if pl.TokensReported {
		return fmt.Sprintf("Compacted: %s → %s tokens.", tokenCount(pl.TokensBefore), tokenCount(pl.TokensAfter))
	}
	return "Compacted the conversation."
}

func tokenCount(n int64) string {
	if n >= 1000 {
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	}
	return fmt.Sprint(n)
}
