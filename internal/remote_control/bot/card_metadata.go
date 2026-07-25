package bot

// Outbound interactive controls now travel as a neutral imbot.ActionSet on
// SendMessageOptions.Actions, and each platform renders them itself.
//
// This file used to build three parallel renderings of the same buttons into
// the metadata bag — a Telegram markup under "replyMarkup", a neutral card
// under "card", and Feishu card JSON under "card_json" — and hope the right
// one got picked up. Two of the three were never read on any send path, and
// the one that was carried a go-telegram type that Feishu could not decode,
// so Feishu users got messages with no buttons at all.
//
// What is left here is the one thing that genuinely is out-of-band metadata:
// the flag asking the send path to remember this message's ID so the action
// menu can be removed later.

import "github.com/tingly-dev/tingly-box/imbot"

const trackActionMenuIDKey = "_trackActionMenuID"

// trackActionMenuMetadata marks an outbound message as the current action-menu
// message, so its ID is recorded and the menu can be taken down later.
func trackActionMenuMetadata() map[string]interface{} {
	return map[string]interface{}{
		trackActionMenuIDKey: true,
	}
}

// forwardReplyContext copies the inbound message's reply-context token onto
// outbound options. Weixin and WeCom tie each reply to the inbound message it
// answers, and drop or misattribute a reply that arrives without the token.
//
// This was five copies of the same block across four files. Consolidating it
// is also what makes Seam 2 a small change later: when the bot learns to carry
// its own reply context, this is the single function that goes away.
//
// TODO(phase-4): this should not be the caller's job at all — the bot knows
// which inbound message it is answering.
func forwardReplyContext(opts *imbot.SendMessageOptions, inbound imbot.Message) {
	if inbound.Metadata == nil {
		return
	}
	token, _ := inbound.Metadata["context_token"].(string)
	if token == "" {
		return
	}
	if opts.Metadata == nil {
		opts.Metadata = make(map[string]interface{})
	}
	opts.Metadata["context_token"] = token
}
