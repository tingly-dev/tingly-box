package typ

import "testing"

func TestResolveClaudeCodeVersion(t *testing.T) {
	for in, want := range map[string]string{
		ClaudeCodeVersionDefault: ClaudeCodeVersionLatest,
		ClaudeCodeVersionLegacy:  ClaudeCodeVersionLegacy,
		ClaudeCodeVersion2_1_280: ClaudeCodeVersion2_1_280,
		"9.9.9":                  ClaudeCodeVersionLatest,
	} {
		if got := ResolveClaudeCodeVersion(in); got != want {
			t.Errorf("ResolveClaudeCodeVersion(%q) = %q, want %q", in, got, want)
		}
	}
	if ClaudeCodeVersionEnabled(ClaudeCodeVersionLegacy) || ClaudeCodeVersionEnabled("") {
		t.Error("legacy and unset must not select the native profile")
	}
	if !ClaudeCodeVersionEnabled(ClaudeCodeVersionLatest) {
		t.Error("latest must select the native profile")
	}
}
