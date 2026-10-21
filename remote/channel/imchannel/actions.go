package imchannel

import (
	"context"
	"errors"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot"
)

// Button is one control of a message sent with SendActions: what it reads
// and the payload segments a press delivers to the bot's callback handler.
type Button struct {
	Label   string
	Payload []string
}

// ErrNoButtons means the channel's platform cannot show buttons; the caller
// sends text instead.
var ErrNoButtons = errors.New("imchannel: the platform has no buttons")

// SendActions sends text with one button per row and returns the sent
// message's id, for Restate. Unlike Prompt it does not wait for a press: the
// press arrives as an ordinary callback, and whoever owns the payload's name
// answers it.
func (c *Channel) SendActions(ctx context.Context, chatID, text string, buttons []Button) (string, error) {
	if c.sender == nil {
		return "", fmt.Errorf("imchannel: no sender")
	}
	if !imbot.GetPlatformCapabilities(c.platform).SupportsInteraction() {
		return "", ErrNoButtons
	}
	kb := imbot.NewKeyboardBuilder()
	for _, b := range buttons {
		kb.AddRow(imbot.ActionButton(b.Label, b.Payload...))
	}
	res, err := c.sender.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		Actions:   kb.BuildActions(),
		Metadata:  c.tokenMetadata(chatID),
	})
	if err != nil {
		return "", err
	}
	return res.MessageID, nil
}

// Restate replaces a message sent with SendActions with text and no buttons.
// A platform that cannot restate gets text as a new message instead, so the
// chat still learns the outcome.
func (c *Channel) Restate(ctx context.Context, chatID, messageID, text string) error {
	if c.sender == nil {
		return fmt.Errorf("imchannel: no sender")
	}
	if r, ok := c.sender.(imbot.MessageRestater); ok && messageID != "" {
		ref := imbot.MessageRef{ChatID: chatID, MessageID: messageID}
		if err := r.Restate(ctx, ref, imbot.RestateOptions{Text: text, ParseMode: imbot.ParseModeMarkdown}); err == nil {
			return nil
		}
	}
	_, err := c.sender.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		Metadata:  c.tokenMetadata(chatID),
	})
	return err
}
