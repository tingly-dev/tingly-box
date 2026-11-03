package agentboot_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// stepScript is an agent whose successive Execute calls run scripted steps:
// each step either emits output before failing, fails pre-output (no emit),
// or succeeds.
type scriptedStep struct {
	emit   bool  // emit one message before Wait's outcome
	err    error // Wait's error; nil means success
	result *agentboot.Result
}

type scriptAgent struct {
	steps    []scriptedStep
	attempts int
}

func (a *scriptAgent) Execute(_ context.Context, _ string, _ agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	i := a.attempts
	a.attempts++
	if i >= len(a.steps) {
		i = len(a.steps) - 1
	}
	step := a.steps[i]

	h, ctrl := agentboot.NewControlledHandle(
		4,
		func(string, agentboot.ControlResponse) error { return nil },
		func() {},
		func() (*agentboot.Result, error) { return step.result, step.err },
	)
	go func() {
		if step.emit {
			ctrl.Emit(context.Background(), agentboot.MessageEvent{Raw: "some output"})
		}
		ctrl.Close()
	}()
	return h, nil
}

func (a *scriptAgent) IsAvailable() bool         { return true }
func (a *scriptAgent) Type() agentboot.AgentType { return fakeAgentType }

// runScript runs svc.Run against the scripted agent and reports the error
// and how many attempts Execute took.
func runScript(t *testing.T, svc *agentboot.AgentService) error {
	t.Helper()
	_, err := svc.Run(context.Background(), agentboot.RunRequest{Prompt: "hi"}, &capturePrompter{}, nil)
	return err
}

func newScriptService(t *testing.T, agent agentboot.Agent) *agentboot.AgentService {
	t.Helper()
	svc, err := agentboot.NewAgentService(agentboot.Config{DefaultAgent: fakeAgentType})
	require.NoError(t, err)
	svc.RegisterAgent(fakeAgentType, agent)
	return svc
}

func TestAgentService_Run_RetriesPreOutputFailure(t *testing.T) {
	// Attempt 1 crashes before any output (empty Events + ProcessError);
	// attempt 2 succeeds.
	agent := &scriptAgent{steps: []scriptedStep{
		{err: &agentboot.ProcessError{AgentType: fakeAgentType, ExitCode: 1}, result: &agentboot.Result{}},
		{result: &agentboot.Result{Output: "ok"}},
	}}
	svc := newScriptService(t, agent)

	err := runScript(t, svc)
	require.NoError(t, err, "a pre-output crash must be retried into a success")
	assert.Equal(t, 2, agent.attempts)
}

func TestAgentService_Run_RetriesLaunchFailure(t *testing.T) {
	// Attempt 1 fails synchronously at process start (wrapped ProcessError);
	// attempt 2 succeeds.
	agent := &scriptAgent{steps: []scriptedStep{{result: &agentboot.Result{Output: "ok"}}}}
	svc := newScriptService(t, &launchOnceAgent{
		inner:   agent,
		onFirst: &agentboot.ProcessError{AgentType: fakeAgentType, ExitCode: -1},
		calls:   new(int),
	})

	err := runScript(t, svc)
	require.NoError(t, err, "a launch failure must be retried")
}

type launchOnceAgent struct {
	inner   *scriptAgent
	onFirst error
	calls   *int
}

func (a *launchOnceAgent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	*a.calls++
	if *a.calls == 1 {
		return nil, a.onFirst
	}
	return a.inner.Execute(ctx, prompt, opts)
}
func (a *launchOnceAgent) IsAvailable() bool         { return true }
func (a *launchOnceAgent) Type() agentboot.AgentType { return fakeAgentType }

func TestAgentService_Run_NoRetryAfterOutput(t *testing.T) {
	// Attempt 1 emitted a message before crashing: Events non-empty, so the
	// failure must surface as-is — no retry, no double execution risk.
	agent := &scriptAgent{steps: []scriptedStep{
		{emit: true, err: &agentboot.ProcessError{AgentType: fakeAgentType, ExitCode: 1},
			result: &agentboot.Result{Events: []protocol.Event{{Type: "assistant"}}}},
	}}
	svc := newScriptService(t, agent)

	err := runScript(t, svc)
	require.Error(t, err, "a post-output crash must NOT be retried")
	assert.Equal(t, 1, agent.attempts)
}

func TestAgentService_Run_NoRetryOnResultError(t *testing.T) {
	// The agent completed the turn with an error result: never retried.
	agent := &scriptAgent{steps: []scriptedStep{
		{err: &agentboot.ResultError{AgentType: fakeAgentType, Subtype: "error_during_execution"},
			result: &agentboot.Result{}},
	}}
	svc := newScriptService(t, agent)

	err := runScript(t, svc)
	require.Error(t, err)
	assert.Equal(t, 1, agent.attempts)
}

func TestAgentService_Run_NoRetryOnPlainExecuteError(t *testing.T) {
	// A synchronous non-process failure (CLI missing, bad spec) is a config
	// problem, not a transient flake: no retry.
	agent := &scriptAgent{steps: []scriptedStep{{result: &agentboot.Result{Output: "ok"}}}}
	wrapped := &launchOnceAgent{inner: agent, onFirst: errors.New("agent CLI not available"), calls: new(int)}
	svc := newScriptService(t, wrapped)

	err := runScript(t, svc)
	require.Error(t, err)
	assert.Equal(t, 1, *wrapped.calls, "a non-process launch failure must NOT be retried")
}

func TestAgentService_Run_NoRetryOnCtxCanceled(t *testing.T) {
	agent := &scriptAgent{steps: []scriptedStep{
		{err: context.Canceled, result: &agentboot.Result{}},
	}}
	svc := newScriptService(t, agent)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.Run(ctx, agentboot.RunRequest{Prompt: "hi"}, &capturePrompter{}, nil)
	require.Error(t, err)
	assert.Equal(t, 1, agent.attempts)
}

func TestPreOutputFailure(t *testing.T) {
	assert.False(t, agentboot.PreOutputFailure(nil))
	assert.False(t, agentboot.PreOutputFailure(context.Canceled))
	assert.False(t, agentboot.PreOutputFailure(context.DeadlineExceeded))
	assert.False(t, agentboot.PreOutputFailure(errors.New("agent CLI not available")))
	assert.False(t, agentboot.PreOutputFailure(&agentboot.ResultError{AgentType: fakeAgentType}))
	assert.False(t, agentboot.PreOutputFailure(&agentboot.ProtocolError{AgentType: fakeAgentType, Err: errors.New("bad json")}))

	assert.True(t, agentboot.PreOutputFailure(&agentboot.ProcessError{AgentType: fakeAgentType, ExitCode: 1}))
	assert.True(t, agentboot.PreOutputFailure(&agentboot.ProtocolError{AgentType: fakeAgentType, Err: agentboot.ErrNoTerminalResult}))
	assert.True(t, agentboot.PreOutputFailure(&agentboot.SessionTerminatedError{Reason: "process exited: exit status 1", ProcessFailure: true}))
	assert.False(t, agentboot.PreOutputFailure(&agentboot.SessionTerminatedError{Reason: "closed"}))
}

func TestRetryConfig_Defaults(t *testing.T) {
	def := agentboot.DefaultRetryConfig()
	assert.Equal(t, 2, def.MaxRetries)
	assert.Equal(t, 250*time.Millisecond, def.BaseBackoff)
	assert.Equal(t, 2*time.Second, def.MaxBackoff)
	assert.Equal(t, def, agentboot.DefaultConfig().Retry)
}
