package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
)

// fakePersistentAgent opens scripted sessions: the Nth Open runs scripts[N].
// It exercises the engine's pre-output retry of a first turn whose process
// crashes before producing anything.
type fakePersistentAgent struct {
	mu      sync.Mutex
	scripts []fakePersistentScript
	opens   int
	// openPrompts/openOpts record every Open call, in order.
	openPrompts []string
	openOpts    []agentboot.ExecutionOptions
}

// fakePersistentScript describes one opened session's single turn: complete
// with a message, crash before emitting anything, or emit a message and then
// crash mid-turn.
type fakePersistentScript struct {
	crashBeforeOutput bool
	crashAfterOutput  bool
}

func (a *fakePersistentAgent) Type() agentboot.AgentType { return agentboot.AgentTypeClaude }
func (a *fakePersistentAgent) IsAvailable() bool         { return true }

func (a *fakePersistentAgent) Execute(context.Context, string, agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	return nil, errors.New("fakePersistentAgent: one-shot Execute not scripted")
}

func (a *fakePersistentAgent) Open(_ context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.PersistentSession, error) {
	a.mu.Lock()
	i := a.opens
	a.opens++
	a.openPrompts = append(a.openPrompts, prompt)
	a.openOpts = append(a.openOpts, opts)
	script := fakePersistentScript{}
	if i < len(a.scripts) {
		script = a.scripts[i]
	}
	a.mu.Unlock()
	return newFakePersistentSession(script), nil
}

// fakePersistentSession is a minimal scripted PersistentSession: one turn
// that either completes with a message or terminates the session.
type fakePersistentSession struct {
	script fakePersistentScript

	mu            sync.Mutex
	state         agentboot.SessionState
	once          sync.Once
	outputEmitted bool
}

func newFakePersistentSession(script fakePersistentScript) *fakePersistentSession {
	return &fakePersistentSession{
		script: script,
		state:  agentboot.SessionStateRunning,
	}
}

func (s *fakePersistentSession) setState(st agentboot.SessionState) {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
}

// Events drives the scripted turn for the session's lifetime and closes the
// channel when the session terminates; a completing session's channel stays
// open, like the real one's.
func (s *fakePersistentSession) Events() <-chan agentboot.StreamEvent {
	events := make(chan agentboot.StreamEvent, 16)
	go func() {
		s.once.Do(func() {
			defer close(events)
			if s.script.crashBeforeOutput {
				s.terminate(events)
				return
			}
			events <- agentboot.MessageEvent{Raw: "turn output"}
			s.mu.Lock()
			s.outputEmitted = true
			s.mu.Unlock()
			if s.script.crashAfterOutput {
				s.terminate(events)
				return
			}
			events <- agentboot.TurnCompleteEvent{Result: &agentboot.Result{Output: "ok"}}
			s.setState(agentboot.SessionStateIdle)
		})
	}()
	return events
}

func (s *fakePersistentSession) terminate(events chan<- agentboot.StreamEvent) {
	s.setState(agentboot.SessionStateTerminated)
	events <- agentboot.SessionStateEvent{State: agentboot.SessionStateTerminated, Reason: "process exited: exit status 1"}
}

func (s *fakePersistentSession) Status() agentboot.SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// TurnOutputSeen mirrors persistentSession.TurnOutputSeen so the Conductor's
// reporter assertion finds the fake too.
func (s *fakePersistentSession) TurnOutputSeen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outputEmitted
}

func (s *fakePersistentSession) Send(context.Context, string) error              { return nil }
func (s *fakePersistentSession) Inject(context.Context, string) error            { return nil }
func (s *fakePersistentSession) SetModel(context.Context, string) error          { return nil }
func (s *fakePersistentSession) SetPermissionMode(context.Context, string) error { return nil }
func (s *fakePersistentSession) Respond(string, agentboot.ControlResponse) error { return nil }
func (s *fakePersistentSession) Interrupt(context.Context) error                 { return nil }
func (s *fakePersistentSession) StopTask(context.Context, string) error          { return nil }
func (s *fakePersistentSession) Close(context.Context) error                     { return nil }

func newPersistentRetryEngine(t *testing.T, agent *fakePersistentAgent) (*Engine, chan TurnEnd) {
	t.Helper()
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	require.NoError(t, err)
	svc.RegisterAgent(agentboot.AgentTypeClaude, agent)
	endCh := make(chan TurnEnd, 1)
	e := New(Config{
		Agent: svc,
		Store: newMemStore(),
		Pool:  pool.New(pool.Config{MaxSessions: 4, IdleTimeout: time.Minute}),
		OnTurnEnd: func(id string, end TurnEnd) {
			endCh <- end
		},
	})
	t.Cleanup(func() { e.Shutdown(context.Background()) })
	return e, endCh
}

func TestRunPersistentTurn_RetriesPreOutputCrash(t *testing.T) {
	agent := &fakePersistentAgent{scripts: []fakePersistentScript{
		{crashBeforeOutput: true},
		{}, // relaunch completes the turn
	}}
	e, endCh := newPersistentRetryEngine(t, agent)

	require.True(t, e.Start("s1", t.TempDir(), "hello", false, nil))
	end := <-endCh
	require.NoError(t, end.Err, "a pre-output crash on the first turn must be retried into a success")
	assert.Equal(t, 2, agent.opens, "the engine must relaunch exactly once")
}

func TestRunPersistentTurn_RecoversInterruptedTurn(t *testing.T) {
	// Open #1 crashes mid-turn (output seen — the interrupted turn reached
	// the transcript); the recovery relaunch resumes the same Claude session
	// and the CLI replays the turn from its transcript.
	agent := &fakePersistentAgent{scripts: []fakePersistentScript{
		{crashAfterOutput: true},
		{}, // the recovery relaunch completes the turn
	}}
	e, endCh := newPersistentRetryEngine(t, agent)

	require.True(t, e.Start("s1", t.TempDir(), "hello", false, nil))
	end := <-endCh
	require.NoError(t, end.Err, "a mid-turn crash must be recovered by resuming")
	assert.Equal(t, 2, agent.opens, "exactly one recovery relaunch")

	agent.mu.Lock()
	defer agent.mu.Unlock()
	assert.Equal(t, "", agent.openPrompts[1],
		"the recovery launch must not inject a message — the CLI replays the interrupted turn")
	assert.True(t, agent.openOpts[1].Resume, "the recovery launch must resume the session")
	foundEnv := false
	for _, kv := range agent.openOpts[1].Env {
		if kv == "CLAUDE_CODE_RESUME_INTERRUPTED_TURN=1" {
			foundEnv = true
		}
	}
	assert.True(t, foundEnv, "the recovery launch must set the resume-interrupted-turn env")
}

func TestRunPersistentTurn_RecoveryFailureSurfaces(t *testing.T) {
	// Both the original process and the recovery relaunch crash mid-turn:
	// one recovery attempt only, then the error surfaces.
	agent := &fakePersistentAgent{scripts: []fakePersistentScript{
		{crashAfterOutput: true},
		{crashAfterOutput: true},
	}}
	e, endCh := newPersistentRetryEngine(t, agent)

	require.True(t, e.Start("s1", t.TempDir(), "hello", false, nil))
	end := <-endCh
	require.Error(t, end.Err, "a recovery that also fails must surface")
	assert.Equal(t, 2, agent.opens, "exactly one recovery attempt — no loop")
}
