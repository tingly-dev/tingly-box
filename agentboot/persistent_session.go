package agentboot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/process"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// Sentinel errors returned by [PersistentSession.Send] / [PersistentSession.Respond].
var (
	// ErrTurnInFlight is returned by Send when a previous turn hasn't
	// reached its terminal event yet.
	ErrTurnInFlight = errors.New("agentboot: a turn is already in flight on this session")

	// ErrSessionClosed is returned by Send/Respond once Close has been
	// called or the underlying process has terminated.
	ErrSessionClosed = errors.New("agentboot: persistent session closed")
)

// PersistentSession is a long-lived, multi-turn counterpart to
// [ExecutionHandle]: one underlying agent process stays alive across
// multiple Send calls instead of exiting after a single turn. See
// .design/claude-code.md for the design this implements.
//
// Lifecycle:
//  1. [Runner.Open] starts the process, submits its prompt as the first
//     turn, and returns a PersistentSession already in the Running state.
//  2. Events() delivers MessageEvent/ApprovalRequestEvent/AskRequestEvent
//     for the in-flight turn, exactly like ExecutionHandle, followed by
//     exactly one TurnCompleteEvent when that turn's terminal result
//     arrives. The session returns to Idle — Events() stays open.
//  3. Send submits the next user turn from Idle, moving back to Running.
//  4. Close ends the session: stdin is closed, a grace period lets the
//     process exit on its own, then it is killed if it hasn't. A final
//     SessionStateEvent{Terminated} is emitted and Events() closes. An
//     unexpected process exit or protocol error reaches the same terminal
//     state and event without a Close call.
//
// Concurrency:
//   - Events() may be consumed by exactly one goroutine.
//   - Send / Respond / Close / Status are safe to call concurrently from
//     any goroutine. Send while a turn is in flight returns
//     ErrTurnInFlight; any call after Close/termination returns
//     ErrSessionClosed.
type PersistentSession interface {
	// Send submits the next user turn on the session's single running
	// process. Returns ErrTurnInFlight if a turn is already running, or
	// ErrSessionClosed once Close has been called or the process has
	// terminated.
	Send(ctx context.Context, prompt string) error

	// Events is the ordered, agent-neutral stream for the session's whole
	// lifetime: the [ExecutionHandle] StreamEvent set, plus
	// TurnCompleteEvent and SessionStateEvent.
	Events() <-chan StreamEvent

	// Respond answers a pending ApprovalRequestEvent/AskRequestEvent from
	// the in-flight turn, exactly like [ExecutionHandle.Respond].
	Respond(reqID string, resp ControlResponse) error

	// Status reports the session's current lifecycle state.
	Status() SessionState

	// Close asks the current turn (if any) to finish, then shuts the
	// underlying process down: close stdin, wait a grace period, then Kill
	// if it hasn't exited. Idempotent; safe to call from any state. Blocks
	// until the process has been reaped or ctx is canceled.
	Close(ctx context.Context) error
}

// persistentSession is the concrete PersistentSession returned by Runner.Open.
type persistentSession struct {
	agentType AgentType
	runCtx    context.Context
	cancel    context.CancelFunc

	events chan StreamEvent

	mu           sync.Mutex
	state        SessionState
	pendingInput map[string]map[string]any
	turnStart    time.Time
	turnEvents   []protocol.Event

	transport AgentTransport
	encoder   *protocol.Encoder
	proc      process.Handle

	shutdownGracePeriod time.Duration
	shutdownOnce        sync.Once
	shutdownWG          sync.WaitGroup

	terminateOnce sync.Once
	done          chan struct{}
}

func (s *persistentSession) Events() <-chan StreamEvent { return s.events }

func (s *persistentSession) Status() SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Send encodes and writes the next turn directly onto the shared encoder.
// The first turn is instead submitted by Runner.Open via the driver's
// InitialInput channel; Send handles every turn after that.
func (s *persistentSession) Send(_ context.Context, prompt string) error {
	s.mu.Lock()
	switch s.state {
	case SessionStateIdle:
		s.state = SessionStateRunning
		s.turnStart = time.Now()
		s.turnEvents = nil
	case SessionStateRunning:
		s.mu.Unlock()
		return ErrTurnInFlight
	default: // Closing or Terminated
		s.mu.Unlock()
		return ErrSessionClosed
	}
	s.mu.Unlock()

	msg := s.transport.EncodeUserMessage(prompt)
	if err := s.encoder.Encode(msg); err != nil {
		// The write failed (most likely the process already exited); let
		// the pump's terminate() path be the authority on the session's
		// final state instead of asserting Idle here.
		return fmt.Errorf("agentboot: send turn: %w", err)
	}
	return nil
}

func (s *persistentSession) Respond(reqID string, resp ControlResponse) error {
	s.mu.Lock()
	if s.state == SessionStateTerminated {
		s.mu.Unlock()
		return ErrSessionClosed
	}
	input, ok := s.pendingInput[reqID]
	if !ok {
		s.mu.Unlock()
		return ErrUnknownRequestID
	}
	delete(s.pendingInput, reqID)
	s.mu.Unlock()

	wire := s.transport.EncodeControlResponse(reqID, resp, input)
	if wire == nil {
		return errors.New("agentboot: transport produced nil control response")
	}
	return s.encoder.Encode(wire)
}

func (s *persistentSession) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.state != SessionStateTerminated {
		s.state = SessionStateClosing
	}
	s.mu.Unlock()

	s.shutdown()

	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// shutdown closes stdin and arms the grace-period Kill escalation, exactly
// like Runner.Execute's shutdownGracefully. Idempotent — safe to call from
// Close and from terminate (an unprompted process exit still needs the
// input side closed/reaped).
func (s *persistentSession) shutdown() {
	s.shutdownOnce.Do(func() {
		s.shutdownWG.Add(2)
		go func() {
			defer s.shutdownWG.Done()
			_ = s.encoder.Close()
		}()
		go func() {
			defer s.shutdownWG.Done()
			timer := time.NewTimer(s.shutdownGracePeriod)
			defer timer.Stop()
			select {
			case <-s.proc.Done():
			case <-timer.C:
				_ = s.proc.Kill()
			}
		}()
	})
}

// completeTurn closes out the in-flight turn: builds its scoped Result,
// returns the session to Idle, and emits TurnCompleteEvent. Called from the
// pump goroutine only.
func (s *persistentSession) completeTurn(turnErr *ResultError) {
	s.mu.Lock()
	events := append([]protocol.Event(nil), s.turnEvents...)
	duration := time.Since(s.turnStart)
	s.turnEvents = nil
	if s.state == SessionStateRunning {
		s.state = SessionStateIdle
	}
	s.mu.Unlock()

	result := &Result{
		Format:   OutputFormatStreamJSON,
		Events:   events,
		Duration: duration,
		Metadata: map[string]any{},
	}
	if turnErr != nil {
		result.Error = turnErr.Error()
	}
	s.emit(TurnCompleteEvent{Result: result})
}

// emit pushes a stream event, registering/cleaning up control-request
// pending state exactly like runnerHandle.emit — see its doc comment.
func (s *persistentSession) emit(ev StreamEvent) {
	var pendingID string
	switch e := ev.(type) {
	case ApprovalRequestEvent:
		pendingID = e.ID
	case AskRequestEvent:
		pendingID = e.ID
	}

	select {
	case s.events <- ev:
	case <-s.runCtx.Done():
		if pendingID != "" {
			s.mu.Lock()
			delete(s.pendingInput, pendingID)
			s.mu.Unlock()
		}
	}
}

// pump is the session's single reader goroutine: it classifies every
// decoded event for the process's whole lifetime (not just one turn) and
// drives turn/session state accordingly. It runs until decoderEvents
// closes, then finalizes via terminate.
func (s *persistentSession) pump(decoderEvents <-chan protocol.Event, decoderErr func() error) {
	for ev := range decoderEvents {
		s.mu.Lock()
		s.turnEvents = append(s.turnEvents, ev)
		s.mu.Unlock()

		kind, parsed := s.transport.Classify(ev)

		switch kind {
		case EventKindIgnore:
			// Drop.

		case EventKindMessage:
			for _, m := range s.transport.AccumulateMessage(ev) {
				s.emit(MessageEvent{Raw: m})
			}

		case EventKindControl:
			if parsed != nil {
				var reqID string
				var input map[string]any
				switch p := parsed.(type) {
				case ApprovalRequestEvent:
					reqID, input = p.ID, p.Input
				case AskRequestEvent:
					reqID, input = p.ID, p.Input
				}
				if reqID != "" {
					s.mu.Lock()
					s.pendingInput[reqID] = input
					s.mu.Unlock()
				}
				s.emit(parsed)
			}

		case EventKindTerminalSuccess:
			s.completeTurn(nil)

		case EventKindTerminalError:
			s.completeTurn(resultErrorFromEvent(s.agentType, ev))
		}
	}

	s.terminate(decoderErr())
}

// terminate is the session's single finalization path, reached whether the
// process exited because of a Close call, a crash, or a fatal protocol
// error. Idempotent.
func (s *persistentSession) terminate(streamErr error) {
	s.terminateOnce.Do(func() {
		fatalDecode := streamErr != nil &&
			!errors.Is(streamErr, context.Canceled) &&
			!errors.Is(streamErr, context.DeadlineExceeded)
		if fatalDecode {
			_ = s.proc.Kill()
		}

		s.shutdown()
		waitErr := s.proc.Wait()
		s.shutdownWG.Wait()

		reason := "closed"
		switch {
		case fatalDecode:
			reason = fmt.Sprintf("protocol error: %v", streamErr)
		case waitErr != nil:
			reason = fmt.Sprintf("process exited: %v", waitErr)
		}

		s.mu.Lock()
		s.state = SessionStateTerminated
		s.mu.Unlock()

		// Emit the terminal event, then close, while runCtx is still live —
		// emit's cancellation-safety select would otherwise race the send
		// against runCtx.Done() and could drop this event. cancel() only
		// releases decode/kill-watcher goroutines that are already inert by
		// this point (the process has exited), so deferring it here costs
		// nothing.
		s.emit(SessionStateEvent{State: SessionStateTerminated, Reason: reason})
		close(s.events)
		close(s.done)
		s.cancel()
	})
}
