package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// PromptsDir returns the directory where operator-editable prompt override
// files live for this config instance: <ConfigDir>/prompts/<agent>/<name>.md.
//
// This is deliberately separate from config.json: config.json is read/merged/
// written wholesale on every Save() (see Config.Save), so it is a poor home
// for large prompt bodies. Files under PromptsDir are read directly from disk
// on demand and never touched by config save/load, so prompt content can grow
// without affecting config.json size or its write path.
func (c *Config) PromptsDir() string {
	dir := c.ConfigDir
	if dir == "" {
		dir = constant.GetTinglyConfDir()
	}
	return constant.GetPromptsDir(dir)
}

// LoadPromptFile reads <PromptsDir>/<agent>/<name>.md and returns its content.
// ok is false (with a nil error) when the file does not exist — callers should
// treat that as "no override configured" and fall back to the built-in
// default, not as a failure.
func (c *Config) LoadPromptFile(agent, name string) (content string, ok bool, err error) {
	path := filepath.Join(c.PromptsDir(), agent, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

// LoadClaudePromptFile is a convenience wrapper for LoadPromptFile(agent="claude", ...).
func (c *Config) LoadClaudePromptFile(name string) (content string, ok bool, err error) {
	return c.LoadPromptFile("claude", name)
}
