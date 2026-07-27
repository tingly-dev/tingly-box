package remoteagent

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

const trackActionMenuIDKey = "_trackActionMenuID"

// trackActionMenuMetadata marks an outbound message as the current action-menu
// message, so its ID is recorded and the menu can be taken down later.
func trackActionMenuMetadata() map[string]interface{} {
	return map[string]interface{}{
		trackActionMenuIDKey: true,
	}
}

// Forwarding of the inbound reply-context token lives in the host package
// (bot.ForwardReplyContext) — the prompt-reply router needs it too.
