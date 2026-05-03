package scenario

import (
	"context"

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// Runtime is what scenarios see at execution time. The interface is
// deliberately minimal: scenarios should not reach for global state
// directly; they ask the runtime for channels, push notifications, run
// interactive prompts, and emit audit records.
type Runtime interface {
	// Resolve returns the channel + target the event should be routed
	// to, based on the bound binding for the scenario. ok=false means
	// no binding exists for this event (the source should fall back).
	Resolve(ctx context.Context, ev Event) (channel.Channel, channel.Target, bool, error)

	// Notify pushes a one-way message to the channel. It is best-effort
	// fire-and-forget; the caller does not block on delivery.
	Notify(ctx context.Context, ch channel.Channel, target channel.Target, msg interaction.Notification) error

	// Ask runs an interactive prompt synchronously. The returned Reply
	// is the human's response (or an error on cancel/timeout/transport
	// failure). Scenarios typically call this from a goroutine they
	// spawned in Trigger.
	Ask(ctx context.Context, ch channel.Channel, target channel.Target, ix interaction.Interaction) (interaction.Reply, error)

	// Audit emits a structured audit record for the action.
	Audit(action string, fields map[string]any)
}
