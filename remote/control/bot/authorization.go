package bot

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/access"
)

type AccessStore interface {
	access.FactSource
	GetCapability(context.Context, string, access.CapabilityName) (access.BotCapability, bool, error)
	DiscoverDirectChat(context.Context, string, string, string) (access.DirectChat, error)
	GetDirectChat(context.Context, string, string) (access.DirectChat, bool, error)
	PairDirectChat(context.Context, string, string, string) error
	DiscoverGroup(context.Context, string, string, string, string) (access.Group, error)
	UpsertActor(context.Context, access.Actor) (access.Actor, error)
}

func isPairingBootstrap(msg imbot.Message) bool {
	if !msg.IsDirectMessage() || !msg.IsTextContent() {
		return false
	}
	cmd, ok := imbot.ParseSlashCommand(msg.GetText(), false)
	return ok && (cmd.Name == "pair" || cmd.Name == "bind")
}

// ParseListNumber reads "2" from a command's argument: the number of an entry
// in the list the chat was last shown (/dir, /project). ok is false for
// anything else, and the argument is then a path. The gate and the command
// handlers share this, so what the gate lets through as "pick from the list" is
// exactly what the handler treats as a pick.
func ParseListNumber(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
		if n > 1<<20 {
			return 0, false
		}
	}
	if n < 1 {
		return 0, false
	}
	return n, true
}

// pickerAction is what choosing among the folders and projects the operator
// registered needs: start in a direct chat, privileged in a group, where every
// member shares the one working folder. Naming a free path (/cd, /dir <path>)
// or browsing the filesystem always needs privileged.
func pickerAction(group bool) access.ActionName {
	if group {
		return access.ActionRemoteControlPrivileged
	}
	return access.ActionRemoteControlStart
}

// commandAction is the action a folder or project command needs, or "" for any
// other command.
func commandAction(cmd imbot.SlashCommand, group bool) access.ActionName {
	switch cmd.Name {
	case "cd":
		return access.ActionRemoteControlPrivileged
	case "dir":
		if _, isNumber := ParseListNumber(cmd.Args); isNumber || cmd.Args == "" {
			return pickerAction(group)
		}
		return access.ActionRemoteControlPrivileged
	case "project":
		return pickerAction(group)
	}
	return ""
}

// buttonAction is the same for buttons: the folder list and the menu's project
// button are pickers, the directory browser is not.
func buttonAction(payload imbot.Payload, group bool) access.ActionName {
	name := payload.Name()
	if name == "action" { // a menu button: action:<name>
		name = payload.Arg(1)
	}
	switch name {
	case "bind":
		return access.ActionRemoteControlPrivileged
	case "project":
		return pickerAction(group)
	}
	return ""
}

func inboundAction(msg imbot.Message, pendingCapability access.CapabilityName, pendingAction access.ActionName) (access.CapabilityName, access.ActionName) {
	group := msg.IsGroupMessage()
	// Folder and project navigation is decided before a pending approval, so
	// an approval prompt waiting in the chat never turns /cd into an approve.
	if msg.IsCallback() {
		if action := buttonAction(msg.Payload, group); action != "" {
			return access.CapabilityRemoteControl, action
		}
	}
	if msg.IsMediaContent() {
		return access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged
	}
	if cmd, ok := imbot.ParseSlashCommand(msg.GetText(), group); ok {
		if action := commandAction(cmd, group); action != "" {
			return access.CapabilityRemoteControl, action
		}
	}
	// A Projects/Claude approval button or an ordinary reply to a pending
	// approval uses the approval action rather than the session-start action.
	if msg.IsCallback() && (msg.Payload.Name() == "perm" || msg.Payload.Name() == "pj") || pendingAction != "" {
		if pendingCapability == "" {
			pendingCapability = access.CapabilityRemoteControl
		}
		if pendingAction == "" {
			pendingAction = access.ActionRemoteControlApprove
		}
		return pendingCapability, pendingAction
	}
	return access.CapabilityRemoteControl, access.ActionRemoteControlStart
}

// authorizationGate is the one production inbound seam for text, callbacks,
// and files. A denied message is claimed here and cannot reach a Capability.
// AuthorizationGate builds the inbound authorization seam. notifyDenied, when
// non-nil, is invoked with the denied action for two denials that must not be
// silent:
//   - a message answering this bot's own pending prompt (pendingForChat
//     reported a pending action for the chat): the bot itself posted the
//     prompt, so telling the sender they cannot answer it leaks nothing and
//     prevents a dead-end where the prompt hangs until timeout.
//   - a privileged command or button (/cd, /dir, folder buttons) from a chat
//     that may otherwise use the bot (it is allowed to start): a paired chat
//     is not privileged until the operator says so, and without a word the
//     command just looks broken. A chat that cannot even start still gets
//     nothing.
func AuthorizationGate(store AccessStore, authorizer access.Authorizer, legacyChats ChatStoreInterface, requirePairing bool, pendingForChat func(string) (access.CapabilityName, access.ActionName), notifyDenied func(msg imbot.Message, platform imbot.Platform, botUUID string, action access.ActionName)) OnMessage {
	return func(msg imbot.Message, platform imbot.Platform, botUUID string) bool {
		if store == nil || authorizer == nil {
			return false
		}
		ctx := context.Background()
		externalTarget := msg.GetReplyTarget()
		externalActor := strings.TrimSpace(msg.Sender.ID)
		if externalTarget == "" || externalActor == "" {
			return true
		}
		actor, err := store.UpsertActor(ctx, access.Actor{BotUUID: botUUID, Platform: string(platform), ExternalActorID: externalActor, DisplayName: msg.Sender.DisplayName})
		if err != nil {
			logrus.WithError(err).Warn("bot access: resolve actor")
			return true
		}
		var target access.TargetRef
		if msg.IsGroupMessage() {
			group, err := store.DiscoverGroup(ctx, botUUID, string(platform), externalTarget, msg.Recipient.DisplayName)
			if err != nil {
				logrus.WithError(err).Warn("bot access: resolve group")
				return true
			}
			target = access.TargetRef{Kind: access.TargetGroup, ID: group.ID}
		} else if msg.IsDirectMessage() {
			chat, err := store.DiscoverDirectChat(ctx, botUUID, string(platform), externalTarget)
			if err != nil {
				logrus.WithError(err).Warn("bot access: resolve direct chat")
				return true
			}
			target = access.TargetRef{Kind: access.TargetDirectChat, ID: chat.ID}
			if isPairingBootstrap(msg) {
				return false
			}
			if chat.PeerActorID == "" {
				legacyPaired := legacyChats != nil && legacyChats.IsChatPaired(externalTarget, botUUID)
				// Pairing-disabled Bots historically accepted their direct peer
				// immediately. Materialize that trust as explicit policy instead
				// of leaving the new authorizer stuck on discovered-chat deny.
				if legacyPaired || !requirePairing {
					if err := store.PairDirectChat(ctx, chat.ID, externalActor, msg.Sender.DisplayName); err != nil {
						logrus.WithError(err).Warn("bot access: establish direct peer")
						return true
					}
				}
			}
		} else {
			return true
		}
		var pendingCapability access.CapabilityName
		var pendingAction access.ActionName
		if pendingForChat != nil {
			pendingCapability, pendingAction = pendingForChat(externalTarget)
		}
		capability, action := inboundAction(msg, pendingCapability, pendingAction)
		decision := authorizer.Evaluate(ctx, access.AuthorizationRequest{BotUUID: botUUID, Target: target, ActorID: actor.ID, Capability: capability, Action: action})
		fields := logrus.Fields{"event": "bot.authorization.decision", "bot_uuid": botUUID, "capability": capability, "action": action, "target_kind": target.Kind, "target_id": target.ID, "actor_id": actor.ID, "allowed": decision.Allowed, "failed_gate": decision.FailedGate, "reason": decision.Reason}
		if decision.Allowed {
			logrus.WithFields(fields).Debug("bot authorization decision")
			return false
		}
		if pendingAction != "" {
			logrus.WithFields(fields).Warn("bot authorization denied a reply to this bot's pending prompt")
			if notifyDenied != nil {
				notifyDenied(msg, platform, botUUID, action)
			}
		} else {
			logrus.WithFields(fields).Debug("bot authorization decision")
			// A file posted in a group is ordinary chatter, not a request to the
			// bot; only commands and buttons there earn a reply.
			chatter := msg.IsGroupMessage() && msg.IsMediaContent()
			if action == access.ActionRemoteControlPrivileged && notifyDenied != nil && !chatter {
				start := authorizer.Evaluate(ctx, access.AuthorizationRequest{BotUUID: botUUID, Target: target, ActorID: actor.ID, Capability: capability, Action: access.ActionRemoteControlStart})
				if start.Allowed {
					notifyDenied(msg, platform, botUUID, action)
				}
			}
		}
		return true
	}
}

// DeniedActionText is what a chat is told when its action is denied.
func DeniedActionText(action access.ActionName) string {
	if action == access.ActionRemoteControlPrivileged {
		return "⚠️ This needs the “Any folder and files” permission, which this chat does not have. Ask the bot owner to turn it on for this chat in Tingly Box (Remote > Who can control this bot)."
	}
	return "⚠️ You are not authorized to answer this request. Ask the bot owner to grant the approve permission for this chat."
}
