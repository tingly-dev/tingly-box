package bot

import (
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

func TestNoApprovalModesPreserveClaudeAutoSemantics(t *testing.T) {
	if noApprovalModes[string(claude.PermissionModeAuto)] {
		t.Fatal("auto mode must use Claude Code's rule classifier, not unconditional approval")
	}
	for _, mode := range []claude.PermissionMode{
		claude.PermissionModeManual,
		claude.PermissionModeDontAsk,
		claude.PermissionModePlan,
	} {
		if noApprovalModes[string(mode)] {
			t.Fatalf("%s mode must preserve Claude Code's permission semantics", mode)
		}
	}
	if !noApprovalModes[string(claude.PermissionModeBypassPermissions)] {
		t.Fatal("bypassPermissions should remain non-interactive")
	}
}
