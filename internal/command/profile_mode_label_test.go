package command

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestProfileModeLabel(t *testing.T) {
	for want, p := range map[string]typ.ProfileMeta{
		"separate":                  {Unified: false},
		"unified":                   {Unified: true},
		"unified + haiku, subagent": {Unified: true, ClaudeCodeSlots: []string{"haiku", "subagent"}},
	} {
		if got := profileModeLabel(p); got != want {
			t.Errorf("profileModeLabel(%+v) = %q, want %q", p, got, want)
		}
	}
}
