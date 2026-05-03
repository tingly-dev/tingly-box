// Package scenario defines the back-end plugin contract of the remote
// middle layer. A Scenario is a named provider of "content" that flows
// through a Channel to a human (Claude Code hooks, agent chat, future
// PR-review or CI-gate plugins).
//
// Plugins receive an Event from a Source (HTTP, in-process, future
// RPC) and use a Runtime to resolve channels, send notifications, ask
// questions, and emit audit records. The Outcome the plugin returns
// drives the Source's response (e.g. the notify HTTP endpoint maps
// non-empty InteractionID to a 202 + wait URL).
package scenario

import (
	"context"
	"time"
)

// Event is a normalized trigger handed to a Scenario. Source identifies
// where the trigger originated; Payload carries the original event
// shape (e.g. the Claude Code hook JSON marshaled to a map).
type Event struct {
	// Source is one of "http", "inproc", or future "rpc" markers.
	Source string
	// Scenario is the registered plugin name (matches the URL path
	// parameter in /tingly/:scenario/notify).
	Scenario string
	// Payload is the raw event data as a JSON-friendly map.
	Payload map[string]any
	// Meta carries cross-cutting hints (request id, trace id, …).
	Meta map[string]any
}

// Outcome is the synchronous result of Scenario.Trigger.
//
// Handled tells the source whether the plugin took ownership of this
// event. False means the plugin chose not to handle (e.g. no binding):
// the source may fall through to a default behavior such as a desktop
// notification.
//
// If InteractionID is non-empty the plugin has spawned an asynchronous
// producer (typically an Ask goroutine) and the source should hand the
// caller a wait URL keyed on that ID. The actual decision is delivered
// later via interaction.Registry.Resolve and surfaced through the long-
// poll endpoint. Handled is implicitly true in this case.
//
// If InteractionID is empty and Handled is true the trigger was push-
// only (no human reply expected) and Decision is the immediate response
// the source may surface.
type Outcome struct {
	Handled       bool
	InteractionID string
	ExpiresAt     time.Time
	Decision      map[string]any
	Reason        string
}

// IsInteractive reports whether the outcome started an asynchronous
// interaction (the source should reply 202 + wait URL).
func (o Outcome) IsInteractive() bool { return o.InteractionID != "" }

// Scenario is the plugin contract. Implementations must be safe for
// concurrent Trigger calls.
type Scenario interface {
	// Name is the unique identifier (matches /tingly/:scenario URL).
	Name() string
	// Trigger handles one event. The call should return promptly; long-
	// running interactive flows must be spawned in a goroutine that
	// resolves the matching interaction.Registry entry.
	Trigger(ctx context.Context, ev Event, rt Runtime) (Outcome, error)
}
