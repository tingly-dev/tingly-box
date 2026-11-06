package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// compactCapture collects what the backend reports about the compaction a
// Compact call started.
type compactCapture struct {
	mu     sync.Mutex
	events []agentboot.CompactEvent
}

func (c *compactCapture) add(ev agentboot.CompactEvent) {
	c.mu.Lock()
	c.events = append(c.events, ev)
	c.mu.Unlock()
}

// outcome reads the finished turn's compaction events.
func (c *compactCapture) outcome() (agentboot.CompactResult, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ev := range c.events {
		switch ev.Phase {
		case agentboot.CompactFailed:
			return agentboot.CompactResult{}, &agentboot.CompactFailedError{Reason: ev.Reason}, true
		case agentboot.CompactDone:
			return ev.Result, nil, true
		}
	}
	return agentboot.CompactResult{}, nil, false
}

// noteCompact hands a compaction event to the Compact call waiting for it.
func (e *Engine) noteCompact(sessionID string, ev agentboot.CompactEvent) {
	e.mu.Lock()
	r := e.runs[sessionID]
	e.mu.Unlock()
	if r != nil && r.compact != nil {
		r.compact.add(ev)
	}
}

// Compact compacts session id's conversation and returns what the backend
// reports of it. It claims the session like Start does (ErrBusy if a turn is
// in flight), runs the compaction as one turn in dir through the same
// process the session's other turns use, records the outcome as KindCompact
// entries and returns when the turn ends. Cancelling ctx stops it. The
// session must already have a conversation to resume.
//
// Errors: [agentboot.ErrCompactUnsupported] (the backend cannot compact),
// *[agentboot.CompactFailedError] (it declined), the turn's own failure,
// ErrBusy (a turn was running: nothing was started) or ErrNotSent (the agent
// began a turn first; OnTurnEnd reported it with NotSent).
func (e *Engine) Compact(ctx context.Context, id, dir, instructions string, launch Launch) (agentboot.CompactResult, error) {
	prompt, err := e.agent.CompactPrompt("", instructions)
	if err != nil {
		return agentboot.CompactResult{}, err
	}
	capture := &compactCapture{}
	r, ok := e.begin(id, dir, prompt, true, launch, capture)
	if !ok {
		return agentboot.CompactResult{}, ErrBusy
	}
	select {
	case <-r.done:
	case <-ctx.Done():
		e.Stop(id)
		<-r.done
	}
	e.mu.Lock()
	end := r.end
	e.mu.Unlock()
	switch {
	case end.Err != nil:
		return agentboot.CompactResult{}, end.Err
	case end.NotSent:
		return agentboot.CompactResult{}, ErrNotSent
	case end.Stopped:
		return agentboot.CompactResult{}, context.Canceled
	}
	res, err, reported := capture.outcome()
	if !reported {
		return agentboot.CompactResult{}, errors.New("engine: the turn ended without a compaction outcome")
	}
	return res, err
}

// ContextUsage asks session id's live process how full its context is.
// A session with no process returns [agentboot.ErrNoProcess]; the host then
// falls back to the figure its last turn recorded.
func (e *Engine) ContextUsage(ctx context.Context, id string) (agentboot.ContextUsage, error) {
	res, ok := e.residentFor(id)
	if !ok {
		return agentboot.ContextUsage{}, agentboot.ErrNoProcess
	}
	ctx, cancel := context.WithTimeout(ctx, liveControlTimeout)
	defer cancel()
	return res.c.ContextUsage(ctx)
}

// compactEntry renders a CompactEvent as a transcript entry. Content is a
// one-line text for hosts that only print it; Payload is the event.
func compactEntry(ev agentboot.CompactEvent) Entry {
	payload, _ := json.Marshal(ev)
	var text string
	switch ev.Phase {
	case agentboot.CompactStarted:
		text = "compacting the conversation"
	case agentboot.CompactFailed:
		text = "could not compact"
		if ev.Reason != "" {
			text += ": " + ev.Reason
		}
	default:
		text = "compacted the conversation"
		if ev.Result.Reported.Tokens {
			text = fmt.Sprintf("compacted the conversation: %d → %d tokens", ev.Result.TokensBefore, ev.Result.TokensAfter)
		}
	}
	return Entry{Kind: KindCompact, Content: text, Payload: payload, Timestamp: time.Now()}
}
