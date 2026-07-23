package agentboot_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

func TestRunner_AppliesConfiguredBufferAndDefaultTimeout(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(ctx context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			_, _ = io.Copy(io.Discard, h.StdinR)
		}()
		go func() {
			<-ctx.Done()
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	agent := claude.NewAgentWithFactory(claude.Config{
		StreamBufferSize:        3,
		DefaultExecutionTimeout: 40 * time.Millisecond,
	}, factory)

	parentCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	handle, err := agent.Execute(parentCtx, "timeout", agentboot.ExecutionOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, cap(handle.Events()))

	for range handle.Events() {
	}
	result, waitErr := handle.Wait()
	require.ErrorIs(t, waitErr, context.DeadlineExceeded)
	require.NotNil(t, result)
	assert.Equal(t, waitErr.Error(), result.Error)
	assert.Less(t, time.Since(started), 500*time.Millisecond)
}
