package project

import (
	"errors"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// PostWith is Post for a person's message with a mode (SendWith's modes).
// While a coordinator turn runs, an insert goes straight into that turn: the
// model reads it at its next step, instead of the message waiting in the
// queue for a turn of its own. Anything that cannot join (no turn running,
// one that has not reached the model yet, a compaction, a slash command, an
// agent that reads nothing mid-turn) is queued exactly as Post does.
func (c *Coordinator) PostWith(projectID, text string, atts []Attachment, src Source, mode SendMode) (*db.ThreadEvent, error) {
	if mode != SendNextTurn && c.live() {
		p, err := c.store.GetProject(projectID)
		if err != nil {
			return nil, err
		}
		if p.ArchivedAt == nil && !p.Paused {
			if ev := c.sessions.insert(p, text, atts, src); ev != nil {
				_ = c.store.TouchProject(projectID)
				return ev, nil
			}
		}
	}
	return c.Post(projectID, text, atts, src)
}

// Insertable reports whether a message to the project's coordinator now
// would join the turn it is running.
func (c *Coordinator) Insertable(projectID string) bool {
	if !c.live() {
		return false
	}
	p, err := c.store.GetProject(projectID)
	if err != nil {
		return false
	}
	main, err := c.store.GetThread(p.MainThreadID)
	if err != nil {
		return false
	}
	return c.sessions.joinable(main.SessionID) && c.sessions.engine.Injectable(main.SessionID)
}

// joinable is whether the session's run is one a message may be added to: a
// turn on a user's input or a thread's report, not a compaction (its prompt
// is the summary request and nothing else).
func (s *coordinatorSessions) joinable(sid string) bool {
	if sid == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[sid]
	return run != nil && !run.compact
}

// insert adds a person's message to the coordinator turn in flight and
// records it in the conversation as one that joined a running turn. It
// returns nil when the message could not join; nothing is recorded then.
func (s *coordinatorSessions) insert(p *db.Project, text string, atts []Attachment, src Source) *db.ThreadEvent {
	main, err := s.c.store.GetThread(p.MainThreadID)
	if err != nil || !s.joinable(main.SessionID) {
		return nil
	}
	ev := &db.ThreadEvent{Kind: db.EvUser, Content: text, Payload: messagePayload(atts, true)}
	src.stamp(ev)
	if _, isCommand := coordinatorCommand(ev); isCommand {
		return nil
	}
	if cmd, ok := personCommand(text, src, len(atts)); ok && cmd.kind != notCommand {
		return nil
	}
	ev.ID, ev.CreatedAt = s.c.store.NewEventID(), time.Now()
	ev.ThreadID = p.MainThreadID
	msgs := renderHistory([]db.ThreadEvent{*ev}, s.c.layout.LibraryDir(p.ID))
	if len(msgs) == 0 {
		return nil
	}
	// The message is on its way as soon as Inject writes it, so the turn may
	// answer it before this function records it: say so first.
	s.mu.Lock()
	s.answerOutsideRun[main.SessionID] = true
	s.mu.Unlock()
	if err := s.engine.Inject(main.SessionID, msgs[0].Content); err != nil {
		if !isNotInjectable(err) {
			logrus.WithError(err).Warn("insert into the coordinator's turn")
		}
		return nil
	}
	if err := s.c.store.AddThreadEventAs(ev); err != nil {
		logrus.WithError(err).Warn("save conversation event")
	}
	s.c.publishEvent(p.ID, ev)
	return ev
}

func isNotInjectable(err error) bool { return errors.Is(err, engine.ErrNotInjectable) }
