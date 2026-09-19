package agentboot

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
)

// Prompter is the consumer-supplied callback the bot layer (or any
// caller of [RunWithPrompter]) provides to satisfy approval and ask
// requests during agent execution.
//
// Contract:
//
//   - Timeout / cancellation. The caller's ctx carries the deadline.
//     On ctx.Done() the Prompter MUST return promptly with
//     Approved=false (i.e., default-deny). Implementations are free to
//     enforce an additional internal timeout (IMPrompter uses 5min by
//     default) but ctx is authoritative.
//
//   - AlwaysAllow caching. If the user previously approved a tool with
//     "remember", subsequent OnApproval calls for the same tool MUST
//     short-circuit to Approved=true without prompting again. The
//     Prompter owns this cache; executors do not maintain per-tool
//     allowlists.
//
//   - No partial failures. On internal error the Prompter returns the
//     error AND a deny result (Approved=false), so [RunWithPrompter]
//     always has a safe response to send to the agent.
//
// The Prompter consumes the stream event types ([ApprovalRequestEvent],
// [AskRequestEvent]) directly and returns the matching control responses,
// so there is no intermediate request/result representation.
//
// Implementation: IMPrompter (production).
type Prompter interface {
	OnApproval(ctx context.Context, req ApprovalRequestEvent) (ApprovalResponse, error)
	OnAsk(ctx context.Context, req AskRequestEvent) (AskResponse, error)
}

// MessageSink receives, in order, the [MessageEvent.Raw] value of each
// message event and any [ErrorEvent] the agent emits (passed as the ErrorEvent
// value itself). An ErrorEvent may be recoverable or may mirror the fatal error
// later returned by handle.Wait. Pass nil to drop these when only completion
// matters.
type MessageSink func(any)

// RunWithPrompter is the convenience consumer of an [ExecutionHandle].
//
// It iterates handle.Events() in order, dispatching:
//   - MessageEvent → sink (if non-nil)
//   - ApprovalRequestEvent → prompter.OnApproval, then handle.Respond
//   - AskRequestEvent → prompter.OnAsk, then handle.Respond
//   - ErrorEvent → logged and forwarded to sink (if non-nil)
//
// Approval/ask invocations are synchronous within the loop (matching the
// existing IMPrompter blocking semantics — Claude waits for a response
// before emitting more events, so back-pressure is not a concern).
//
// Once the channel closes, RunWithPrompter calls handle.Wait() and returns
// its result.
//
// Use this from executor code that does not need event-level visibility.
// Tests and executors that DO want fine-grained control should iterate
// handle.Events() directly.
func RunWithPrompter(ctx context.Context, h ExecutionHandle, prompter Prompter, sink MessageSink) (*Result, error) {
	for ev := range h.Events() {
		dispatchStreamEvent(ctx, ev, prompter, sink, h.Respond, "RunWithPrompter")
	}

	return h.Wait()
}

// dispatchStreamEvent handles the event kinds [RunWithPrompter] and
// [RunTurnWithPrompter] both dispatch identically — MessageEvent,
// ApprovalRequestEvent, AskRequestEvent, ErrorEvent — routing them to sink
// and prompter/respond. It reports whether ev was one of those shared
// kinds; RunTurnWithPrompter type-switches on the remainder itself for its
// own turn-boundary events (TurnCompleteEvent, SessionStateEvent), which
// have no RunWithPrompter counterpart since a one-shot handle's channel
// simply closes at completion instead of emitting one.
func dispatchStreamEvent(ctx context.Context, ev StreamEvent, prompter Prompter, sink MessageSink, respond func(reqID string, resp ControlResponse) error, logTag string) bool {
	switch e := ev.(type) {
	case MessageEvent:
		if sink != nil {
			sink(e.Raw)
		}

	case ApprovalRequestEvent:
		res, perr := prompter.OnApproval(ctx, e)
		if perr != nil {
			logrus.WithError(perr).Warnf("agentboot.%s: prompter.OnApproval error; denying", logTag)
			res = ApprovalResponse{Approved: false, Reason: perr.Error()}
		}
		if rerr := respond(e.ID, res); rerr != nil {
			logrus.WithError(rerr).Warnf("agentboot.%s: Respond error", logTag)
		}

	case AskRequestEvent:
		res, aerr := prompter.OnAsk(ctx, e)
		if aerr != nil {
			logrus.WithError(aerr).Warnf("agentboot.%s: prompter.OnAsk error; denying", logTag)
			res = AskResponse{Approved: false, Reason: aerr.Error()}
		}
		if rerr := respond(e.ID, res); rerr != nil {
			logrus.WithError(rerr).Warnf("agentboot.%s: Respond error", logTag)
		}

	case ErrorEvent:
		logrus.WithError(e.Err).Warnf("agentboot.%s: agent ErrorEvent", logTag)
		if sink != nil {
			sink(e)
		}

	default:
		return false
	}
	return true
}

// ErrSessionEventsClosedMidTurn means a [PersistentSession]'s Events()
// channel closed without ever producing a TurnCompleteEvent or a
// SessionStateEvent{State: SessionStateTerminated} — both of which
// [persistentSession.terminate] always emits before closing the channel, so
// this indicates a bug in the session implementation rather than a normal
// runtime outcome.
var ErrSessionEventsClosedMidTurn = errors.New("agentboot: persistent session events closed without a turn boundary")

// RunTurnWithPrompter is [RunWithPrompter]'s counterpart for a
// [PersistentSession]: it drains Events() through exactly one turn
// boundary — a TurnCompleteEvent, whose Result it returns — instead of
// waiting for the channel to close, since a persistent session's channel
// stays open across further turns.
//
// If the session terminates (a crash, or a fatal protocol error) before the
// turn completes, RunTurnWithPrompter returns the SessionStateEvent's
// reason as an error instead of a Result. Callers should treat this the
// same as any other execution failure and remove the session from
// whatever registry (e.g. an [github.com/tingly-dev/tingly-box/agentboot/pool.Pool])
// tracks it — see .design/claude-code.md §5.3/§5.4.
//
// ctx bounds the turn: unlike a one-shot [ExecutionHandle], a
// [PersistentSession]'s process is deliberately detached from any single
// caller's ctx (see [Runner.Open]'s doc comment) so that it survives past
// the call that opened it. That means nothing else stops a runaway or
// unwanted turn — ctx.Done() here is it. There is no way to interrupt just
// the in-flight turn without ending the process (the CLI's stream-json
// protocol has no documented per-turn interrupt), so on ctx.Done()
// RunTurnWithPrompter closes the whole session — the same effect ctx
// cancellation has on a one-shot Execute — and returns ctx.Err(). Callers
// driving a cancelable request (a timeout, a user "/stop") should expect
// the session to be gone afterward, not just this one turn.
//
// Dispatch for MessageEvent/ApprovalRequestEvent/AskRequestEvent/ErrorEvent
// mirrors RunWithPrompter exactly.
func RunTurnWithPrompter(ctx context.Context, session PersistentSession, prompter Prompter, sink MessageSink) (*Result, error) {
	events := session.Events()
	for {
		select {
		case <-ctx.Done():
			// ctx just fired, so it must not also be what bounds how long
			// we're willing to wait for the session to actually close.
			closeCtx, cancel := context.WithTimeout(context.Background(), SessionCloseTimeout)
			_ = session.Close(closeCtx)
			cancel()
			return nil, ctx.Err()

		case ev, ok := <-events:
			if !ok {
				return nil, ErrSessionEventsClosedMidTurn
			}
			if dispatchStreamEvent(ctx, ev, prompter, sink, session.Respond, "RunTurnWithPrompter") {
				continue
			}
			switch e := ev.(type) {
			case TurnCompleteEvent:
				if e.Result != nil && e.Result.Error != "" {
					return e.Result, errors.New(e.Result.Error)
				}
				return e.Result, nil

			case SessionStateEvent:
				if e.State == SessionStateTerminated {
					return nil, fmt.Errorf("agentboot: persistent session terminated before this turn completed: %s", e.Reason)
				}
			}
		}
	}
}
