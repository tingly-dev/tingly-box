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
