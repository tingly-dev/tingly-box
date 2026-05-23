package agentboot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
)

const fakeAgentType agentboot.AgentType = "fake"

// fakeAgent returns a controlled handle that echoes the prompt as a single
// MessageEvent and completes with a fixed Result. It records the inputs it was
// called with so tests can assert the service threaded them through.
type fakeAgent struct {
	result     *agentboot.Result
	execErr    error
	gotPrompt  string
	gotProject string
}

func (a *fakeAgent) Execute(_ context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	a.gotPrompt = prompt
	a.gotProject = opts.ProjectPath
	if a.execErr != nil {
		return nil, a.execErr
	}
	res := a.result
	h, ctrl := agentboot.NewControlledHandle(
		4,
		func(string, agentboot.ControlResponse) error { return nil },
		func() {},
		func() (*agentboot.Result, error) { return res, nil },
	)
	go func() {
		ctrl.Emit(context.Background(), agentboot.MessageEvent{Raw: prompt})
		ctrl.Close()
	}()
	return h, nil
}

func (a *fakeAgent) IsAvailable() bool                         { return true }
func (a *fakeAgent) Type() agentboot.AgentType                 { return fakeAgentType }
func (a *fakeAgent) SetDefaultFormat(_ agentboot.OutputFormat) {}
func (a *fakeAgent) GetDefaultFormat() agentboot.OutputFormat {
	return agentboot.OutputFormatStreamJSON
}

func newFakeService(t *testing.T, agent agentboot.Agent) *agentboot.AgentService {
	t.Helper()
	svc, err := agentboot.NewAgentService(agentboot.Config{DefaultAgent: fakeAgentType})
	require.NoError(t, err)
	if agent != nil {
		svc.RegisterAgent(fakeAgentType, agent)
	}
	return svc
}

func TestAgentService_Run_DrivesHandleToCompletion(t *testing.T) {
	want := &agentboot.Result{Output: "hi", ExitCode: 0}
	agent := &fakeAgent{result: want}
	svc := newFakeService(t, agent)

	var sink []any
	got, err := svc.Run(context.Background(), agentboot.RunRequest{
		ProjectPath: "/tmp/proj",
		Prompt:      "echo me",
	}, &capturePrompter{approve: true}, func(raw any) { sink = append(sink, raw) })

	require.NoError(t, err)
	assert.Same(t, want, got)

	// Service threaded prompt + project path into the agent.
	assert.Equal(t, "echo me", agent.gotPrompt)
	assert.Equal(t, "/tmp/proj", agent.gotProject)

	// And the streamed message reached the sink.
	require.Len(t, sink, 1)
	assert.Equal(t, "echo me", sink[0])
}

func TestAgentService_Run_NoAgentRegistered_Errors(t *testing.T) {
	svc := newFakeService(t, nil) // default agent type not registered

	_, err := svc.Run(context.Background(), agentboot.RunRequest{Prompt: "hi"},
		&capturePrompter{}, nil)
	require.Error(t, err, "resolveAgent failure must surface before any run")
}

func TestAgentService_Run_ExecuteError_Propagates(t *testing.T) {
	agent := &fakeAgent{execErr: errors.New("cli missing")}
	svc := newFakeService(t, agent)

	_, err := svc.Run(context.Background(), agentboot.RunRequest{Prompt: "hi"},
		&capturePrompter{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cli missing")
}
