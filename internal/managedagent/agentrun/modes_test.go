package agentrun

import (
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// agentboot silently drops a --permission-mode value it does not recognise,
// which would turn a user's explicit choice into "whatever the settings
// say" with only a server log to show for it. Every mode the control plane
// offers must therefore be one agentboot forwards.
func TestPermissionModes_AllForwardedByAgentboot(t *testing.T) {
	for _, m := range managedagent.PermissionModes {
		if !claude.IsValidPermissionMode(string(m)) {
			t.Errorf("mode %q would be dropped by agentboot's CLI builder", m)
		}
	}
}
