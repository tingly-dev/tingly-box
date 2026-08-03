package agent

import (
	"os"
	"testing"
	"time"
)

func TestRestoreAgent_InvalidAgentType(t *testing.T) {
	if _, err := RestoreAgent(AgentType("not-a-real-agent")); err == nil {
		t.Fatal("expected error for invalid agent type")
	}
}

func TestRestoreAgent_PartialFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Claude Code manages two files: ~/.claude/settings.json and
	// ~/.claude.json. Apply twice so settings.json has a backup, then delete
	// .claude.json's backup directory so only one of the two files can be
	// restored — RestoreAgent should report a partial (non-aborting) failure.
	cfg := &ClaudeCodeConfig{}
	if _, err := cfg.Apply(&ClaudeCodeParams{BaseURL: "https://first", APIKey: "key-1"}); err != nil {
		t.Fatalf("first Apply returned error: %v", err)
	}
	if _, err := cfg.Apply(&ClaudeCodeParams{BaseURL: "https://second", APIKey: "key-2"}); err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}

	onboardingBackupDir := home + "/backup"
	if err := os.RemoveAll(onboardingBackupDir); err != nil {
		t.Fatalf("failed to remove onboarding backup dir: %v", err)
	}

	// RestoreLatestBackup takes its own pre-restore backup of the live file
	// before overwriting it, using the same second-granularity timestamp
	// scheme as Apply. Give it a moment to cross a timestamp boundary so it
	// doesn't collide with the backup the second Apply just created.
	time.Sleep(1100 * time.Millisecond)

	result, err := RestoreAgent(AgentTypeClaudeCode)
	if err != nil {
		t.Fatalf("RestoreAgent returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected partial restore to report Success=false, got %+v", result)
	}
	if len(result.RestoredFiles) == 0 {
		t.Error("expected settings.json to still be restored")
	}
	if len(result.Failures) == 0 {
		t.Error("expected .claude.json restore to fail (no backup)")
	}
	if result.Message == "" {
		t.Error("expected a human-readable message to be built")
	}
}

func TestRestoreAgent_UnknownAgentInfo(t *testing.T) {
	// AgentType.IsValid() only recognizes the three registered constants, so
	// this path (valid-looking but unregistered type) is unreachable through
	// normal parsing — RestoreAgent should still fail closed rather than panic.
	result, err := RestoreAgent(AgentType("codex"))
	if err != nil {
		t.Fatalf("RestoreAgent returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure with no codex backups present, got %+v", result)
	}
}
