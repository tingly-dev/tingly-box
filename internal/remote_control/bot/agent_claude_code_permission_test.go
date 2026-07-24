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

func TestClaudePermissionPolicyInheritsProfileWhenSessionUnset(t *testing.T) {
	mode, autoApprove := claudePermissionPolicy("")
	if mode != "" {
		t.Fatalf("empty session mode became %q; profile defaultMode must remain authoritative", mode)
	}
	if autoApprove {
		t.Fatal("inherited permission mode must not enable host-side auto approval")
	}

	mode, autoApprove = claudePermissionPolicy(string(claude.PermissionModeBypassPermissions))
	if mode != string(claude.PermissionModeBypassPermissions) || !autoApprove {
		t.Fatalf("explicit bypass override = (%q, %v)", mode, autoApprove)
	}
}

func TestToggledYoloPermissionModeClearsSessionOverride(t *testing.T) {
	if got := toggledYoloPermissionMode(string(claude.PermissionModeBypassPermissions)); got != "" {
		t.Fatalf("disabling /yolo returned %q; want inherited mode", got)
	}
	if got := toggledYoloPermissionMode(""); got != string(claude.PermissionModeBypassPermissions) {
		t.Fatalf("enabling /yolo returned %q", got)
	}
}
