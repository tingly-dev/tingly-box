package claude

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestBuildCleanEnv_AutoModeServerDefaultAndOverride(t *testing.T) {
	has := func(env []string, kv string) bool {
		for _, e := range env {
			if e == kv {
				return true
			}
		}
		return false
	}

	t.Setenv("CLAUDE_CODE_AUTO_MODE_SERVER", "")
	os.Unsetenv("CLAUDE_CODE_AUTO_MODE_SERVER")
	env, err := NewCLIDiscovery().buildCleanEnv(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !has(env, "CLAUDE_CODE_AUTO_MODE_SERVER=0") {
		t.Errorf("default CLAUDE_CODE_AUTO_MODE_SERVER=0 missing")
	}

	t.Setenv("CLAUDE_CODE_AUTO_MODE_SERVER", "1")
	env, err = NewCLIDiscovery().buildCleanEnv(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !has(env, "CLAUDE_CODE_AUTO_MODE_SERVER=1") || has(env, "CLAUDE_CODE_AUTO_MODE_SERVER=0") {
		t.Errorf("user's CLAUDE_CODE_AUTO_MODE_SERVER=1 must win: %v", env)
	}

	// Per-launch env still overrides the base: MergeEnv appends it after the
	// base and exec keeps the last duplicate.
	merged := MergeEnv(env, []string{"CLAUDE_CODE_AUTO_MODE_SERVER=0"})
	last := ""
	for _, e := range merged {
		if strings.HasPrefix(e, "CLAUDE_CODE_AUTO_MODE_SERVER=") {
			last = e
		}
	}
	if last != "CLAUDE_CODE_AUTO_MODE_SERVER=0" {
		t.Errorf("opts.Env must override, last entry = %q", last)
	}
}

func TestBuildCleanEnv_DropsInheritedSessionIdentity(t *testing.T) {
	t.Setenv("CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST", "1")
	t.Setenv("CLAUDE_CODE_REMOTE", "1")
	t.Setenv("CLAUDE_CODE_REMOTE_SESSION_ID", "abc")
	t.Setenv("CLAUDE_SESSION_INGRESS_TOKEN_FILE", "/tmp/x")
	t.Setenv("SESSION_INGRESS_URL", "https://x")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "parent-session")
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "remote_mobile")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_MESSAGING_TOKEN", "tok")
	// User configuration must survive.
	t.Setenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS", "32000")
	t.Setenv("CLAUDE_CONFIG_DIR", "/tmp/cfg")
	t.Setenv("ANTHROPIC_BASE_URL", "http://gateway")

	d := NewCLIDiscovery()
	env, err := d.buildCleanEnv(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, dropped := range []string{
		"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST=", "CLAUDE_CODE_REMOTE=", "CLAUDE_CODE_REMOTE_SESSION_ID=",
		"CLAUDE_SESSION_INGRESS_TOKEN_FILE=", "SESSION_INGRESS_URL=",
		"CLAUDE_CODE_SESSION_ID=", "CLAUDE_CODE_ENTRYPOINT=", "CLAUDECODE=", "CLAUDE_CODE_MESSAGING_TOKEN=",
	} {
		if strings.Contains(joined, "\n"+dropped) {
			t.Errorf("%s should have been dropped", dropped)
		}
	}
	for _, kept := range []string{"CLAUDE_CODE_MAX_OUTPUT_TOKENS=32000", "CLAUDE_CONFIG_DIR=/tmp/cfg", "ANTHROPIC_BASE_URL=http://gateway"} {
		if !strings.Contains(joined, "\n"+kept+"\n") {
			t.Errorf("%s should have been kept", kept)
		}
	}
}
