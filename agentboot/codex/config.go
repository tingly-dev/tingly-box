package codex

// Config controls the small, unattended Codex CLI surface used by agentboot.
// The defaults deliberately keep the native Codex sandbox enabled and deny
// interactive approvals; callers must never need a dangerous bypass flag.
type Config struct {
	CLIPath          string
	SandboxMode      string
	ApprovalPolicy   string
	CustomEnv        []string
	SkipGitRepoCheck bool
}

func DefaultConfig() Config {
	return Config{
		CLIPath:          "codex",
		SandboxMode:      "workspace-write",
		ApprovalPolicy:   "never",
		SkipGitRepoCheck: true,
	}
}
