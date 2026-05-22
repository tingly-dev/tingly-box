package smart_guide

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// CompletionResult reports the outcome of an ExecuteWithHandler run.
type CompletionResult struct {
	Success    bool
	DurationMS int64
	SessionID  string
	Error      string
}

// StreamHandler receives streaming output and the completion signal from
// ExecuteWithHandler. The smart-guide agent runs a tingly-agentscope ReAct
// loop (not agentboot's process pipeline), so it streams intermediate
// messages as plain maps via OnMessage and reports the final outcome via
// OnComplete.
type StreamHandler interface {
	OnMessage(msg any) error
	OnError(err error)
	OnComplete(result *CompletionResult)
}

// Approver answers a permission request for a non-whitelisted command.
// *imchannel.IMPrompter satisfies this via its OnApproval method.
type Approver interface {
	OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error)
}
