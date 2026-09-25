package desk

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// approvalTimeout bounds how long a turn waits for a web answer before
// giving up and denying — matching IMPrompter's own default so an
// abandoned tab does not wedge a turn forever.
const approvalTimeout = 5 * time.Minute

// webPrompter implements agentboot.Prompter for one turn: it appends the
// question to the session's transcript (so a page loading mid-turn sees it
// without a separate "pending" channel) and blocks until Respond resolves
// it, ctx is cancelled, or approvalTimeout elapses.
type webPrompter struct {
	sessionID string
	sessions  *session.Manager

	mu      sync.Mutex
	pending map[string]chan approvalResult
}

type approvalResult struct {
	approved bool
	answer   string
}

func newWebPrompter(sessionID string, sessions *session.Manager) *webPrompter {
	return &webPrompter{sessionID: sessionID, sessions: sessions, pending: map[string]chan approvalResult{}}
}

// hasPending reports whether a question or approval is waiting on the user.
func (p *webPrompter) hasPending() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pending) > 0
}

// resolve answers a pending request. Returns false if there was none (an
// unknown or already-answered id).
func (p *webPrompter) resolve(requestID string, approved bool, answer string) bool {
	p.mu.Lock()
	ch, ok := p.pending[requestID]
	if ok {
		delete(p.pending, requestID)
	}
	p.mu.Unlock()
	if !ok {
		return false
	}
	ch <- approvalResult{approved: approved, answer: answer}
	return true
}

// await registers requestID as pending, appends kind to the transcript, and
// blocks for an answer.
func (p *webPrompter) await(ctx context.Context, kind, requestID, text string, payload any) approvalResult {
	ch := make(chan approvalResult, 1)
	p.mu.Lock()
	p.pending[requestID] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.pending, requestID)
		p.mu.Unlock()
	}()

	raw, _ := json.Marshal(payload)
	p.sessions.AppendMessage(p.sessionID, session.Message{
		Kind: kind, RequestID: requestID, Content: text, Payload: raw, Timestamp: time.Now(),
	})

	select {
	case res := <-ch:
		return res
	case <-ctx.Done():
		return approvalResult{approved: false}
	case <-time.After(approvalTimeout):
		return approvalResult{approved: false}
	}
}

func (p *webPrompter) OnApproval(ctx context.Context, req agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	res := p.await(ctx, "approval_request", req.ID, req.ToolName, req.Input)
	label := "denied"
	if res.approved {
		label = "approved"
	}
	p.sessions.AppendMessage(p.sessionID, session.Message{Kind: "approval_response", RequestID: req.ID, Content: label, Timestamp: time.Now()})
	return agentboot.ApprovalResponse{Approved: res.approved}, nil
}

func (p *webPrompter) OnAsk(ctx context.Context, req agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	res := p.await(ctx, "ask_request", req.ID, req.Message, req.Input)
	p.sessions.AppendMessage(p.sessionID, session.Message{Kind: "ask_response", RequestID: req.ID, Content: res.answer, Timestamp: time.Now()})
	return agentboot.AskResponse{Approved: res.approved, Response: res.answer}, nil
}

// autoApprovePrompter wraps a prompter to auto-approve every permission
// request (bypassPermissions) while still deferring AskUserQuestion to it —
// the same split @cc's own executor makes: bypass is unconditional approval
// of tool use, never of a free-form question.
type autoApprovePrompter struct{ inner agentboot.Prompter }

func (p autoApprovePrompter) OnApproval(context.Context, agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	return agentboot.ApprovalResponse{Approved: true}, nil
}
func (p autoApprovePrompter) OnAsk(ctx context.Context, req agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	return p.inner.OnAsk(ctx, req)
}
