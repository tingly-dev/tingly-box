// Package imchannel adapts an IM bot (one of the platforms in
// github.com/tingly-dev/tingly-box/imbot) into the
// internal/remote/channel.Channel contract.
//
// Send maps to imbot.Bot.SendMessage. Prompt translates the
// channel-neutral interaction.Interaction into the ask.Request shape
// the existing IMPrompter understands, then translates the resulting
// ask.Result back into an interaction.Reply. This keeps the rich,
// platform-aware prompting logic (button cards, text-mode fallback,
// AskUserQuestion multi-choice rendering) in one place while letting
// the rest of the remote middle layer speak only the JSON-friendly
// interaction.* types.
package imchannel

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// Prompter is the subset of the IM prompter the channel needs. The
// existing remote_control/bot.IMPrompter satisfies this interface.
type Prompter interface {
	Prompt(ctx context.Context, req ask.Request) (ask.Result, error)
}

// Sender is the subset of imbot.Bot needed for one-way notifications.
type Sender interface {
	SendMessage(ctx context.Context, target string, opts *imbot.SendMessageOptions) (*imbot.SendResult, error)
}

// Channel is an imbot-backed channel.Channel implementation.
type Channel struct {
	id       string
	platform string
	sender   Sender
	prompter Prompter
}

// New constructs a Channel. id is typically the bot UUID; platform is
// the imbot platform name ("telegram", "feishu", …); sender wraps
// imbot.Bot for SendMessage; prompter is the IMPrompter that handles
// interactive flows.
func New(id, platform string, sender Sender, prompter Prompter) *Channel {
	return &Channel{
		id:       id,
		platform: platform,
		sender:   sender,
		prompter: prompter,
	}
}

// ID returns the stable channel identifier (bot UUID).
func (c *Channel) ID() string { return c.id }

// Platform returns the imbot platform name.
func (c *Channel) Platform() string { return c.platform }

// Capabilities reports per-platform UI affordances. Today every imbot
// platform supports rich features through the existing prompter, so
// we report all capabilities; future per-platform tuning can refine
// this.
func (c *Channel) Capabilities() channel.Capabilities {
	return channel.Capabilities{
		Buttons:      true,
		EditMessages: true,
		Markdown:     true,
	}
}

// Send delivers a one-way notification.
func (c *Channel) Send(ctx context.Context, target channel.Target, msg interaction.Notification) error {
	if c.sender == nil {
		return fmt.Errorf("imchannel: no sender")
	}
	text := msg.Body
	if msg.Title != "" {
		if text == "" {
			text = msg.Title
		} else {
			text = msg.Title + "\n" + msg.Body
		}
	}
	_, err := c.sender.SendMessage(ctx, target.ChatID, &imbot.SendMessageOptions{Text: text})
	return err
}

// Prompt blocks until the human answers the interaction or ctx /
// ix.Timeout fires. The translation between Interaction and ask.Request
// is intentionally narrow: meta keys recognised by the IMPrompter are
// passed through (notably tool_name / tool_input / session_id /
// agent_type for the Claude Code scenario).
func (c *Channel) Prompt(ctx context.Context, target channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
	if c.prompter == nil {
		return interaction.Reply{}, fmt.Errorf("imchannel: no prompter")
	}
	req := ToAskRequest(c.id, c.platform, target, ix)
	result, err := c.prompter.Prompt(ctx, req)
	if err != nil {
		return interaction.Reply{InteractionID: ix.ID, Status: interaction.StatusError}, err
	}
	return FromAskResult(ix, result), nil
}

// ToAskRequest translates an Interaction + target + channel identity
// into the ask.Request the IMPrompter consumes. Exposed for test cases
// that exercise the translation without spinning up a real bot.
func ToAskRequest(channelID, platform string, target channel.Target, ix interaction.Interaction) ask.Request {
	req := ask.Request{
		ID:       ix.ID,
		ChatID:   target.ChatID,
		Platform: platform,
		BotUUID:  channelID,
		Title:    ix.Title,
		Message:  ix.Body,
		Timeout:  ix.Timeout,
	}
	if req.Message == "" {
		req.Message = ix.Title
	}
	switch ix.Kind {
	case interaction.KindAsk, interaction.KindChoose:
		req.Type = ask.TypeQuestion
	default:
		req.Type = ask.TypePermission
	}
	if v, ok := ix.Meta["agent_type"].(string); ok {
		req.AgentType = agentboot.AgentType(v)
	}
	if v, ok := ix.Meta["session_id"].(string); ok {
		req.SessionID = v
	}
	if v, ok := ix.Meta["tool_name"].(string); ok {
		req.ToolName = v
	}
	if v, ok := ix.Meta["tool_input"].(map[string]interface{}); ok {
		req.Input = v
	}
	if v, ok := ix.Meta["reason"].(string); ok {
		req.Reason = v
	}
	return req
}

// FromAskResult converts ask.Result into an interaction.Reply. Cancel
// reasons map to StatusCancelled; everything else is StatusAnswered.
// Reply.Selected carries "allow" / "deny" for permission flows so
// scenarios can branch without inspecting the Reason free-text.
func FromAskResult(ix interaction.Interaction, result ask.Result) interaction.Reply {
	reply := interaction.Reply{
		InteractionID: ix.ID,
		Meta:          map[string]any{},
	}
	if isCancelReason(result.Reason) && !result.Approved {
		reply.Status = interaction.StatusCancelled
	} else {
		reply.Status = interaction.StatusAnswered
	}
	if result.Approved {
		reply.Selected = "allow"
	} else if reply.Status != interaction.StatusCancelled {
		reply.Selected = "deny"
	}
	if result.Reason != "" {
		reply.FreeText = result.Reason
	}
	if result.UpdatedInput != nil {
		reply.Meta["updated_input"] = result.UpdatedInput
	}
	if len(result.Selection) > 0 {
		reply.Meta["selection"] = result.Selection
	}
	if result.Remember {
		reply.Meta["remember"] = true
	}
	return reply
}

func isCancelReason(reason string) bool {
	return reason == "cancel" || reason == "cancelled"
}
