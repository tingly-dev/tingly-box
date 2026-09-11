package claude

import (
	"context"
	"strings"
	"testing"
)

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
