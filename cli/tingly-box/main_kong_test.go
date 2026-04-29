//go:build !legacy

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/tingly-dev/tingly-box/internal/command"
)

func newTestParser(t *testing.T) (*CLI, *kong.Kong) {
	t.Helper()
	var cli CLI
	parser, err := kong.New(&cli, kong.Vars{
		"version":   "test",
		"gitCommit": "abcdef",
		"buildTime": "2026-01-01",
		"goVersion": "go1.25",
		"platform":  "linux/amd64",
	}, kong.Exit(func(int) {}))
	if err != nil {
		t.Fatalf("kong.New failed: %v", err)
	}
	return &cli, parser
}

func TestKongCLIDefinitionParses(t *testing.T) {
	newTestParser(t)
}

func TestKongAllTopLevelSubcommandsRecognized(t *testing.T) {
	// Each top-level command is exercised with arguments that fully resolve
	// the command tree without invoking any handler. Commands that have
	// required subcommands are descended; commands with required flags get
	// --help.
	cases := []struct {
		name string
		args []string
	}{
		{"start", []string{"start", "--help"}},
		{"stop", []string{"stop", "--help"}},
		{"status", []string{"status", "--help"}},
		{"restart", []string{"restart", "--help"}},
		{"open", []string{"open", "--help"}},
		{"provider-add", []string{"provider", "add", "--help"}},
		{"provider-list", []string{"provider", "list", "--help"}},
		{"provider-delete", []string{"provider", "delete", "--help"}},
		{"provider-update", []string{"provider", "update", "--help"}},
		{"provider-get", []string{"provider", "get", "--help"}},
		{"agent-apply", []string{"agent", "apply", "--help"}},
		{"agent-list", []string{"agent", "list", "--help"}},
		{"agent-show", []string{"agent", "show", "--help"}},
		{"oauth", []string{"oauth", "--help"}},
		{"export", []string{"export", "--request-model", "x", "--scenario", "y", "--help"}},
		{"import", []string{"import", "--help"}},
		{"cc", []string{"cc", "--help"}},
		{"swagger", []string{"swagger", "--help"}},
		{"quota-list", []string{"quota", "list", "--help"}},
		{"quota-get", []string{"quota", "get", "--help"}},
		{"quota-refresh", []string{"quota", "refresh", "--help"}},
		{"quota-summary", []string{"quota", "summary", "--help"}},
		{"remote-list", []string{"remote", "list", "--help"}},
		{"remote-start", []string{"remote", "start", "--help"}},
		{"remote-config", []string{"remote", "config", "--help"}},
		{"quickstart", []string{"quickstart", "--help"}},
		{"mcp-builtin", []string{"mcp-builtin", "--help"}},
		{"version", []string{"version", "--help"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, parser := newTestParser(t)
			if _, err := parser.Parse(tc.args); err != nil && !isHelpErr(err) {
				t.Fatalf("args %v failed to parse: %v", tc.args, err)
			}
		})
	}
}

// TestOAuthCommandRoutesByName ensures `oauth` is reachable rather than being
// auto-kebab-cased to `o-auth`.
func TestOAuthCommandRoutesByName(t *testing.T) {
	_, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"oauth", "--help"}); err != nil && !isHelpErr(err) {
		t.Fatalf("oauth should parse: %v", err)
	}
}

// TestMCPBuiltinTopLevel ensures the legacy `mcp-builtin` command path is preserved.
// internal/mcp/runtime/builtin_registry.go invokes the binary with this exact arg.
func TestMCPBuiltinTopLevel(t *testing.T) {
	_, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"mcp-builtin", "--help"}); err != nil && !isHelpErr(err) {
		t.Fatalf("mcp-builtin should parse: %v", err)
	}
}

// TestProviderHasAllSubcommands ensures provider exposes add/list/delete/update/get
// (legacy parity).
func TestProviderHasAllSubcommands(t *testing.T) {
	for _, sub := range []string{"add", "list", "delete", "update", "get"} {
		t.Run(sub, func(t *testing.T) {
			_, parser := newTestParser(t)
			if _, err := parser.Parse([]string{"provider", sub, "--help"}); err != nil && !isHelpErr(err) {
				t.Fatalf("provider %s should parse: %v", sub, err)
			}
		})
	}
}

// TestExportCmdParsesAndForwardsFlags ensures `export` actually accepts its
// flags and the Kong handler can read them. The previous implementation
// errored with "no such flag -request-model" because the mock cobra cmd had
// no flags registered.
func TestExportCmdParsesAndForwardsFlags(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{
		"export",
		"--request-model", "gpt-4",
		"--scenario", "general",
		"--format", "base64",
		"--output", "out.txt",
	}); err != nil {
		t.Fatalf("export flags should parse: %v", err)
	}
	if cli.Export.RequestModel != "gpt-4" {
		t.Errorf("RequestModel: %q", cli.Export.RequestModel)
	}
	if cli.Export.Scenario != "general" {
		t.Errorf("Scenario: %q", cli.Export.Scenario)
	}
	if cli.Export.Format != "base64" {
		t.Errorf("Format: %q", cli.Export.Format)
	}
	if cli.Export.Output != "out.txt" {
		t.Errorf("Output: %q", cli.Export.Output)
	}
}

// TestStartCmdDebugFlagSet ensures --debug sets EnableDebug. The earlier
// implementation parsed --debug into EnableDebug, but the unregistered mock
// cobra command silently dropped it before ResolveStartOptions could see
// Changed("debug")=true, so config.GetDebug() always won.
func TestStartCmdDebugFlagSet(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"start", "--debug"}); err != nil {
		t.Fatalf("start --debug should parse: %v", err)
	}
	if !cli.Start.EnableDebug {
		t.Error("Start.EnableDebug should be true after --debug")
	}
}

// TestImportFromStdinWhenNoFile ensures `import` with no positional gets an
// empty args slice, so runImport reads from stdin instead of trying to open
// a file named "".
func TestImportFromStdinWhenNoFile(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"import"}); err != nil {
		t.Fatalf("import (no file) should parse: %v", err)
	}
	if cli.Import.File != "" {
		t.Errorf("File should be empty, got %q", cli.Import.File)
	}
}

// TestQuickstartUseTUIDefault ensures --tui defaults to true (legacy parity:
// without the flag the TUI wizard runs, not the plain quickstart).
func TestQuickstartUseTUIDefault(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"quickstart"}); err != nil {
		t.Fatalf("quickstart should parse: %v", err)
	}
	if !cli.Quickstart.UseTUI {
		t.Error("Quickstart.UseTUI should default to true")
	}
}

// TestRemoteAddSubcommandExists ensures `remote add` is reachable (legacy has
// it, but the original Kong version was missing it entirely).
func TestRemoteAddSubcommandExists(t *testing.T) {
	_, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"remote", "add", "--help"}); err != nil && !isHelpErr(err) {
		t.Fatalf("remote add should parse: %v", err)
	}
}

// TestRemoteStartFlagsParse ensures the start subcommand exposes the
// --data-path/--provider/--model/--force flags that legacy supports.
func TestRemoteStartFlagsParse(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{
		"remote", "start", "abc",
		"--data-path", "/tmp/d",
		"--provider", "p",
		"--model", "m",
		"--force",
	}); err != nil {
		t.Fatalf("remote start with all flags should parse: %v", err)
	}
	if cli.Remote.Start.UUID != "abc" {
		t.Errorf("UUID: %q", cli.Remote.Start.UUID)
	}
	if cli.Remote.Start.DataPath != "/tmp/d" {
		t.Errorf("DataPath: %q", cli.Remote.Start.DataPath)
	}
	if cli.Remote.Start.Provider != "p" {
		t.Errorf("Provider: %q", cli.Remote.Start.Provider)
	}
	if cli.Remote.Start.Model != "m" {
		t.Errorf("Model: %q", cli.Remote.Start.Model)
	}
	if !cli.Remote.Start.Force {
		t.Error("Force should be true")
	}
}

// TestAgentApplyUnifiedDefaultIsTrue ensures the legacy default (--unified=true)
// is preserved in Kong.
func TestAgentApplyUnifiedDefaultIsTrue(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"agent", "apply", "claude-code"}); err != nil {
		t.Fatalf("agent apply should parse: %v", err)
	}
	if !cli.Agent.Apply.Unified {
		t.Error("Apply.Unified should default to true")
	}
}

// TestParentCommandDefaultsParse ensures `provider` and `quota` parse with no
// subcommand (their hidden default subcommands take over).
func TestParentCommandDefaultsParse(t *testing.T) {
	for _, name := range []string{"provider", "quota"} {
		t.Run(name, func(t *testing.T) {
			_, parser := newTestParser(t)
			if _, err := parser.Parse([]string{name}); err != nil {
				t.Fatalf("%s with no subcommand should parse: %v", name, err)
			}
		})
	}
}

func TestVersionCmdPrintsAllFields(t *testing.T) {
	command.BuildVersion = "test-version"
	command.BuildGitCommit = "test-commit"
	command.BuildBuildTime = "test-time"
	command.BuildGoVersion = "test-go"
	command.BuildPlatform = "test-platform"

	out := captureStdout(t, func() {
		cmd := &command.VersionCmdKong{}
		if err := cmd.Run(nil); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	})

	for _, want := range []string{
		"Tingly Box CLI",
		"Version:    test-version",
		"Git Commit: test-commit",
		"Build Time: test-time",
		"Go Version: test-go",
		"Platform:   test-platform",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func isHelpErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "help") || strings.Contains(msg, "usage")
}
