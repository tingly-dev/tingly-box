// Package config resolves where tingly-project keeps its data and which
// address it serves on.
package config

import (
	"os"
	"path/filepath"
)

// DefaultPort sits next to tingly-box's 12580.
const DefaultPort = 12590

// Config is the runtime configuration.
type Config struct {
	Host    string
	Port    int
	DataDir string
}

// DefaultDataDir is $TINGLY_PROJECT_HOME, else ~/.tingly-project.
func DefaultDataDir() string {
	if d := os.Getenv("TINGLY_PROJECT_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".tingly-project"
	}
	return filepath.Join(home, ".tingly-project")
}

// DBPath is the SQLite database file.
func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "tingly-project.db")
}

// ProjectsDir holds each project's memory, library and thread worktrees.
func (c Config) ProjectsDir() string {
	return filepath.Join(c.DataDir, "projects")
}
