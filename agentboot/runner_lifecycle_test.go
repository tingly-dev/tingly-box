package agentboot_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/claude/fixture"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

func TestRunner_PropagatesProcessExitError(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			_, _ = io.Copy(io.Discard, h.StdinR)
		}()
		go func() {
			h.FinishOutput()
			h.SignalExit(testExitError{code: 23})
			_ = h.Kill()
		}()
	}
	agent := claude.NewAgentWithFactory(claude.Config{}, factory)

	handle, err := agent.Execute(context.Background(), "fail", agentboot.ExecutionOptions{})
	require.NoError(t, err)
	var streamedError error
	for event := range handle.Events() {
		if errorEvent, ok := event.(agentboot.ErrorEvent); ok {
			streamedError = errorEvent.Err
		}
	}
	result, waitErr := handle.Wait()

	var processErr *agentboot.ProcessError
	require.ErrorAs(t, waitErr, &processErr)
	assert.Equal(t, 23, processErr.ExitCode)
	assert.Equal(t, 23, result.ExitCode)
	assert.Equal(t, waitErr.Error(), result.Error)
	require.ErrorAs(t, streamedError, &processErr)
}

func TestRunner_PropagatesProtocolDecodeError(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			_, _ = io.Copy(io.Discard, h.StdinR)
		}()
		go func() {
			_, _ = h.WriteOutput([]byte("{not-json}\n"))
			h.FinishOutput()
			h.SignalExit(nil)
			_ = h.Kill()
		}()
	}
	agent := claude.NewAgentWithFactory(claude.Config{}, factory)

	handle, err := agent.Execute(context.Background(), "decode", agentboot.ExecutionOptions{})
	require.NoError(t, err)
	var streamedError error
	for event := range handle.Events() {
		if errorEvent, ok := event.(agentboot.ErrorEvent); ok {
			streamedError = errorEvent.Err
		}
	}
	result, waitErr := handle.Wait()

	var protocolErr *agentboot.ProtocolError
	require.ErrorAs(t, waitErr, &protocolErr)
	assert.False(t, errors.Is(waitErr, agentboot.ErrNoTerminalResult))
	assert.Equal(t, waitErr.Error(), result.Error)
	require.ErrorAs(t, streamedError, &protocolErr)
}

func TestRunner_RejectsStreamWithoutTerminalResult(t *testing.T) {
	agent := claude.NewAgentWithFactory(claude.Config{}, fixture.Factory(fixture.Script{
		fixture.AssistantText("partial"),
	}))

	handle, err := agent.Execute(context.Background(), "missing result", agentboot.ExecutionOptions{})
	require.NoError(t, err)
	for range handle.Events() {
	}
	result, waitErr := handle.Wait()

	require.ErrorIs(t, waitErr, agentboot.ErrNoTerminalResult)
	var protocolErr *agentboot.ProtocolError
	require.ErrorAs(t, waitErr, &protocolErr)
	assert.Equal(t, waitErr.Error(), result.Error)
}

func TestRunner_PreservesStructuredResultError(t *testing.T) {
	agent := claude.NewAgentWithFactory(claude.Config{}, fixture.Factory(fixture.Script{
		fixture.Raw(map[string]any{
			"type":     claude.SDKResultMessage,
			"subtype":  claude.ResultSubtypeErrorMaxTurns,
			"is_error": true,
			"errors":   []string{"maximum turns reached", "raise max_turns"},
		}),
	}))

	handle, err := agent.Execute(context.Background(), "run", agentboot.ExecutionOptions{})
	require.NoError(t, err)
	for range handle.Events() {
	}
	result, waitErr := handle.Wait()

	var resultErr *agentboot.ResultError
	require.ErrorAs(t, waitErr, &resultErr)
	assert.Equal(t, claude.ResultSubtypeErrorMaxTurns, resultErr.Subtype)
	assert.Equal(t, []string{"maximum turns reached", "raise max_turns"}, resultErr.Details)
	assert.True(t, strings.Contains(result.Error, "maximum turns reached"))
	assert.False(t, result.IsSuccess())
}

type testExitError struct {
	code int
}

func (e testExitError) Error() string { return "test process failed" }
func (e testExitError) ExitCode() int { return e.code }
