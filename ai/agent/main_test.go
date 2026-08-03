package agent

import (
	"os"
	"testing"
)

// TestMain isolates every test in this package from the developer's real
// home directory. Apply/Restore functions resolve their target paths from
// os.UserHomeDir() (e.g. ~/.claude/settings.json, ~/.codex/config.toml), so
// the whole binary is redirected to a throwaway HOME before any test runs.
// Mirrors internal/server/config/main_test.go's isolation strategy.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "tb-agent-test-home-")
	if err != nil {
		panic("failed to create isolated test home: " + err.Error())
	}
	defer os.RemoveAll(tmp)
	os.Setenv("HOME", tmp)
	os.Setenv("USERPROFILE", tmp)
	os.Exit(m.Run())
}
