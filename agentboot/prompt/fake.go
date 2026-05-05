package prompt

import (
	"context"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// FakePrompter is a deterministic, scriptable [agentboot.Prompter] for
// tests. It implements the documented Prompter contract:
//
//   - Default behavior with no scripted responses: approve everything.
//   - Scripted responses are consumed FIFO via [QueueApproval] /
//     [QueueAsk].
//   - When a returned ApprovalResult has Remember=true, the tool name
//     is added to an AlwaysAllow cache; subsequent OnApproval calls for
//     the same tool short-circuit to Approved=true *without* consuming
//     a queued response.
//   - If [WithDelay] is set, the prompter blocks for the delay (or
//     until ctx is canceled). On ctx cancel/deadline it returns
//     Approved=false, matching the production deny-on-timeout default.
//
// Concurrency: safe for concurrent use across goroutines. Useful
// because the agentboot runner pumps events on its own goroutine.
type FakePrompter struct {
	mu sync.Mutex

	approvals []agentboot.PermissionResult
	asks      []agentboot.AskResult

	// alwaysAllow caches tool names that the script (or a previous
	// caller) approved with Remember=true.
	alwaysAllow map[string]bool

	delay time.Duration

	// callCounters track how many times each method was invoked,
	// useful in tests asserting "AlwaysAllow short-circuited the
	// second call".
	approvalCalls int
	askCalls      int
}

// NewFakePrompter returns a FakePrompter that approves everything by
// default. Call [QueueApproval] / [QueueAsk] to script specific
// responses, or [WithDelay] to simulate a slow user.
func NewFakePrompter() *FakePrompter {
	return &FakePrompter{
		alwaysAllow: make(map[string]bool),
	}
}

// QueueApproval enqueues one PermissionResult to be returned by the
// next OnApproval call (FIFO). Pass Remember=true on the result to
// install the tool into the AlwaysAllow cache.
func (p *FakePrompter) QueueApproval(r agentboot.PermissionResult) *FakePrompter {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.approvals = append(p.approvals, r)
	return p
}

// QueueAsk enqueues one AskResult to be returned by the next OnAsk
// call (FIFO).
func (p *FakePrompter) QueueAsk(r agentboot.AskResult) *FakePrompter {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asks = append(p.asks, r)
	return p
}

// WithDelay configures the prompter to block for d before responding,
// useful for testing ctx cancellation semantics. Setting d=0 (default)
// returns immediately.
func (p *FakePrompter) WithDelay(d time.Duration) *FakePrompter {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.delay = d
	return p
}

// AddAlwaysAllow seeds the AlwaysAllow cache directly. Equivalent to
// processing a prior approval with Remember=true for toolName.
func (p *FakePrompter) AddAlwaysAllow(toolName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.alwaysAllow[toolName] = true
}

// ApprovalCalls returns the number of OnApproval invocations,
// including those short-circuited by AlwaysAllow.
func (p *FakePrompter) ApprovalCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.approvalCalls
}

// AskCalls returns the number of OnAsk invocations.
func (p *FakePrompter) AskCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.askCalls
}

// OnApproval implements [agentboot.Prompter].
func (p *FakePrompter) OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	p.mu.Lock()
	p.approvalCalls++
	if req.ToolName != "" && p.alwaysAllow[req.ToolName] {
		p.mu.Unlock()
		return agentboot.PermissionResult{
			Approved:     true,
			UpdatedInput: req.Input,
			Reason:       "tool is whitelisted (Always Allow)",
		}, nil
	}
	delay := p.delay
	var result agentboot.PermissionResult
	if len(p.approvals) > 0 {
		result = p.approvals[0]
		p.approvals = p.approvals[1:]
	} else {
		result = agentboot.PermissionResult{Approved: true, UpdatedInput: req.Input}
	}
	if result.Approved && req.ToolName != "" && result.Remember {
		p.alwaysAllow[req.ToolName] = true
	}
	p.mu.Unlock()

	if delay > 0 {
		if err := waitOrCtx(ctx, delay); err != nil {
			return agentboot.PermissionResult{Approved: false, Reason: err.Error()}, err
		}
	}
	return result, nil
}

// OnAsk implements [agentboot.Prompter].
func (p *FakePrompter) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	p.mu.Lock()
	p.askCalls++
	delay := p.delay
	var result agentboot.AskResult
	if len(p.asks) > 0 {
		result = p.asks[0]
		p.asks = p.asks[1:]
	} else {
		result = agentboot.AskResult{ID: req.ID, Approved: true, UpdatedInput: req.Input}
	}
	p.mu.Unlock()

	if delay > 0 {
		if err := waitOrCtx(ctx, delay); err != nil {
			return agentboot.AskResult{ID: req.ID, Approved: false, Reason: err.Error()}, err
		}
	}
	return result, nil
}

func waitOrCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
