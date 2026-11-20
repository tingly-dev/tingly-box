package project

import (
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
)

// Agents run coordinator and thread turns. Turns go through agentboot's
// session engine; a session keeps a resident Claude Code process between
// turns (so background tasks outlive the turn that started them), and falls
// back to one process per turn without a pool.
type Agents struct {
	Service *agentboot.AgentService
	// Pool keeps the threads' resident processes; nil runs every turn
	// one-shot.
	Pool *pool.Pool
	// CoordinatorPool keeps the coordinators' resident processes, apart from
	// the threads' so each has its own idle window. Nil uses Pool.
	CoordinatorPool *pool.Pool
	// coordinatorHold is what CoordinatorPool asks before closing a process;
	// the service sets it once it can tell which threads are working.
	coordinatorHold *lateHold
}

// SessionConfig tunes how long resident Claude Code processes stay open.
// A closed process is not lost: the next turn resumes its session, at the
// price of one cold start (and a prompt cache that has likely expired).
type SessionConfig struct {
	// ThreadIdleTimeout closes a thread's process this long after its last
	// activity. Zero uses DefaultThreadIdleTimeout; negative never closes it
	// for being idle.
	ThreadIdleTimeout time.Duration
	// CoordinatorIdleTimeout does the same for a project's coordinator. It
	// counts from the moment the project has no thread working: while any
	// thread works the coordinator stays open however long it has been
	// quiet. Zero uses DefaultCoordinatorIdleTimeout; negative never closes
	// it for being idle.
	CoordinatorIdleTimeout time.Duration
}

// The defaults outlast Claude Code's one-hour prompt cache: a session
// resumed within the hour still reads its context from the cache, so closing
// it sooner only makes the next turn dearer. A thread gets pool.DefaultIdleTimeout
// (65 min). The coordinator waits on the threads and on the user, so it gets
// twice that.
const (
	DefaultThreadIdleTimeout      = pool.DefaultIdleTimeout
	DefaultCoordinatorIdleTimeout = 2 * pool.DefaultIdleTimeout
)

// maxResidentSessions is how many processes each pool keeps open at most; it
// matches MaxRunningSessions.
const maxResidentSessions = 10

// idleTimeout is d, or def for zero; negative disables idle closing.
func idleTimeout(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}

// threadPoolConfig is the pool of thread processes.
func (c SessionConfig) threadPoolConfig() pool.Config {
	return pool.Config{MaxSessions: maxResidentSessions, IdleTimeout: idleTimeout(c.ThreadIdleTimeout, DefaultThreadIdleTimeout)}
}

// coordinatorPoolConfig is the pool of coordinator processes.
func (c SessionConfig) coordinatorPoolConfig(hold func(string) bool) pool.Config {
	return pool.Config{MaxSessions: maxResidentSessions, IdleTimeout: idleTimeout(c.CoordinatorIdleTimeout, DefaultCoordinatorIdleTimeout), Hold: hold}
}

// lateHold is a pool's Hold whose answer is wired in after the pool exists:
// the pool is built before the service that knows what keeps a session
// needed.
type lateHold struct {
	mu sync.RWMutex
	fn func(key string) bool
}

func (h *lateHold) set(fn func(key string) bool) {
	h.mu.Lock()
	h.fn = fn
	h.mu.Unlock()
}

func (h *lateHold) hold(key string) bool {
	h.mu.RLock()
	fn := h.fn
	h.mu.RUnlock()
	return fn != nil && fn(key)
}

// ClaudeAgents runs threads and coordinators on the Claude Code CLI found on
// PATH (or CLAUDE_CLI_PATH), with resident processes.
func ClaudeAgents(cfg SessionConfig) Agents {
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		panic(err) // only fails on an invalid default config
	}
	svc.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgentWithConfig(claude.DefaultConfig()))
	hold := &lateHold{}
	return Agents{
		Service:         svc,
		Pool:            pool.New(cfg.threadPoolConfig()),
		CoordinatorPool: pool.New(cfg.coordinatorPoolConfig(hold.hold)),
		coordinatorHold: hold,
	}
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
