// Package interaction defines the domain-neutral request / reply / result
// types that flow between scenarios (back-end content providers) and
// channels (human-facing surfaces) inside the remote middle layer.
//
// All types are JSON-friendly so a future out-of-process plugin transport
// (HTTP / gRPC) can use the same contract without redesign.
package interaction

import "time"

// Kind classifies how an interaction should be presented and answered.
type Kind string

const (
	// KindNotify is a one-way push (no reply expected).
	KindNotify Kind = "notify"
	// KindConfirm is a yes/no/cancel-style approval.
	KindConfirm Kind = "confirm"
	// KindChoose is a "pick one of N" selection.
	KindChoose Kind = "choose"
	// KindAsk is a free-form text question.
	KindAsk Kind = "ask"
)

// Status reports how an interaction concluded.
type Status string

const (
	// StatusAnswered means the human selected an option or replied.
	StatusAnswered Status = "answered"
	// StatusCancelled means the human explicitly cancelled.
	StatusCancelled Status = "cancelled"
	// StatusTimeout means the budget elapsed before any reply.
	StatusTimeout Status = "timeout"
	// StatusError means the channel failed to deliver or read a reply.
	StatusError Status = "error"
)

// Interaction is the domain-neutral request a scenario sends to a human
// through a Channel. Channels are responsible for rendering it natively
// (buttons on Telegram, blocks on Slack, plain numbered text fallback).
type Interaction struct {
	ID      string         `json:"id"`
	Kind    Kind           `json:"kind"`
	Title   string         `json:"title"`
	Body    string         `json:"body,omitempty"`
	Options []Option       `json:"options,omitempty"`
	Timeout time.Duration  `json:"timeout,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

// Option is a selectable choice presented to the human.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
	// Style is an optional render hint: "default" | "primary" | "danger".
	Style string `json:"style,omitempty"`
}

// Reply is what the human sent back through a Channel.
type Reply struct {
	InteractionID string `json:"interaction_id"`
	Status        Status `json:"status"`
	// Selected carries the chosen Option.Value when applicable.
	Selected string `json:"selected,omitempty"`
	// FreeText carries free-form replies (KindAsk) or supplementary input.
	FreeText string `json:"free_text,omitempty"`
	// Meta is scenario-specific extra data the channel surfaced.
	Meta map[string]any `json:"meta,omitempty"`
}

// Notification is a one-way push (no reply expected). Title may be empty;
// channels should default to a sensible header.
type Notification struct {
	Title string         `json:"title,omitempty"`
	Body  string         `json:"body"`
	Meta  map[string]any `json:"meta,omitempty"`
}

// Result is the final outcome a scenario delivers to the long-poll waiter.
// Decision is scenario-defined (e.g. Claude Code's hookSpecificOutput map).
type Result struct {
	Status   Status         `json:"status"`
	Decision map[string]any `json:"decision,omitempty"`
	Reason   string         `json:"reason,omitempty"`
}
