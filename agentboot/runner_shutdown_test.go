package agentboot_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

func TestRunner_ClosesInputForGracefulExitAfterResult(t *testing.T) {
	factory := process.NewFakeFactory()
	sawInputEOF := make(chan struct{})
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			decoder := json.NewDecoder(h.StdinR)
			var initial map[string]any
			if err := decoder.Decode(&initial); err != nil {
				h.FinishOutput()
				h.SignalExit(err)
				return
			}

			result, _ := json.Marshal(map[string]any{
				"type":     claude.SDKResultMessage,
				"subtype":  claude.ResultSubtypeSuccess,
				"is_error": false,
			})
			_, _ = h.WriteOutput(append(result, '\n'))

			var next map[string]any
			if err := decoder.Decode(&next); errors.Is(err, io.EOF) {
				close(sawInputEOF)
			}
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	agent := claude.NewAgentWithFactory(claude.Config{}, factory)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	handle, err := agent.Execute(ctx, "flush", agentboot.ExecutionOptions{})
	require.NoError(t, err)
	for range handle.Events() {
	}
	result, waitErr := handle.Wait()

	require.NoError(t, waitErr)
	assert.True(t, result.IsSuccess())
	select {
	case <-sawInputEOF:
	default:
		t.Fatal("agent process did not observe stdin EOF before completion")
	}
}

func TestRunner_TerminalSuccessSurvivesShutdownEscalation(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			_, _ = io.Copy(io.Discard, h.StdinR)
		}()
		go func() {
			result, _ := json.Marshal(map[string]any{
				"type":     claude.SDKResultMessage,
				"subtype":  claude.ResultSubtypeSuccess,
				"is_error": false,
			})
			_, _ = h.WriteOutput(append(result, '\n'))
			// Deliberately leave stdout and the process open. Runner must
			// escalate after the grace period without changing the result.
		}()
	}

	driver := claude.NewDriver(claude.Config{})
	driver.SetForceAvailable(true)
	driver.SetCLIPath("claude-fixture-binary")
	runner := agentboot.NewRunnerWithFactoryAndConfig(
		driver,
		func() agentboot.AgentTransport { return claude.NewTransport() },
		factory,
		agentboot.RunnerConfig{
			DefaultFormat:       agentboot.OutputFormatStreamJSON,
			ShutdownGracePeriod: 10 * time.Millisecond,
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	handle, err := runner.Execute(ctx, "flush timeout", agentboot.ExecutionOptions{})
	require.NoError(t, err)

	var streamedError error
	for event := range handle.Events() {
		if errorEvent, ok := event.(agentboot.ErrorEvent); ok {
			streamedError = errorEvent.Err
		}
	}
	result, waitErr := handle.Wait()

	require.NoError(t, waitErr)
	assert.NoError(t, streamedError)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.IsSuccess())
}
