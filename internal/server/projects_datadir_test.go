package server

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectsDataDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("TINGLY_PROJECT_HOME", "")

	assert.Equal(t, filepath.Join(home, ".tingly-project"), projectsDataDir(filepath.Join(home, ".tingly-box")))
	other := filepath.Join(home, "work", "tb-dev")
	assert.Equal(t, other+"-project", projectsDataDir(other))

	t.Setenv("TINGLY_PROJECT_HOME", "/srv/tp")
	assert.Equal(t, "/srv/tp", projectsDataDir(other))
}
