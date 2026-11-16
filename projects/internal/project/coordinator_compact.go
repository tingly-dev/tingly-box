package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// /compact for the project conversation (.design/slash-commands.md). The
// message is queued like any other input, so it waits for the turn in flight
// and survives a restart; when its turn comes the coordinator compacts
// instead of answering.

// coordinatorCommand reads a queued input as a slash command: only a
// person's text message, with no files.
func coordinatorCommand(event *db.ThreadEvent) (command, bool) {
	if event == nil || event.Kind != db.EvUser {
		return command{}, false
	}
	cmd, ok := personCommand(event.Content, Source{Kind: event.Source}, len(ParseAttachments(event.Payload)))
	return cmd, ok && cmd.kind == cmdCompact
}

func (s *coordinatorSessions) takeResend(projectID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	was := s.resend[projectID]
	delete(s.resend, projectID)
	return was
}

// compact runs a /compact the user sent to the project conversation. Only a
// session that is busy fails the queue, as it does for any turn: a compaction
// is harmless to retry by hand, so every other problem is told to the user
// instead of leaving the input needing recovery.
func (s *coordinatorSessions) compact(ctx context.Context, p *db.Project, main *db.Thread, instructions string) error {
	tell := func(kind, text string) {
		s.c.final(s.c.addEvent(p.ID, &db.ThreadEvent{Kind: kind, Content: text}))
	}
	if main.Turns == 0 {
		s.c.addEvent(p.ID, &db.ThreadEvent{Kind: db.EvSystem, Content: ErrNothingToCompact.Error() + "."})
		return nil
	}
	config, err := s.tools.config(main.ID)
	if err != nil {
		tell(db.EvError, fmt.Sprintf("Could not compact: coordinator tools: %v", err))
		return nil
	}
	opts, err := s.launchOptions(ctx, p, config)
	if err != nil {
		tell(db.EvError, "Could not compact: "+err.Error())
		return nil
	}
	run := &coordinatorSessionRun{end: make(chan engine.TurnEnd, 1), threadID: main.ID, projectID: p.ID, compact: true}
	s.mu.Lock()
	s.runs[main.SessionID] = run
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.runs, main.SessionID); s.mu.Unlock() }()

	res, err := s.engine.Compact(ctx, main.SessionID, main.Cwd, instructions, func(context.Context) agentboot.ExecutionOptions { return opts })
	var refused *agentboot.CompactFailedError
	switch {
	case err == nil:
		s.mu.Lock()
		s.resend[p.ID] = true
		s.mu.Unlock()
		s.refreshContext(main, res)
	case errors.As(err, &refused):
		// The card says why; nothing else to do.
	case errors.Is(err, agentboot.ErrCompactUnsupported):
		tell(db.EvError, "Could not compact: this agent cannot compact its conversation.")
	case errors.Is(err, engine.ErrBusy), errors.Is(err, engine.ErrNotSent):
		return fmt.Errorf("coordinator session is busy")
	default:
		tell(db.EvError, "Could not compact: "+err.Error())
	}
	return nil
}

// refreshContext sets the coordinator's context figures after a compaction,
// which makes no model call and so records no usage.
func (s *coordinatorSessions) refreshContext(main *db.Thread, res agentboot.CompactResult) {
	used, window, ok := contextAfterCompact(s.engine, main.SessionID, res)
	if !ok {
		return
	}
	main.ContextTokens = used
	if window > 0 {
		main.ContextWindow = int(window)
	}
	_ = s.c.store.SaveThreadUsage(main)
}

// noteCompact keeps the project conversation's compaction card, like
// Threads.noteCompact does for a thread. The chats bound to the conversation
// hear the outcome of a compaction they asked for.
func (s *coordinatorSessions) noteCompact(sessionID string, e engine.Entry) {
	var ce agentboot.CompactEvent
	if json.Unmarshal(e.Payload, &ce) != nil {
		return
	}
	s.mu.Lock()
	run := s.runs[sessionID]
	if run != nil && ce.Phase == agentboot.CompactDone {
		run.compacted = true
	}
	s.mu.Unlock()
	if run == nil {
		return
	}
	s.mu.Lock()
	card := run.card
	s.mu.Unlock()
	if ce.Phase == agentboot.CompactStarted || card == nil {
		card = s.c.addEvent(run.projectID, &db.ThreadEvent{Kind: db.EvCommand, Content: "compact", Payload: jsonString(openingPayload(run.compact))})
		s.mu.Lock()
		run.card = card
		s.mu.Unlock()
		if ce.Phase == agentboot.CompactStarted {
			return
		}
	}
	var pl commandPayload
	_ = json.Unmarshal([]byte(card.Payload), &pl)
	pl = pl.settled(ce)
	card.Payload = jsonString(pl)
	_ = s.c.store.UpdateThreadEvent(card)
	s.c.publishEvent(run.projectID, card)
	s.mu.Lock()
	run.card = nil
	s.mu.Unlock()
	if run.compact {
		s.c.final(&db.ThreadEvent{Kind: db.EvAssistant, ThreadID: card.ThreadID, Content: compactLine(pl)})
	}
}
