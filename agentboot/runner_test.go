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
// AgentService registry. It never actually runs.
type stubAgent struct {
	t agentboot.AgentType
}

func (s *stubAgent) Execute(_ context.Context, _ string, _ agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	return nil, errors.New("stubAgent: Execute not supported")
}

func (s *stubAgent) IsAvailable() bool         { return true }
func (s *stubAgent) Type() agentboot.AgentType { return s.t }

const stubAgentType agentboot.AgentType = "stub"

func newStubAgent() *stubAgent { return &stubAgent{t: stubAgentType} }

// --- AgentService registry tests -------------------------------------------

func newRegistryService(t *testing.T) *agentboot.AgentService {
	t.Helper()
	svc, err := agentboot.NewAgentService(agentboot.Config{})
	require.NoError(t, err)
	return svc
}

func TestAgentService_RegisterAndList(t *testing.T) {
	svc := newRegistryService(t)

	svc.RegisterAgent(stubAgentType, newStubAgent())

	assert.Contains(t, svc.RegisteredAgents(), stubAgentType)
}

func TestAgentService_ExecuteUnregistered(t *testing.T) {
	svc := newRegistryService(t)

	_, err := svc.Execute(context.Background(), "nonexistent", "/tmp", "hi", agentboot.ExecutionOptions{})
	assert.Error(t, err)
}

func TestAgentService_SetDefaultAgent(t *testing.T) {
	svc := newRegistryService(t)

	svc.RegisterAgent(stubAgentType, newStubAgent())

	require.NoError(t, svc.SetDefaultAgent(stubAgentType))
	assert.Equal(t, stubAgentType, svc.Config().DefaultAgent)
}

func TestAgentService_SetDefaultAgent_Unregistered(t *testing.T) {
	svc := newRegistryService(t)

	err := svc.SetDefaultAgent("ghost")
	assert.Error(t, err)
}

func TestAgentService_DefaultAgent_ConcurrentWithSetter(t *testing.T) {
	svc := newRegistryService(t)

	const alternateType agentboot.AgentType = "alternate"
	svc.RegisterAgent(stubAgentType, newStubAgent())
	svc.RegisterAgent(alternateType, &stubAgent{t: alternateType})
	require.NoError(t, svc.SetDefaultAgent(stubAgentType))

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
			if setErr := svc.SetDefaultAgent(next); setErr != nil {
				errs <- setErr
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			got := svc.Config().DefaultAgent
			if got != stubAgentType && got != alternateType {
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
