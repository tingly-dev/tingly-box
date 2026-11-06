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
	if event == nil || event.Kind != db.EvUser || !byPerson(Source{Kind: event.Source}) || len(ParseAttachments(event.Payload)) > 0 {
		return command{}, false
	}
	cmd := parseCommand(event.Content)
	return cmd, cmd.kind == cmdCompact
}

func (s *coordinatorSessions) takeResend(projectID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	was := s.resend[projectID]
	delete(s.resend, projectID)
	return was
}

// compact runs a /compact the user sent to the project conversation. Nothing
// here fails the queue: a compaction is harmless to retry by hand, so a
// problem is told to the user instead of leaving the input needing recovery.
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
	case errors.Is(err, engine.ErrBusy):
		return fmt.Errorf("coordinator session is busy")
	default:
		tell(db.EvError, "Could not compact: "+err.Error())
	}
	return nil
}

// refreshContext sets the coordinator's context figures after a compaction,
// which makes no model call and so records no usage.
func (s *coordinatorSessions) refreshContext(main *db.Thread, res agentboot.CompactResult) {
	used, window := int64(0), int64(0)
	if u, err := s.engine.ContextUsage(context.Background(), main.SessionID); err == nil {
		used, window = u.Used, u.Max
	} else if res.Reported.Tokens {
		used = res.TokensAfter
	} else {
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
	var card *db.ThreadEvent
	if run != nil {
		card = run.card
	}
	s.mu.Unlock()
	if run == nil {
		return
	}
	open, settled := stepCard(card, ce, run.compact,
		func(c *db.ThreadEvent) { s.c.addEvent(run.projectID, c) },
		func(c *db.ThreadEvent) {
			_ = s.c.store.UpdateThreadEvent(c)
			s.c.publishEvent(run.projectID, c)
		})
	s.mu.Lock()
	run.card = open
	s.mu.Unlock()
	if settled != nil && run.compact {
		s.c.final(&db.ThreadEvent{Kind: db.EvAssistant, ThreadID: run.threadID, Content: compactLine(*settled)})
	}
}
