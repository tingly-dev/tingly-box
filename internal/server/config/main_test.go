package config

import (
	"os"
	"testing"
)

// TestMain isolates every test in this package from the user's real home
// directory. Several functions under test resolve their target path from
// os.UserHomeDir() (e.g. ~/.codex/config.toml, ~/.claude/settings.json). Tests
// that exercise the full apply/read path must never write into the developer's
// real config files, so the whole binary is redirected to a throwaway HOME
// before any test runs. Individual tests may still point HOME at their own
// t.TempDir() for finer-grained isolation; this is the safety net that catches
// any test that forgets to.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "tb-config-test-home-")
	if err != nil {
		panic("failed to create isolated test home: " + err.Error())
	}
	defer os.RemoveAll(tmp)
	os.Setenv("HOME", tmp)
	os.Setenv("USERPROFILE", tmp)
	os.Exit(m.Run())
}
