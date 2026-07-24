package agentboot_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// stubAgent is a minimal Agent implementation for tests that exercise the
// AgentBoot registry. It never actually runs.
type stubAgent struct {
	t agentboot.AgentType
}

func (s *stubAgent) Execute(_ context.Context, _ string, _ agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	return nil, errors.New("stubAgent: Execute not supported")
}

func (s *stubAgent) IsAvailable() bool                         { return true }
func (s *stubAgent) Type() agentboot.AgentType                 { return s.t }
func (s *stubAgent) SetDefaultFormat(_ agentboot.OutputFormat) {}
func (s *stubAgent) GetDefaultFormat() agentboot.OutputFormat {
	return agentboot.OutputFormatStreamJSON
}

const stubAgentType agentboot.AgentType = "stub"

func newStubAgent() *stubAgent { return &stubAgent{t: stubAgentType} }

// --- AgentBoot registry tests ----------------------------------------------

func TestAgentBoot_RegisterAndGet(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	ab.RegisterAgent(stubAgentType, newStubAgent())

	got, err := ab.GetAgent(stubAgentType)
	require.NoError(t, err)
	assert.Equal(t, stubAgentType, got.Type())
}

func TestAgentBoot_GetUnregistered(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	_, err = ab.GetAgent("nonexistent")
	assert.Error(t, err)
}

func TestAgentBoot_ResumeSession(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	opts := ab.ResumeSession("sess-abc")
	assert.Equal(t, "sess-abc", opts.SessionID)
	assert.True(t, opts.Resume)
}

func TestAgentBoot_SetDefaultAgent(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	ab.RegisterAgent(stubAgentType, newStubAgent())

	require.NoError(t, ab.SetDefaultAgent(stubAgentType))

	got, err := ab.GetDefaultAgent()
	require.NoError(t, err)
	assert.Equal(t, stubAgentType, got.Type())
}

func TestAgentBoot_SetDefaultAgent_Unregistered(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	err = ab.SetDefaultAgent("ghost")
	assert.Error(t, err)
}

func TestAgentBoot_GetDefaultAgent_ConcurrentWithSetter(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	const alternateType agentboot.AgentType = "alternate"
	ab.RegisterAgent(stubAgentType, newStubAgent())
	ab.RegisterAgent(alternateType, &stubAgent{t: alternateType})
	require.NoError(t, ab.SetDefaultAgent(stubAgentType))

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			next := stubAgentType
			if i%2 == 0 {
				next = alternateType
			}
			if setErr := ab.SetDefaultAgent(next); setErr != nil {
				errs <- setErr
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			got, getErr := ab.GetDefaultAgent()
			if getErr != nil {
				errs <- getErr
				return
			}
			if got.Type() != stubAgentType && got.Type() != alternateType {
				errs <- errors.New("unexpected default agent")
				return
			}
		}
	}()
	wg.Wait()
	close(errs)

	for concurrentErr := range errs {
		require.NoError(t, concurrentErr)
	}
}
