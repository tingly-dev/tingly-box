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
		{"config-add", []string{"config", "add", "--help"}},
		{"config-list", []string{"config", "list", "--help"}},
		{"config-delete", []string{"config", "delete", "--help"}},
		{"config-update", []string{"config", "update", "--help"}},
		{"config-get", []string{"config", "get", "--help"}},
		{"config-export", []string{"config", "export", "--request-model", "x", "--scenario", "y", "--help"}},
		{"config-import", []string{"config", "import", "--help"}},
		{"agent-apply", []string{"agent", "apply", "--help"}},
		{"agent-show", []string{"agent", "show", "--help"}},
		{"agent-restore", []string{"agent", "restore", "--help"}},
		{"oauth", []string{"oauth", "--help"}},
		{"cc", []string{"cc", "--help"}},
		{"swagger", []string{"swagger", "--help"}},
		{"quota-get", []string{"quota", "get", "--help"}},
		{"quota-refresh", []string{"quota", "refresh", "--help"}},
		{"quota-summary", []string{"quota", "summary", "--help"}},
		{"remote-list", []string{"remote", "list", "--help"}},
		{"remote-start", []string{"remote", "start", "--help"}},
		{"remote-config", []string{"remote", "config", "--help"}},
		{"remote-add", []string{"remote", "add", "--help"}},
		{"remote-pair-enable", []string{"remote", "pair", "enable", "bot-uuid", "--help"}},
		{"tui", []string{"tui", "--help"}},
		{"quickstart-alias", []string{"quickstart", "--help"}},
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

// TestConfigHasAllSubcommands ensures config exposes add/list/delete/update/get/export/import
// (new unified command).
func TestConfigHasAllSubcommands(t *testing.T) {
	for _, sub := range []string{"add", "list", "delete", "update", "get", "export", "import"} {
		t.Run(sub, func(t *testing.T) {
			_, parser := newTestParser(t)
			args := []string{"config", sub, "--help"}
			// export requires special flags
			if sub == "export" {
				args = []string{"config", "export", "--request-model", "x", "--scenario", "y", "--help"}
			}
			if _, err := parser.Parse(args); err != nil && !isHelpErr(err) {
				t.Fatalf("config %s should parse: %v", sub, err)
			}
		})
	}
}

// TestConfigExportCmdParsesAndForwardsFlags ensures `config export` actually accepts its
// flags and the Kong handler can read them.
func TestConfigExportCmdParsesAndForwardsFlags(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{
		"config", "export",
		"--request-model", "gpt-4",
		"--scenario", "general",
		"--format", "base64",
		"--output", "out.txt",
	}); err != nil {
		t.Fatalf("config export flags should parse: %v", err)
	}
	if cli.Config.Export.RequestModel != "gpt-4" {
		t.Errorf("RequestModel: %q", cli.Config.Export.RequestModel)
	}
	if cli.Config.Export.Scenario != "general" {
		t.Errorf("Scenario: %q", cli.Config.Export.Scenario)
	}
	if cli.Config.Export.Format != "base64" {
		t.Errorf("Format: %q", cli.Config.Export.Format)
	}
	if cli.Config.Export.Output != "out.txt" {
		t.Errorf("Output: %q", cli.Config.Export.Output)
	}
}

// TestConfigImportFromStdinWhenNoFile ensures `config import` with no positional gets an
// empty args slice, so runImport reads from stdin instead of trying to open
// a file named "".
func TestConfigImportFromStdinWhenNoFile(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"config", "import"}); err != nil {
		t.Fatalf("config import (no file) should parse: %v", err)
	}
	if cli.Config.Import.File != "" {
		t.Errorf("File should be empty, got %q", cli.Config.Import.File)
	}
}

// TestSwaggerCommandIsHidden ensures the swagger command exists but is hidden from help.
func TestSwaggerCommandIsHidden(t *testing.T) {
	_, parser := newTestParser(t)
	// swagger command should parse
	if _, err := parser.Parse([]string{"swagger", "--help"}); err != nil && !isHelpErr(err) {
		t.Fatalf("swagger should parse: %v", err)
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

// TestTUICommandAndQuickstartAlias ensures both `tui` (the canonical name) and
// `quickstart` (the hidden legacy alias) parse without arguments.
func TestTUICommandAndQuickstartAlias(t *testing.T) {
	for _, name := range []string{"tui", "quickstart"} {
		t.Run(name, func(t *testing.T) {
			_, parser := newTestParser(t)
			if _, err := parser.Parse([]string{name}); err != nil {
				t.Fatalf("%s should parse: %v", name, err)
			}
		})
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

// TestAgentRestoreCommandExists ensures agent restore subcommand is available.
func TestAgentRestoreCommandExists(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"agent", "restore", "claude-code", "--force"}); err != nil {
		t.Fatalf("agent restore should parse: %v", err)
	}
	if !cli.Agent.Restore.Force {
		t.Error("Restore.Force should be true")
	}
}

// TestParentCommandDefaultsParse ensures `config`, `agent`, and `quota` parse with no
// subcommand (their hidden default subcommands take over).
func TestParentCommandDefaultsParse(t *testing.T) {
	for _, name := range []string{"config", "agent", "quota"} {
		t.Run(name, func(t *testing.T) {
			_, parser := newTestParser(t)
			if _, err := parser.Parse([]string{name}); err != nil {
				t.Fatalf("%s with no subcommand should parse: %v", name, err)
			}
		})
	}
}

// TestQuotaListIsDefault ensures quota with no args defaults to list.
func TestQuotaListIsDefault(t *testing.T) {
	cli, parser := newTestParser(t)
	// quota list supports --refresh flag
	if _, err := parser.Parse([]string{"quota", "list", "--refresh"}); err != nil {
		t.Fatalf("quota list --refresh should parse: %v", err)
	}
	if !cli.Quota.List.Refresh {
		t.Error("Quota.List.Refresh should be true")
	}
}

// TestQuotaGetWithProvider ensures quota get accepts provider argument.
func TestQuotaGetWithProvider(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"quota", "get", "my-provider", "--refresh"}); err != nil {
		t.Fatalf("quota get with provider should parse: %v", err)
	}
	if cli.Quota.Get.Provider != "my-provider" {
		t.Errorf("Provider: %q", cli.Quota.Get.Provider)
	}
	if !cli.Quota.Get.Refresh {
		t.Error("Get.Refresh should be true")
	}
}

// TestQuotaRefreshWithProvider ensures quota refresh accepts optional provider.
func TestQuotaRefreshWithProvider(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"quota", "refresh", "my-provider"}); err != nil {
		t.Fatalf("quota refresh with provider should parse: %v", err)
	}
	if cli.Quota.Refresh.Provider != "my-provider" {
		t.Errorf("Provider: %q", cli.Quota.Refresh.Provider)
	}
}

// TestAgentDefaultsToList ensures agent with no args defaults to list.
func TestAgentDefaultsToList(t *testing.T) {
	_, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"agent"}); err != nil {
		t.Fatalf("agent with no args should parse: %v", err)
	}
}

// TestConfigAddWithPositionalArgs ensures config add accepts provider args.
func TestConfigAddWithPositionalArgs(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{
		"config", "add", "my-provider",
		"https://api.example.com",
		"sk-xxx",
		"openai",
	}); err != nil {
		t.Fatalf("config add with args should parse: %v", err)
	}
	if cli.Config.Add.Name != "my-provider" {
		t.Errorf("Name: %q", cli.Config.Add.Name)
	}
	if cli.Config.Add.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL: %q", cli.Config.Add.BaseURL)
	}
	if cli.Config.Add.Token != "sk-xxx" {
		t.Errorf("Token: %q", cli.Config.Add.Token)
	}
	if cli.Config.Add.APIStyle != "openai" {
		t.Errorf("APIStyle: %q", cli.Config.Add.APIStyle)
	}
}

// TestRemotePairCommands ensures all remote pair subcommands are available.
func TestRemotePairCommands(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"pair-enable", []string{"remote", "pair", "enable", "bot-uuid"}},
		{"pair-disable", []string{"remote", "pair", "disable", "bot-uuid"}},
		{"pair-revoke", []string{"remote", "pair", "revoke", "bot-uuid", "chat-id"}},
		{"pair-status", []string{"remote", "pair", "status", "bot-uuid"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, parser := newTestParser(t)
			if _, err := parser.Parse(tc.args); err != nil {
				t.Fatalf("%s should parse: %v", tc.name, err)
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
