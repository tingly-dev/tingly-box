package bot

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/access"
)

// PrivilegeChecker reports whether a chat's sender may use remote_control's
// privileged actions (a free folder path, files; in a group, every folder and
// project command). /help uses it to list only what the chat can run. The
// gate stays the one place that refuses a command; this only answers a
// question about the same policy.
type PrivilegeChecker func(platform imbot.Platform, botUUID, chatID, senderID string, group bool) bool

type privilegeCheckerKey struct{}

// WithPrivilegeChecker hands a bot's consumers the checker of the host's
// access policy through the context they are attached with.
func WithPrivilegeChecker(ctx context.Context, check PrivilegeChecker) context.Context {
	return context.WithValue(ctx, privilegeCheckerKey{}, check)
}

// PrivilegeCheckerFrom returns the checker WithPrivilegeChecker stored, or nil
// when the host has no access policy.
func PrivilegeCheckerFrom(ctx context.Context) PrivilegeChecker {
	check, _ := ctx.Value(privilegeCheckerKey{}).(PrivilegeChecker)
	return check
}

// NewPrivilegeChecker answers from the same authorizer as AuthorizationGate.
// A failure to resolve the chat or sender is "no".
func NewPrivilegeChecker(store AccessStore, authorizer access.Authorizer) PrivilegeChecker {
	return func(platform imbot.Platform, botUUID, chatID, senderID string, group bool) bool {
		if store == nil || authorizer == nil || chatID == "" || senderID == "" {
			return false
		}
		ctx := context.Background()
		actor, err := store.UpsertActor(ctx, access.Actor{BotUUID: botUUID, Platform: string(platform), ExternalActorID: senderID})
		if err != nil {
			return false
		}
		var target access.TargetRef
		if group {
			g, err := store.DiscoverGroup(ctx, botUUID, string(platform), chatID, "")
			if err != nil {
				return false
			}
			target = access.TargetRef{Kind: access.TargetGroup, ID: g.ID}
		} else {
			c, err := store.DiscoverDirectChat(ctx, botUUID, string(platform), chatID)
			if err != nil {
				return false
			}
			target = access.TargetRef{Kind: access.TargetDirectChat, ID: c.ID}
		}
		return authorizer.Evaluate(ctx, access.AuthorizationRequest{
			BotUUID: botUUID, Target: target, ActorID: actor.ID,
			Capability: access.CapabilityRemoteControl, Action: access.ActionRemoteControlPrivileged,
		}).Allowed
	}
}
