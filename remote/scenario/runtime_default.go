package scenario

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/remote/binding"
	channel2 "github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// AuditFunc is the audit callback the runtime emits to. Production
// wiring connects this to the audit.Logger; tests can pass a closure
// that records into a slice.
type AuditFunc func(action string, fields map[string]any)

// DefaultRuntime is the production Runtime backed by a channel.Registry,
// a binding.Resolver, and an optional AuditFunc.
type DefaultRuntime struct {
	channels *channel2.Registry
	bindings *binding.Resolver
	audit    AuditFunc
}

// NewDefaultRuntime wires the runtime to its dependencies. audit may be
// nil (Audit becomes a no-op).
func NewDefaultRuntime(channels *channel2.Registry, bindings *binding.Resolver, audit AuditFunc) *DefaultRuntime {
	return &DefaultRuntime{channels: channels, bindings: bindings, audit: audit}
}

// Resolve matches the event to a registered channel via the binding
// resolver. ok=false means no binding (the source falls back); err is
// non-nil only on store / configuration faults.
func (r *DefaultRuntime) Resolve(_ context.Context, ev Event) (channel2.Channel, channel2.Target, bool, error) {
	if r == nil || r.bindings == nil {
		return nil, channel2.Target{}, false, nil
	}
	event := eventName(ev)
	resolved, err := r.bindings.Resolve(ev.Scenario, event)
	if err != nil {
		return nil, channel2.Target{}, false, err
	}
	if resolved == nil {
		return nil, channel2.Target{}, false, nil
	}
	ch, ok := r.channels.Get(resolved.BotUUID)
	if !ok || ch == nil {
		// Bot is bound but not running. Treat as no-binding so the
		// source falls through silently rather than returning a fake
		// success.
		return nil, channel2.Target{}, false, nil
	}
	target := channel2.Target{ChatID: resolved.Binding.ChatID}
	// Stash the resolved binding's options on the event Meta so
	// scenario plugins can read scenario-specific config without a
	// second resolver lookup.
	if ev.Meta == nil {
		ev.Meta = map[string]any{}
	}
	ev.Meta["__binding_options"] = resolved.Binding.Options
	return ch, target, true, nil
}

// Notify pushes a one-way message asynchronously. Errors are logged
// (the source has already responded by the time delivery happens).
func (r *DefaultRuntime) Notify(ctx context.Context, ch channel2.Channel, target channel2.Target, msg interaction.Notification) error {
	if ch == nil {
		return fmt.Errorf("nil channel")
	}
	if err := ch.Send(ctx, target, msg); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"channel":  ch.ID(),
			"platform": ch.Platform(),
		}).Warn("notify push failed")
		return err
	}
	return nil
}

// Ask blocks on the channel's Prompt until a reply arrives or the
// context fires.
func (r *DefaultRuntime) Ask(ctx context.Context, ch channel2.Channel, target channel2.Target, ix interaction.Interaction) (interaction.Reply, error) {
	if ch == nil {
		return interaction.Reply{}, fmt.Errorf("nil channel")
	}
	return ch.Prompt(ctx, target, ix)
}

// Audit records a structured event when an audit sink is wired.
func (r *DefaultRuntime) Audit(action string, fields map[string]any) {
	if r == nil || r.audit == nil {
		return
	}
	r.audit(action, fields)
}

// BindingOptions returns the resolved binding's free-form options map
// from the event Meta, or nil if Resolve has not been called yet.
// Plugins use this to read scenario-specific settings (e.g. permission
// policy) without a second store lookup.
func BindingOptions(ev Event) map[string]any {
	if ev.Meta == nil {
		return nil
	}
	if v, ok := ev.Meta["__binding_options"].(map[string]any); ok {
		return v
	}
	return nil
}

func eventName(ev Event) string {
	if ev.Payload == nil {
		return ""
	}
	if v, ok := ev.Payload["hook_event_name"].(string); ok {
		return v
	}
	if v, ok := ev.Payload["event"].(string); ok {
		return v
	}
	return ""
}
