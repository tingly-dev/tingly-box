package claude

import (
	"context"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot"
)

func prepareEnv(t *testing.T, opts agentboot.ExecutionOptions) string {
	t.Helper()
	d := NewDriver(Config{})
	d.SetCLIPath("/bin/true")
	d.SetForceAvailable(true)
	spec, err := d.Prepare(context.Background(), "hi", opts)
	if err != nil {
		t.Fatal(err)
	}
	return "\n" + strings.Join(spec.Env, "\n") + "\n"
}

// Headless launches are recorded as "sdk-cli" and hidden from the interactive
// /resume picker (Claude Code 2.1.309). Hosts opt out of that per execution
// with Entrypoint; without it the CLI default stands and the parent's value
// never leaks in.
func TestPrepare_Entrypoint(t *testing.T) {
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "remote_mobile")

	if env := prepareEnv(t, agentboot.ExecutionOptions{}); strings.Contains(env, "\nCLAUDE_CODE_ENTRYPOINT=") {
		t.Error("entrypoint set without being asked")
	}

	env := prepareEnv(t, agentboot.ExecutionOptions{Entrypoint: "tingly-box"})
	if !strings.Contains(env, "\nCLAUDE_CODE_ENTRYPOINT=tingly-box\n") || strings.Contains(env, "remote_mobile") {
		t.Errorf("entrypoint not applied: %q", env)
	}

	env = prepareEnv(t, agentboot.ExecutionOptions{Entrypoint: "tingly-box", Env: []string{"CLAUDE_CODE_ENTRYPOINT=custom"}})
	if !strings.Contains(env, "\nCLAUDE_CODE_ENTRYPOINT=custom\n") {
		t.Errorf("Env entry should win: %q", env)
	}
}
