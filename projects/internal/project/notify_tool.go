package project

import (
	"context"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// toolNotifyCoordinator is how a thread reaches its project's coordinator
// before its turn ends. It is the mirror of message_thread, and the wrapped
// replacement for Claude Code's own cross-session SendMessage, which a
// Projects session may not use (see thread-coordinator-channel.md).
const toolNotifyCoordinator = "notify_coordinator"

func (f *sessionToolsMCP) notifyTools() []Tool {
	c := f.service.Coordinator
	return []Tool{
		{Name: toolNotifyCoordinator, ThreadOnly: true, Strict: true,
			Description: "Tell this project's coordinator something now, without ending your turn: work you found that needs its own thread, a blocker the coordinator can route, a fact other threads need. It reads it as information after its current turn; it is not the user and cannot approve anything, and you get no answer here. To ask for a new thread, say what it should do and why: the coordinator offers it to the user as a card, and the user decides. Your report at the end of the turn still goes to the coordinator on its own.",
			Properties:  map[string]any{"message": strProp("What the coordinator should know, in a few sentences (at most 2000 characters).")},
			Required:    []string{"message"},
			Handle: threadTool(f, func(ctx context.Context, t *db.Thread, in struct{ Message string }) (string, error) {
				return c.NotifyFromThread(ctx, t, in.Message)
			})},
	}
}
