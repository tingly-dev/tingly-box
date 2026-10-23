package claude

import (
	"context"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Sessions launched with the inherited or default entrypoint are recorded as
// "sdk-cli" and hidden from the interactive /resume picker; verified against
// Claude Code 2.1.309. Launches must carry their own entrypoint.
func TestPrepare_SetsResumableEntrypoint(t *testing.T) {
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "remote_mobile")
	d := NewDriver(Config{})
	d.SetCLIPath("/bin/true")
	d.SetForceAvailable(true)
	spec, err := d.Prepare(context.Background(), "hi", agentboot.ExecutionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(spec.Env, "\n") + "\n"
	if !strings.Contains(joined, "\nCLAUDE_CODE_ENTRYPOINT=tingly-box\n") {
		t.Errorf("entrypoint not set to %q", DefaultEntrypoint)
	}
	if strings.Contains(joined, "remote_mobile") {
		t.Error("inherited entrypoint leaked")
	}
}
