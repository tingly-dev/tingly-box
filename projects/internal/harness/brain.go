package harness

import (
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
)

// BrainFunc plays one turn of the coordinator's Claude Code session: it reads
// the turn's input, calls Projects tools (c.Call runs them for real, through
// the session's MCP server) and answers with c.Reply. Returning an error fails
// the turn, as a model or CLI failure would.
type BrainFunc func(c *coordtest.Turn) error

// NoUpdate is the old sentinel text for saying nothing.
const NoUpdate = coordtest.NoUpdate

// Request is one coordinator turn as the session saw it.
type Request struct {
	// Prompt is what the turn was sent: on the first turn the earlier
	// conversation and the project context, then the new input's envelope.
	Prompt string
	// Calls are the tools the turn called, in order.
	Calls []coordtest.Call
}

// CheckIn reports whether a thread's report woke this turn.
func (r Request) CheckIn() bool { return strings.Contains(r.Prompt, `kind="check-in"`) }

type brain struct {
	fn    BrainFunc
	mu    sync.Mutex
	turns []*coordtest.Turn
}

func (b *brain) turn(c *coordtest.Turn) error {
	b.mu.Lock()
	b.turns = append(b.turns, c)
	b.mu.Unlock()
	if b.fn == nil {
		return nil
	}
	return b.fn(c)
}

func (b *brain) requests() []Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Request, 0, len(b.turns))
	for _, t := range b.turns {
		out = append(out, Request{Prompt: t.Prompt, Calls: t.Calls()})
	}
	return out
}
