package claude

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateClaudeCodeCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX executable script")
	}

	discovery := NewCLIDiscovery()
	valid := writeVersionCLI(t, "2.1.218 (Claude Code)")
	version, err := discovery.ValidateClaudeCodeCLI(context.Background(), valid)
	if err != nil {
		t.Fatalf("ValidateClaudeCodeCLI() error = %v", err)
	}
	if version != "2.1.218" {
		t.Fatalf("version = %q, want 2.1.218", version)
	}

	legacy := writeVersionCLI(t, "Claude CLI v1.0.0")
	if _, err := discovery.ValidateClaudeCodeCLI(context.Background(), legacy); err != nil {
		t.Fatalf("legacy Claude CLI banner should remain compatible: %v", err)
	}

	desktop := writeVersionCLI(t, "1.0.0 (Claude)")
	if _, err := discovery.ValidateClaudeCodeCLI(context.Background(), desktop); err == nil {
		t.Fatal("generic Claude desktop banner was accepted as Claude Code CLI")
	}
}

func TestBundledCandidatesExcludeClaudeDesktop(t *testing.T) {
	for _, path := range NewCLIDiscovery().bundledCandidatePaths() {
		if strings.Contains(path, "/Applications/Claude.app/") {
			t.Fatalf("Claude Desktop must not be a CLI candidate: %s", path)
		}
	}
}

func TestDriverHonorsAndValidatesConfiguredCLIPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX executable script")
	}

	valid := writeVersionCLI(t, "2.1.218 (Claude Code)")
	driver := NewDriver(Config{CLIPath: valid})
	if driver.cliPath != valid {
		t.Fatalf("driver CLI path = %q, want %q", driver.cliPath, valid)
	}
	if !driver.IsAvailable() {
		t.Fatal("verified configured Claude Code CLI should be available")
	}

	desktop := writeVersionCLI(t, "1.0.0 (Claude)")
	if NewDriver(Config{CLIPath: desktop}).IsAvailable() {
		t.Fatal("configured Claude Desktop executable should be rejected")
	}
}

func TestPermissionModesMatchClaudeCodeCLI(t *testing.T) {
	for _, mode := range []PermissionMode{
		PermissionModeDefault,
		PermissionModeAcceptEdits,
		PermissionModeAuto,
		PermissionModeBypassPermissions,
		PermissionModeManual,
		PermissionModeDontAsk,
		PermissionModePlan,
	} {
		if !IsValidPermissionMode(string(mode)) {
			t.Errorf("permission mode %q should be valid", mode)
		}
	}
}

func writeVersionCLI(t *testing.T, banner string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\nprintf '%s\\n' '" + banner + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
