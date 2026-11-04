package claude

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// A recovery launch (empty prompt, ResumeInterruptedTurn) must inject no
// user message — the CLI replays the interrupted turn from its transcript —
// and must keep stdin open: an immediately-closed InitialInput channel would
// feed EOF and end the --print stream-json process before it recovers
// anything. Verified empirically against CLI 2.1.303.
func TestPrepare_EmptyPromptHoldsStdinOpen(t *testing.T) {
	d := NewDriver(Config{})
	d.SetCLIPath("/bin/true")
	d.SetForceAvailable(true)

	spec, err := d.Prepare(context.Background(), "", agentboot.ExecutionOptions{
		PermissionPromptTool: "stdio",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.InitialInput == nil {
		t.Fatal("recovery launch has no InitialInput channel at all")
	}
	select {
	case msg, ok := <-spec.InitialInput:
		if ok {
			t.Fatalf("recovery launch injected a message: %v", msg)
		}
		t.Fatal("recovery launch's InitialInput closed immediately — stdin would EOF")
	case <-time.After(100 * time.Millisecond):
		// Still open and empty: correct.
	}

	// A normal launch still injects exactly one user message.
	spec, err = d.Prepare(context.Background(), "hello", agentboot.ExecutionOptions{
		PermissionPromptTool: "stdio",
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := <-spec.InitialInput
	if !ok {
		t.Fatal("normal launch produced no initial message")
	}
	if m, isMap := msg.(map[string]any); !isMap || m["type"] != "user" {
		t.Fatalf("normal launch's first message is not a user message: %v", msg)
	}
}
