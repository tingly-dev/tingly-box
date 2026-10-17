package project

import (
	"os"
	"os/exec"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
)

// Agents run thread turns. Turns go through agentboot's session engine, the
// a thread keeps a resident Claude Code process
// between turns (so background tasks outlive the turn that started them),
// and falls back to one process per turn without a pool.
type Agents struct {
	Service *agentboot.AgentService
	// Pool keeps resident processes; nil runs every turn one-shot.
	Pool *pool.Pool
}

// threadPoolConfig sets the limits: at most this many resident processes,
// each closed after sitting idle this long (a later turn resumes it).
var threadPoolConfig = pool.Config{MaxSessions: 10, IdleTimeout: 10 * time.Minute}

// ClaudeAgents runs threads on the Claude Code CLI found on PATH (or
// CLAUDE_CLI_PATH), with resident processes.
func ClaudeAgents() Agents {
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		panic(err) // only fails on an invalid default config
	}
	svc.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgentWithConfig(claude.DefaultConfig()))
	return Agents{Service: svc, Pool: pool.New(threadPoolConfig)}
}

// ClaudeCLIPath is the Claude Code CLI threads run (CLAUDE_CLI_PATH, else
// claude on PATH), or "" if none is found.
func ClaudeCLIPath() string {
	if p := os.Getenv("CLAUDE_CLI_PATH"); p != "" {
		return p
	}
	p, err := exec.LookPath("claude")
	if err != nil {
		return ""
	}
	return p
}
