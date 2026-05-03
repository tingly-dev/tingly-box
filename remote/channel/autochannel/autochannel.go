// Package autochannel provides a non-IM Channel implementation for
// headless / CI / programmatic setups. It synthesizes Reply objects
// from a configured Policy without involving any human or IM platform.
//
// It exists to validate the channel.Channel abstraction against a
// non-IM backend (zero imbot or IM-platform code is touched here) and
// to give headless tingly-box deployments a way to honor scenario
// plugins via deterministic auto-decisions.
package autochannel

import (
	"context"
	"sync"

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// Decision values for the policy fields.
const (
	DecisionAllow     = "allow"
	DecisionDeny      = "deny"
	DecisionAutoFirst = "auto-first"
	DecisionCancel    = "cancel"
)

// Policy decides what an autochannel reply looks like for each
// interaction kind.
type Policy struct {
	// OnPermission handles KindConfirm / KindNotify-as-confirm: one of
	// "allow" | "deny". Empty defaults to "deny" (safest).
	OnPermission string
	// OnQuestion handles KindChoose / KindAsk: one of "auto-first" |
	// "cancel". "auto-first" picks Options[0].Value (or empty if none);
	// "cancel" returns StatusCancelled. Empty defaults to "cancel".
	OnQuestion string
}

// Sink is invoked on every Send / Prompt to surface activity to
// observability. May be nil to discard.
type Sink func(action string, fields map[string]any)

// Channel is a Channel implementation that auto-decides per Policy.
type Channel struct {
	id       string
	platform string
	policy   Policy
	sink     Sink
	mu       sync.Mutex
}

// New constructs an autochannel. id is the registry identifier (e.g.
// "auto"); policy controls the auto-decision behavior; sink may be nil.
func New(id string, policy Policy, sink Sink) *Channel {
	return &Channel{id: id, platform: "auto", policy: policy, sink: sink}
}

// ID returns the channel identifier.
func (c *Channel) ID() string { return c.id }

// Platform returns "auto".
func (c *Channel) Platform() string { return c.platform }

// Capabilities reports the auto-channel's affordances. It does not
// support buttons or message editing because no human is reading.
func (c *Channel) Capabilities() channel.Capabilities {
	return channel.Capabilities{Buttons: false, EditMessages: false, Markdown: false}
}

// Send records the notification to the sink and returns immediately.
func (c *Channel) Send(_ context.Context, target channel.Target, msg interaction.Notification) error {
	c.fire("autochannel.send", map[string]any{
		"chat_id": target.ChatID,
		"title":   msg.Title,
		"body":    msg.Body,
	})
	return nil
}

// Prompt synthesizes a Reply per Policy and returns immediately.
// ctx and ix.Timeout are honored only insofar as already-cancelled
// contexts return ctx.Err() — the auto path itself does not block.
func (c *Channel) Prompt(ctx context.Context, target channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
	if err := ctx.Err(); err != nil {
		return interaction.Reply{InteractionID: ix.ID, Status: interaction.StatusError}, err
	}
	reply := c.decide(ix)
	c.fire("autochannel.prompt", map[string]any{
		"chat_id":        target.ChatID,
		"interaction_id": ix.ID,
		"kind":           string(ix.Kind),
		"title":          ix.Title,
		"reply_status":   string(reply.Status),
		"reply_selected": reply.Selected,
	})
	return reply, nil
}

// decide builds the Reply from the policy. Pure function — exposed for
// reuse in tests.
func (c *Channel) decide(ix interaction.Interaction) interaction.Reply {
	reply := interaction.Reply{InteractionID: ix.ID}
	switch ix.Kind {
	case interaction.KindAsk, interaction.KindChoose:
		switch normalize(c.policy.OnQuestion, DecisionCancel) {
		case DecisionAutoFirst:
			reply.Status = interaction.StatusAnswered
			if len(ix.Options) > 0 {
				reply.Selected = ix.Options[0].Value
			}
		default:
			reply.Status = interaction.StatusCancelled
		}
	default:
		switch normalize(c.policy.OnPermission, DecisionDeny) {
		case DecisionAllow:
			reply.Status = interaction.StatusAnswered
			reply.Selected = DecisionAllow
		default:
			reply.Status = interaction.StatusAnswered
			reply.Selected = DecisionDeny
		}
	}
	reply.FreeText = "auto-decided by autochannel policy"
	return reply
}

func (c *Channel) fire(action string, fields map[string]any) {
	if c.sink == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sink(action, fields)
}

func normalize(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// Compile-time check that *Channel implements channel.Channel.
var _ channel.Channel = (*Channel)(nil)
