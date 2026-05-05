package agentboot

import (
	"context"

	"github.com/sirupsen/logrus"
)

// Prompter is the consumer-supplied callback the bot layer (or any
// caller of [RunWithPrompter]) provides to satisfy approval and ask
// requests during agent execution.
//
// Contract:
//
//   - Timeout / cancellation. The caller's ctx carries the deadline.
//     On ctx.Done() the Prompter MUST return promptly with
//     Approved=false (i.e., default-deny). Implementations are free to
//     enforce an additional internal timeout (IMPrompter uses 5min by
//     default) but ctx is authoritative.
//
//   - AlwaysAllow caching. If the user previously approved a tool with
//     "remember", subsequent OnApproval calls for the same tool MUST
//     short-circuit to Approved=true without prompting again. The
//     Prompter owns this cache; executors do not maintain per-tool
//     allowlists.
//
//   - No partial failures. On internal error the Prompter returns the
//     error AND a deny result (Approved=false), so [RunWithPrompter]
//     always has a safe response to send to the agent.
//
// Implementations: IMPrompter (production) and prompt.FakePrompter
// (tests).
type Prompter interface {
	OnApproval(ctx context.Context, req PermissionRequest) (PermissionResult, error)
	OnAsk(ctx context.Context, req AskRequest) (AskResult, error)
}

// MessageSink receives the [MessageEvent.Raw] value of each message event,
// in order. Pass nil to drop message events (e.g. when only completion
// matters).
type MessageSink func(any)

// RunWithPrompter is the convenience consumer of an [ExecutionHandle].
//
// It iterates handle.Events() in order, dispatching:
//   - MessageEvent → sink (if non-nil)
//   - ApprovalRequestEvent → prompter.OnApproval, then handle.Respond
//   - AskRequestEvent → prompter.OnAsk, then handle.Respond
//   - ErrorEvent → logged and ignored
//
// Approval/ask invocations are synchronous within the loop (matching the
// existing IMPrompter blocking semantics — Claude waits for a response
// before emitting more events, so back-pressure is not a concern).
//
// Once the channel closes, RunWithPrompter calls handle.Wait() and returns
// its result.
//
// Use this from executor code that does not need event-level visibility.
// Tests and executors that DO want fine-grained control should iterate
// handle.Events() directly.
func RunWithPrompter(ctx context.Context, h ExecutionHandle, prompter Prompter, sink MessageSink) (*Result, error) {
	for ev := range h.Events() {
		switch e := ev.(type) {
		case MessageEvent:
			if sink != nil {
				sink(e.Raw)
			}

		case ApprovalRequestEvent:
			req := PermissionRequest{
				RequestID: e.ID,
				AgentType: e.AgentType,
				ToolName:  e.ToolName,
				Input:     e.Input,
				Reason:    e.Reason,
				SessionID: e.SessionID,
				BotUUID:   e.BotUUID,
				ChatID:    e.ChatID,
				Platform:  e.Platform,
			}
			res, perr := prompter.OnApproval(ctx, req)
			if perr != nil {
				logrus.WithError(perr).Warn("agentboot.RunWithPrompter: prompter.OnApproval error; denying")
				res = PermissionResult{Approved: false, Reason: perr.Error()}
			}
			if rerr := h.Respond(e.ID, ApprovalResponse{
				Approved:     res.Approved,
				UpdatedInput: res.UpdatedInput,
				Reason:       res.Reason,
			}); rerr != nil {
				logrus.WithError(rerr).Warn("agentboot.RunWithPrompter: Respond error")
			}

		case AskRequestEvent:
			req := AskRequest{
				ID:        e.ID,
				Type:      e.Type,
				AgentType: e.AgentType,
				Platform:  e.Platform,
				ChatID:    e.ChatID,
				BotUUID:   e.BotUUID,
				SessionID: e.SessionID,
				ToolName:  e.ToolName,
				Input:     e.Input,
				Message:   e.Message,
				CallID:    e.CallID,
				Reason:    e.Reason,
			}
			res, aerr := prompter.OnAsk(ctx, req)
			if aerr != nil {
				logrus.WithError(aerr).Warn("agentboot.RunWithPrompter: prompter.OnAsk error; denying")
				res = AskResult{ID: e.ID, Approved: false, Reason: aerr.Error()}
			}
			if rerr := h.Respond(e.ID, AskResponse{
				Approved:     res.Approved,
				UpdatedInput: res.UpdatedInput,
				Reason:       res.Reason,
				Response:     res.Response,
				Selection:    res.Selection,
			}); rerr != nil {
				logrus.WithError(rerr).Warn("agentboot.RunWithPrompter: Respond error")
			}

		case ErrorEvent:
			logrus.WithError(e.Err).Warn("agentboot.RunWithPrompter: agent ErrorEvent")
		}
	}

	return h.Wait()
}
