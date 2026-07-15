package codex

import (
	"context"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

func TestAgent_CollectsCodexJSONL(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, handle *process.FakeHandle) {
		go func() {
			lines := []string{
				`{"type":"thread.started","thread_id":"thread-1"}`,
				`{"type":"turn.started"}`,
				`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"finished"}}`,
				`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`,
			}
			for _, line := range lines {
				if _, err := handle.WriteOutput([]byte(line + "\n")); err != nil {
					return
				}
			}
			handle.FinishOutput()
			handle.SignalExit(nil)
		}()
	}

	agent := NewAgentWithFactory(DefaultConfig(), factory)
	handle, err := agent.Execute(context.Background(), "work", agentboot.ExecutionOptions{
		ProjectPath:  t.TempDir(),
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawThread bool
	for event := range handle.Events() {
		if message, ok := event.(agentboot.MessageEvent); ok {
			if raw, ok := message.Raw.(Message); ok && raw.GetSessionID() == "thread-1" {
				sawThread = true
			}
		}
	}
	result, err := handle.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if !sawThread || result.GetSessionID() != "thread-1" || strings.TrimSpace(result.TextOutput()) != "finished" {
		t.Fatalf("result session=%q text=%q sawThread=%v", result.GetSessionID(), result.TextOutput(), sawThread)
	}
}
