package server

import (
	"os"
	"testing"
)

// TestMain gives git a committer identity, so the merges the code under test
// runs work on machines without one (CI runners).
func TestMain(m *testing.M) {
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t",
	} {
		os.Setenv(k, v)
	}
	os.Exit(m.Run())
}
