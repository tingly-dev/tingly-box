package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadPromptFile_MissingReturnsNotOkNoError(t *testing.T) {
	cfg := &Config{ConfigDir: t.TempDir()}

	content, ok, err := cfg.LoadPromptFile("claude", "system_prompt")
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, content)
}

func TestLoadPromptFile_ReadsOverrideContent(t *testing.T) {
	cfg := &Config{ConfigDir: t.TempDir()}

	agentDir := filepath.Join(cfg.PromptsDir(), "claude")
	require.NoError(t, os.MkdirAll(agentDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "system_prompt.md"), []byte("custom prompt body"), 0600))

	content, ok, err := cfg.LoadPromptFile("claude", "system_prompt")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "custom prompt body", content)
}

func TestLoadClaudePromptFile_IsClaudeShortcut(t *testing.T) {
	cfg := &Config{ConfigDir: t.TempDir()}

	agentDir := filepath.Join(cfg.PromptsDir(), "claude")
	require.NoError(t, os.MkdirAll(agentDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "guide.md"), []byte("guide body"), 0600))

	content, ok, err := cfg.LoadClaudePromptFile("guide")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "guide body", content)
}

func TestPromptsDir_FallsBackToDefaultWhenConfigDirEmpty(t *testing.T) {
	cfg := &Config{}
	require.NotEmpty(t, cfg.PromptsDir())
}
