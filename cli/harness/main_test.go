package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
)

func newTestParser(t *testing.T) (*CLI, *kong.Kong) {
	t.Helper()
	var cli CLI
	parser, err := kong.New(&cli, kong.Vars{
		"version":   "test",
		"gitCommit": "abcdef",
		"buildTime": "2026-01-01",
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
	// For each top-level subcommand, parse with arguments that should succeed
	// without invoking the handler. We use --help where the command has no
	// required subcommand; for those that do, we descend to a child.
	cases := []struct {
		name string
		args []string
	}{
		{"version", []string{"version"}},
		{"matrix-help", []string{"matrix", "--help"}},
		{"agent-help", []string{"agent", "--help"}},
		{"provider-list", []string{"provider", "list"}},
		{"provider-test", []string{"provider", "test"}},
		{"init-config-help", []string{"init-config", "--help"}},
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

func TestAgentCmdParsesAllFlags(t *testing.T) {
	cli, parser := newTestParser(t)
	_, err := parser.Parse([]string{
		"agent", "claude",
		"--mock",
		"--timeout", "30s",
		"--prompt", "hello",
		"--summary", "x.csv",
		"--output-dir", "out",
		"--resume", "key",
		"--filter", "a",
		"--filter", "b",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cli.Agent.AgentType != "claude" {
		t.Errorf("AgentType: got %q", cli.Agent.AgentType)
	}
	if !cli.Agent.Mock {
		t.Error("Mock not set")
	}
	if cli.Agent.Timeout != 30*time.Second {
		t.Errorf("Timeout: want 30s, got %v", cli.Agent.Timeout)
	}
	if cli.Agent.Prompt != "hello" {
		t.Errorf("Prompt: got %q", cli.Agent.Prompt)
	}
	if cli.Agent.Summary != "x.csv" {
		t.Errorf("Summary: got %q", cli.Agent.Summary)
	}
	if cli.Agent.OutputDir != "out" {
		t.Errorf("OutputDir: got %q", cli.Agent.OutputDir)
	}
	if cli.Agent.Resume != "key" {
		t.Errorf("Resume: got %q", cli.Agent.Resume)
	}
	if len(cli.Agent.Filter) != 2 || cli.Agent.Filter[0] != "a" || cli.Agent.Filter[1] != "b" {
		t.Errorf("Filter: got %v", cli.Agent.Filter)
	}
}

func TestAgentCmdTimeoutDefaultIs2m(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"agent", "--mock", "claude"}); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cli.Agent.Timeout != 2*time.Minute {
		t.Errorf("default Timeout: want 2m, got %v", cli.Agent.Timeout)
	}
}

func TestMatrixCmdNonStreamingFlag(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"matrix", "--non-streaming"}); err != nil {
		t.Fatalf("--non-streaming should parse: %v", err)
	}
	if !cli.Matrix.NonStream {
		t.Error("Matrix.NonStream should be true")
	}
}

func TestProviderTestCmdReturnsNotImplemented(t *testing.T) {
	var p ProviderTestCmd
	err := p.Run()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("expected not-implemented error, got: %v", err)
	}
}

func TestProviderListCmdReturnsNotImplemented(t *testing.T) {
	var p ProviderListCmd
	err := p.Run()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("expected not-implemented error, got: %v", err)
	}
}

func TestVersionCmdPrintsAllFields(t *testing.T) {
	version = "test-version"
	gitCommit = "test-commit"
	buildTime = "test-time"

	out := captureStdout(t, func() {
		v := &VersionCmd{}
		if err := v.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	})

	for _, want := range []string{
		"Tingly-Box Protocol Validation Harness",
		"Version:   test-version",
		"Commit:    test-commit",
		"Built:     test-time",
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
