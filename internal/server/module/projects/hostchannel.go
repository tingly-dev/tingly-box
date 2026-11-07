package projects

import "github.com/tingly-dev/tingly-box/remote/agent"

var _ interface{ Outbox() agent.Outbox } = (*Host)(nil)

// SetChatBridge gives the host the bridge whose outbox Projects speaks through.
func (h *Host) SetChatBridge(b *ChatBridge) { h.out = b.Outbox() }

// Outbox is the host's half of Projects' optional OutboxHost: the coordinator's
// replies, thread reports and approvals reach the IM chats bound to a project.
// It is nil until the bridge is set.
func (h *Host) Outbox() agent.Outbox { return h.out }
