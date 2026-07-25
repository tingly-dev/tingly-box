package configapply

import (
	"os"
	"testing"
)

// TestMain isolates every test in this package from the user's real home
// directory. The apply handlers resolve their target paths from
// os.UserHomeDir() (e.g. ~/.claude/settings.json via ApplyClaudeSettings*,
// ~/.codex/config.toml via ReadCodexConfig). A test that drives the full
// handler path must never mutate the developer's real config files, so the
// whole binary is redirected to a throwaway HOME before any test runs.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "tb-configapply-test-home-")
	if err != nil {
		panic("failed to create isolated test home: " + err.Error())
	}
	defer os.RemoveAll(tmp)
	os.Setenv("HOME", tmp)
	os.Setenv("USERPROFILE", tmp)
	os.Exit(m.Run())
}
