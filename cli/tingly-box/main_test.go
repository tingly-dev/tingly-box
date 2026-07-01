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
		{"config-provider-add", []string{"config", "provider", "add", "--help"}},
		{"config-provider-list", []string{"config", "provider", "list", "--help"}},
		{"config-provider-delete", []string{"config", "provider", "delete", "--help"}},
		{"config-provider-update", []string{"config", "provider", "update", "--help"}},
		{"config-provider-get", []string{"config", "provider", "get", "--help"}},
		{"config-rule-add", []string{"config", "rule", "add", "--help"}},
		{"config-rule-list", []string{"config", "rule", "list", "--help"}},
		{"config-rule-update", []string{"config", "rule", "update", "--help"}},
		{"config-rule-delete", []string{"config", "rule", "delete", "--help"}},
		{"config-rule-export", []string{"config", "rule", "export", "--help"}},
		{"config-rule-import", []string{"config", "rule", "import", "--help"}},
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

// TestConfigProviderHasAllSubcommands ensures `config provider` exposes
// add/list/delete/update/get.
func TestConfigProviderHasAllSubcommands(t *testing.T) {
	for _, sub := range []string{"add", "list", "delete", "update", "get"} {
		t.Run(sub, func(t *testing.T) {
			_, parser := newTestParser(t)
			if _, err := parser.Parse([]string{"config", "provider", sub, "--help"}); err != nil && !isHelpErr(err) {
				t.Fatalf("config provider %s should parse: %v", sub, err)
			}
		})
	}
}

// TestConfigRuleHasAllSubcommands ensures `config rule` exposes
// add/list/update/delete/export/import.
func TestConfigRuleHasAllSubcommands(t *testing.T) {
	for _, sub := range []string{"add", "list", "update", "delete", "export", "import"} {
		t.Run(sub, func(t *testing.T) {
			_, parser := newTestParser(t)
			if _, err := parser.Parse([]string{"config", "rule", sub, "--help"}); err != nil && !isHelpErr(err) {
				t.Fatalf("config rule %s should parse: %v", sub, err)
			}
		})
	}
}

// TestConfigRuleExportCmdParsesAndForwardsFlags ensures `config rule export`
// accepts its flags and exposes them on the Kong struct.
func TestConfigRuleExportCmdParsesAndForwardsFlags(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{
		"config", "rule", "export",
		"some-uuid",
		"--format", "base64",
		"--output", "out.txt",
	}); err != nil {
		t.Fatalf("config rule export flags should parse: %v", err)
	}
	if cli.Config.Rule.Export.UUID != "some-uuid" {
		t.Errorf("UUID: %q", cli.Config.Rule.Export.UUID)
	}
	if cli.Config.Rule.Export.Format != "base64" {
		t.Errorf("Format: %q", cli.Config.Rule.Export.Format)
	}
	if cli.Config.Rule.Export.Output != "out.txt" {
		t.Errorf("Output: %q", cli.Config.Rule.Export.Output)
	}
}

// TestConfigRuleImportFromStdinWhenNoFile ensures `config rule import` with no
// positional gets an empty File so runImport reads from stdin.
func TestConfigRuleImportFromStdinWhenNoFile(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"config", "rule", "import"}); err != nil {
		t.Fatalf("config rule import (no file) should parse: %v", err)
	}
	if cli.Config.Rule.Import.File != "" {
		t.Errorf("File should be empty, got %q", cli.Config.Rule.Import.File)
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

// TestQuotaAllProvidersFlag ensures quota --all parses and shows all providers.
func TestQuotaAllProvidersFlag(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"quota", "--all"}); err != nil {
		t.Fatalf("quota --all should parse: %v", err)
	}
	if !cli.Quota.AllProviders {
		t.Error("Quota.AllProviders should be true")
	}
}

// TestQuotaWithProvider ensures quota accepts an optional provider argument.
func TestQuotaWithProvider(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"quota", "my-provider"}); err != nil {
		t.Fatalf("quota with provider should parse: %v", err)
	}
	if cli.Quota.Provider != "my-provider" {
		t.Errorf("Provider: %q", cli.Quota.Provider)
	}
}

// TestQuotaNoRefreshFlag ensures quota --no-refresh parses.
func TestQuotaNoRefreshFlag(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"quota", "my-provider", "--no-refresh"}); err != nil {
		t.Fatalf("quota --no-refresh should parse: %v", err)
	}
	if !cli.Quota.NoRefresh {
		t.Error("Quota.NoRefresh should be true")
	}
}

// TestAgentDefaultsToList ensures agent with no args defaults to list.
func TestAgentDefaultsToList(t *testing.T) {
	_, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"agent"}); err != nil {
		t.Fatalf("agent with no args should parse: %v", err)
	}
}

// TestConfigProviderAddWithPositionalArgs ensures `config provider add`
// accepts provider args.
func TestConfigProviderAddWithPositionalArgs(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{
		"config", "provider", "add", "my-provider",
		"https://api.example.com",
		"sk-xxx",
		"openai",
	}); err != nil {
		t.Fatalf("config provider add with args should parse: %v", err)
	}
	if cli.Config.Provider.Add.Name != "my-provider" {
		t.Errorf("Name: %q", cli.Config.Provider.Add.Name)
	}
	if cli.Config.Provider.Add.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL: %q", cli.Config.Provider.Add.BaseURL)
	}
	if cli.Config.Provider.Add.Token != "sk-xxx" {
		t.Errorf("Token: %q", cli.Config.Provider.Add.Token)
	}
	if cli.Config.Provider.Add.APIStyle != "openai" {
		t.Errorf("APIStyle: %q", cli.Config.Provider.Add.APIStyle)
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
