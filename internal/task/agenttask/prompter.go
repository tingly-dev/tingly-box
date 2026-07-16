package agenttask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// pausingPrompter is the unattended attendance mode of the task run core
// (design: .design/task-board.md §5.1). Any Ask/Approval that reaches the
// run is outside the Task's pre-authorized boundary: the prompter records
// the first one as the pause outcome (needs_input for business questions,
// handoff_required for permission requests), denies it, and cancels the
// run so the process ends promptly.
//
// agentboot.RunWithPrompter dispatches events from a single goroutine, so
// no locking is needed.
type pausingPrompter struct {
	cancel func()
	record func(kind, summary string, data json.RawMessage)
	pause  *Result
}

func (p *pausingPrompter) OnAsk(_ context.Context, req agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	if p.pause == nil {
		question := strings.TrimSpace(req.Message)
		if question == "" {
			question = strings.TrimSpace(req.Reason)
		}
		if question == "" {
			question = "The agent needs information before it can continue."
		}
		p.pause = &Result{State: "needs_input", Summary: "The automated run stopped for a business question.", Question: truncate(question)}
		p.pause.ExitReason = "business_input_required"
		p.record("input_required", question, nil)
	}
	p.cancel()
	return agentboot.AskResponse{Approved: false, Reason: "Unattended Task runs cannot answer interactive questions"}, nil
}

func (p *pausingPrompter) OnApproval(_ context.Context, req agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	if p.pause == nil {
		tool := strings.TrimSpace(req.ToolName)
		if tool == "" {
			tool = "a protected tool"
		}
		summary := fmt.Sprintf("Native handoff required: %s requested permission outside this Task's automation boundary.", tool)
		p.pause = &Result{State: "handoff_required", Summary: summary, Question: "Open the native session to review the request, then continue automation when ready."}
		p.pause.ExitReason = "permission_boundary"
		p.record("handoff_required", summary, eventData(map[string]any{"tool": req.ToolName, "reason": req.Reason}))
	}
	p.cancel()
	return agentboot.ApprovalResponse{Approved: false, Reason: "Permission is outside the Task's pre-authorized automation boundary"}, nil
}
