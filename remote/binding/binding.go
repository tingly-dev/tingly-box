// Package binding describes which channel a scenario should use for a
// given event. Bindings live alongside bot settings (see
// internal/data/db.Settings.Scenarios) as a JSON-encoded list per bot.
//
// The binding type is generic across scenarios — fields beyond Name /
// ChatID / Events are stored as a free-form Options map so new
// scenarios can add scenario-specific settings without changing the
// schema or the generic resolver.
package binding

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// Binding declares how a single bot serves a named scenario.
type Binding struct {
	// Name matches the scenario plugin Name() and the :scenario URL
	// segment of /tingly/:scenario/...
	Name string `json:"name"`
	// ChatID is the IM chat the bot routes to. Channels look this up
	// when delivering Send / Prompt.
	ChatID string `json:"chat_id"`
	// Events optionally restricts which event names this binding
	// handles. Empty list = all events.
	Events []string `json:"events,omitempty"`
	// Options carries scenario-specific configuration the resolver
	// returns verbatim to the plugin (e.g. permission policy for the
	// claude_code scenario).
	Options map[string]any `json:"-"`
}

// Resolved is what the resolver returns to the plugin: the binding
// itself plus the bot identity needed to look up a Channel.
type Resolved struct {
	Binding  Binding
	BotUUID  string
	Platform string
	BotName  string
}

// Store is the subset of the imbot settings store the resolver needs.
// Defining it as an interface keeps the resolver testable.
type Store interface {
	ListEnabledSettings() ([]db.Settings, error)
}

// Resolver matches (scenario, event) to a single bot binding by
// scanning enabled bot settings. Read-only and safe for concurrent use.
type Resolver struct {
	store Store
}

// NewResolver constructs a resolver backed by the given store.
func NewResolver(store Store) *Resolver { return &Resolver{store: store} }

// Resolve returns the first enabled bot whose binding matches scenario
// + event. ok=false means no binding exists; err is non-nil only on
// store failures.
func (r *Resolver) Resolve(scenario, event string) (*Resolved, error) {
	if r == nil || r.store == nil || scenario == "" {
		return nil, nil
	}
	settings, err := r.store.ListEnabledSettings()
	if err != nil {
		return nil, fmt.Errorf("list enabled settings: %w", err)
	}
	for _, s := range settings {
		bindings, err := parse(s.Scenarios)
		if err != nil {
			// One malformed row should not stop the rest.
			continue
		}
		for _, b := range bindings {
			if b.Name != scenario {
				continue
			}
			if !eventAllowed(b.Events, event) {
				continue
			}
			return &Resolved{
				Binding:  b,
				BotUUID:  s.UUID,
				Platform: s.Platform,
				BotName:  s.Name,
			}, nil
		}
	}
	return nil, nil
}

// rawBinding mirrors Binding but uses a free-form map[string]json.RawMessage
// so we can keep all unknown fields in Options.
type rawBinding map[string]json.RawMessage

func parse(raw string) ([]Binding, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var rows []rawBinding
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, fmt.Errorf("parse scenarios: %w", err)
	}
	out := make([]Binding, 0, len(rows))
	for _, row := range rows {
		b := Binding{Options: map[string]any{}}
		for k, v := range row {
			switch k {
			case "name":
				_ = json.Unmarshal(v, &b.Name)
			case "chat_id":
				_ = json.Unmarshal(v, &b.ChatID)
			case "events":
				_ = json.Unmarshal(v, &b.Events)
			default:
				var anyVal any
				if err := json.Unmarshal(v, &anyVal); err == nil {
					b.Options[k] = anyVal
				}
			}
		}
		out = append(out, b)
	}
	return out, nil
}

func eventAllowed(events []string, event string) bool {
	if len(events) == 0 {
		return true
	}
	for _, e := range events {
		if strings.EqualFold(e, event) {
			return true
		}
	}
	return false
}
