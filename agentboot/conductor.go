package agentboot

import (
	"context"
	"errors"
	"sync"

	"github.com/sirupsen/logrus"
)

// ConductorHooks are a [Conductor]'s callbacks. All run on its reader
// goroutine, in event order.
type ConductorHooks struct {
	// Sink receives every message the process emits, in a turn or between
	// turns — a background task's progress arrives while the session is idle.
	Sink MessageSink

	// OnUnsolicitedTurn is called when the agent starts a turn by itself
	// (see [TurnStartEvent]). It returns the context and Prompter for that
	// turn's approvals; a nil Prompter denies them. Canceling the context
	// only unblocks a pending approval — stop the turn with Interrupt.
	OnUnsolicitedTurn func() (context.Context, Prompter)

	// OnUnsolicitedTurnComplete reports the end of such a turn. Turns
	// started through RunTurn/Await report to their caller instead.
	OnUnsolicitedTurnComplete func(res *Result, err error)

	// OnTerminated reports the process ending — Close, a crash, or a
	// protocol error. The Conductor is done after it.
	OnTerminated func(reason string)
}

// Conductor drives every turn of a [PersistentSession] from one reader that
// lives as long as the process does. [RunTurnWithPrompter] reads only while
// the caller's turn is in flight, so anything the process emits between
// turns — background task progress, or a whole turn the agent starts on its
// own when a background task finishes — would sit unread in the session's
// buffer and be mistaken for the next turn's events. A Conductor reads it as
// it happens: messages go to Sink, an agent-started turn is announced through
// OnUnsolicitedTurn and closed through OnUnsolicitedTurnComplete.
//
// Once a session has a Conductor, nothing else may read its Events().
type Conductor struct {
	session PersistentSession
	hooks   ConductorHooks

	mu   sync.Mutex
	turn *conductedTurn // the turn in flight, whoever started it

	done chan struct{}
}

type conductedTurn struct {
	ctx         context.Context
	prompter    Prompter
	unsolicited bool
	outcome     chan turnOutcome // solicited turns only; buffered
}

type turnOutcome struct {
	res *Result
	err error
}

// NewConductor starts reading session's events. Call it right after
// [Runner.Open], before the first turn's events can pile up; that first
// turn, already submitted by Open, is then collected with Await.
func NewConductor(session PersistentSession, hooks ConductorHooks) *Conductor {
	c := &Conductor{session: session, hooks: hooks, done: make(chan struct{})}
	go c.read()
	return c
}

// Session returns the conducted session.
func (c *Conductor) Session() PersistentSession { return c.session }

// Done is closed once the process has ended and every hook has run.
func (c *Conductor) Done() <-chan struct{} { return c.done }

// Await collects the turn already in flight from Runner.Open, answering its
// approvals with prompter. Like RunTurn, it returns the turn's result.
func (c *Conductor) Await(ctx context.Context, prompter Prompter) (*Result, error) {
	t := &conductedTurn{ctx: ctx, prompter: prompter, outcome: make(chan turnOutcome, 1)}
	c.mu.Lock()
	if c.turn != nil {
		c.mu.Unlock()
		return nil, ErrTurnInFlight
	}
	c.turn = t
	c.mu.Unlock()
	return c.wait(ctx, t)
}

// RunTurn sends prompt as the next turn and waits for it to complete.
// Returns ErrTurnInFlight if a turn is already in flight — including one the
// agent started itself. Canceling ctx interrupts the turn (the process and
// its background work survive) rather than closing the session; only an
// agent without interrupt support, or one that ignores it, is closed.
func (c *Conductor) RunTurn(ctx context.Context, prompt string, prompter Prompter) (*Result, error) {
	t := &conductedTurn{ctx: ctx, prompter: prompter, outcome: make(chan turnOutcome, 1)}
	c.mu.Lock()
	if c.turn != nil {
		c.mu.Unlock()
		return nil, ErrTurnInFlight
	}
	// Registered before Send, under the lock the reader takes to route a
	// turn's events, so none of this turn's events can reach a turn-less
	// Conductor.
	c.turn = t
	err := c.session.Send(ctx, prompt)
	if err != nil {
		c.turn = nil
	}
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return c.wait(ctx, t)
}

// Interrupt stops the turn in flight, whoever started it; see
// [PersistentSession.Interrupt].
func (c *Conductor) Interrupt(ctx context.Context) error { return c.session.Interrupt(ctx) }

// StopTask stops one background task; see [PersistentSession.StopTask].
func (c *Conductor) StopTask(ctx context.Context, taskID string) error {
	return c.session.StopTask(ctx, taskID)
}

func (c *Conductor) wait(ctx context.Context, t *conductedTurn) (*Result, error) {
	select {
	case o := <-t.outcome:
		return o.res, o.err
	case <-ctx.Done():
	}

	// Stop just this turn and wait for its result, so the next turn starts
	// from a clean boundary. Past the grace period — or with no interrupt to
	// send — close the session instead, like RunTurnWithPrompter does.
	graceCtx, cancel := context.WithTimeout(context.Background(), SessionCloseTimeout)
	defer cancel()
	if err := c.session.Interrupt(graceCtx); err == nil {
		select {
		case <-t.outcome:
			return nil, ctx.Err()
		case <-graceCtx.Done():
		}
	} else if !errors.Is(err, ErrControlUnsupported) {
		logrus.WithError(err).Warn("agentboot.Conductor: interrupt failed; closing the session")
	}
	closeCtx, cancelClose := context.WithTimeout(context.Background(), SessionCloseTimeout)
	defer cancelClose()
	_ = c.session.Close(closeCtx)
	return nil, ctx.Err()
}

func (c *Conductor) read() {
	defer close(c.done)
	for ev := range c.session.Events() {
		switch e := ev.(type) {
		case MessageEvent:
			if c.hooks.Sink != nil {
				c.hooks.Sink(e.Raw)
			}

		case ErrorEvent:
			logrus.WithError(e.Err).Warn("agentboot.Conductor: agent ErrorEvent")
			if c.hooks.Sink != nil {
				c.hooks.Sink(e)
			}

		case ApprovalRequestEvent:
			ctx, prompter := c.current()
			res := ApprovalResponse{Approved: false, Reason: "no turn is waiting for this request"}
			if prompter != nil {
				r, err := prompter.OnApproval(ctx, e)
				if err != nil {
					r = ApprovalResponse{Approved: false, Reason: err.Error()}
				}
				res = r
			}
			if err := c.session.Respond(e.ID, res); err != nil {
				logrus.WithError(err).Warn("agentboot.Conductor: Respond error")
			}

		case AskRequestEvent:
			ctx, prompter := c.current()
			res := AskResponse{Approved: false, Reason: "no turn is waiting for this request"}
			if prompter != nil {
				r, err := prompter.OnAsk(ctx, e)
				if err != nil {
					r = AskResponse{Approved: false, Reason: err.Error()}
				}
				res = r
			}
			if err := c.session.Respond(e.ID, res); err != nil {
				logrus.WithError(err).Warn("agentboot.Conductor: Respond error")
			}

		case TurnStartEvent:
			if !e.Unsolicited {
				continue
			}
			ctx, prompter := context.Background(), Prompter(nil)
			if c.hooks.OnUnsolicitedTurn != nil {
				ctx, prompter = c.hooks.OnUnsolicitedTurn()
			}
			c.mu.Lock()
			c.turn = &conductedTurn{ctx: ctx, prompter: prompter, unsolicited: true}
			c.mu.Unlock()

		case TurnCompleteEvent:
			var err error
			if e.Result != nil && e.Result.Error != "" {
				err = errors.New(e.Result.Error)
			}
			c.finish(e.Result, err)

		case SessionStateEvent:
			if e.State != SessionStateTerminated {
				continue
			}
			c.finish(nil, errors.New("agentboot: persistent session terminated before this turn completed: "+e.Reason))
			if c.hooks.OnTerminated != nil {
				c.hooks.OnTerminated(e.Reason)
			}
		}
	}
}

// current returns the context and Prompter of the turn in flight.
func (c *Conductor) current() (context.Context, Prompter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.turn == nil {
		return context.Background(), nil
	}
	return c.turn.ctx, c.turn.prompter
}

// finish closes the turn in flight, if any, with its outcome.
func (c *Conductor) finish(res *Result, err error) {
	c.mu.Lock()
	t := c.turn
	c.turn = nil
	c.mu.Unlock()
	switch {
	case t == nil:
	case t.unsolicited:
		if c.hooks.OnUnsolicitedTurnComplete != nil {
			c.hooks.OnUnsolicitedTurnComplete(res, err)
		}
	default:
		t.outcome <- turnOutcome{res: res, err: err}
	}
}
