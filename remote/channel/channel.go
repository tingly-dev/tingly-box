// Package channel defines the human-facing side of the remote middle
// layer: a Channel is a surface (IM bot, web UI, CLI, …) that can
// deliver an interaction.Notification or interaction.Interaction to a
// human and (for interactive ones) collect a Reply.
//
// The Channel interface deliberately speaks the JSON-friendly
// interaction.* types so the same contract can be served over RPC in
// the future.
package channel

import (
	"context"

	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// Target identifies who on a Channel should receive a message.
type Target struct {
	// ChatID is the channel-native conversation/room identifier.
	ChatID string
	// User is the optional channel-native user identifier (for routing
	// to a specific person inside a multi-user chat).
	User string
}

// Capabilities reports what UI affordances a Channel supports. Used by
// scenarios to decide whether to use buttons or fall back to numbered
// text replies.
type Capabilities struct {
	Buttons      bool
	EditMessages bool
	Markdown     bool
}

// Channel is a one-way + interactive message surface for humans.
//
// Implementations must be safe for concurrent use by multiple
// goroutines. Send is fire-and-forget (no reply); Prompt blocks until
// the human replies, the context is cancelled, or the interaction's
// own Timeout elapses.
type Channel interface {
	// ID is the stable identifier this channel is registered under
	// (typically a bot UUID).
	ID() string
	// Platform returns the channel-native platform name ("telegram",
	// "feishu", …) for logging / routing decisions.
	Platform() string
	// Capabilities reports what UI affordances the channel supports.
	Capabilities() Capabilities
	// Send delivers a one-way notification.
	Send(ctx context.Context, target Target, msg interaction.Notification) error
	// Prompt delivers an interactive request and blocks until a reply
	// arrives or the context / timeout fires.
	Prompt(ctx context.Context, target Target, ix interaction.Interaction) (interaction.Reply, error)
}
