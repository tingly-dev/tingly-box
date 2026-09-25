package protocolserver

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// X-Tingly-Applied-Flags is how the probe journey shows which flags drove
// the request; claude_code_version must be listed or an applied profile
// looks like no profile at all.
func TestFormatAppliedFlags_ClaudeCodeVersion(t *testing.T) {
	if got := formatAppliedFlags(typ.RuleFlags{ClaudeCodeVersion: typ.ClaudeCodeVersion2_1_280}); got != "claude_code_version=2.1.280" {
		t.Errorf("formatAppliedFlags = %q", got)
	}
	if got := formatAppliedFlags(typ.RuleFlags{}); got != "" {
		t.Errorf("zero flags must format empty, got %q", got)
	}
}
