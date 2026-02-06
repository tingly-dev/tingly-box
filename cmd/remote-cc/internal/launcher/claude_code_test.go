package launcher

import (
	"testing"
	"time"
)

func TestClaudeCodeLauncher_IsAvailable(t *testing.T) {
	launcher := NewClaudeCodeLauncher()

	result := launcher.IsAvailable()

	t.Logf("Claude Code CLI available: %v", result)
}

func TestClaudeCodeLauncher_GetCLIInfo(t *testing.T) {
	launcher := NewClaudeCodeLauncher()

	info := launcher.GetCLIInfo()

	if info == nil {
		t.Fatal("Expected CLI info to be returned")
	}

	t.Logf("CLI Info: %v", info)
}

func TestClaudeCodeLauncher_Execute_Success(t *testing.T) {
	// This test requires actual Claude Code CLI installation
	// Skipping as the CLI might not be available or might have different command
	t.Skip("Skipping - requires actual Claude Code CLI")
}

func TestClaudeCodeLauncher_ExecuteWithTimeout_Timeout(t *testing.T) {
	// This test tests Claude Code's timeout behavior, not our code
	t.Skip("Skipping - tests Claude Code timeout behavior, not our code")
}

func TestClaudeCodeLauncher_ExecuteWithTimeout_Success(t *testing.T) {
	// This test requires actual Claude Code CLI installation
	t.Skip("Skipping - requires actual Claude Code CLI")
}

func TestClaudeCodeLauncher_DefaultTimeout(t *testing.T) {
	launcher := NewClaudeCodeLauncher()

	expected := 5 * time.Minute
	if launcher.defaultTimeout != expected {
		t.Errorf("Expected default timeout %v, got %v", expected, launcher.defaultTimeout)
	}
}
