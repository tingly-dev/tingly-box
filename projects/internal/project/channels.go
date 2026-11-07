package project

import (
	"time"

	"github.com/tingly-dev/tingly-box/remote/agent"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// OutboxHost is an optional Host capability: the host can deliver to IM chats.
// Projects asks it for the agent.Outbox it speaks through (the replies of a
// project's coordinator, thread reports, approvals), and the standalone
// binary, which has no IM, keeps compiling and simply never sends. See
// .design/projects-im.md.
type OutboxHost interface {
	// Outbox returns the outbox, or nil while the host has none.
	Outbox() agent.Outbox
}

// Values of the buttons of an approval (questions are answered by replying).
const (
	ChoiceAllow       = interaction.ChoiceAllow
	ChoiceAllowThread = interaction.ChoiceAllowThread
	ChoiceDeny        = interaction.ChoiceDeny
)

// Limits for what the relay says in a chat.
const (
	// relayLineMax caps a folded thread report, which is one line in a chat.
	relayLineMax     = 240
	relaySendTimeout = 30 * time.Second
)
