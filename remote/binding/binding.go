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

// RemoteAgentScenario is the mount name for the remote-agent purpose
// (controlling Claude Code / SmartGuide from chat). Unlike outbound scenarios
// (e.g. claude_code hooks) it has no registered plugin — it is an inbound
// mount stored in the same per-bot Scenarios list so a bot's purposes all
// live in one place. See ScenarioMounted for the on/off semantics.
const RemoteAgentScenario = "remote_agent"

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
	// Enabled is the mount switch. nil means "on" so that bindings written
	// before the switch existed (and outbound bindings, which are always
	// active when present) keep working. Set explicitly to false to mount a
	// scenario but keep it turned off.
	Enabled *bool `json:"enabled,omitempty"`
	// Options carries scenario-specific configuration the resolver
	// returns verbatim to the plugin (e.g. permission policy for the
	// claude_code scenario).
	Options map[string]any `json:"-"`
}

// ScenarioMounted reports whether the named scenario is mounted (active) on a
// bot given its raw Scenarios JSON.
//
// A scenario is mounted when an explicit binding with that name is present and
// not turned off (Enabled nil or true). For backward compatibility with bots
// configured before mounts existed, the ABSENCE of a binding for name also
// counts as mounted — otherwise every legacy bot would stop serving. To turn a
// mount off you therefore write an explicit binding with enabled:false rather
// than removing it. A malformed Scenarios blob is treated as mounted so a bad
// row never silently takes a bot offline.
func ScenarioMounted(scenariosJSON, name string) bool {
	bindings, err := parse(scenariosJSON)
	if err != nil {
		return true
	}
	for _, b := range bindings {
		if b.Name == name {
			return b.Enabled == nil || *b.Enabled
		}
	}
	return true
}

// OutboundScenarioMounted reports whether the bot serves the notify purpose:
// it has at least one outbound scenario binding (any name other than the
// remote_agent inbound mount) that is not turned off. This is the mount
// predicate for the notify consumer — a bot with only outbound bindings runs
// as a pure notification/interaction surface even when remote_agent is off.
//
// Unlike ScenarioMounted, absence does NOT count as mounted: a bot with no
// outbound bindings has nothing for the resolver to route to it, so registering
// a channel would be dead weight. A malformed blob likewise counts as not
// mounted here — the remote_agent side already fails open, which keeps the bot
// online for diagnosis.
func OutboundScenarioMounted(scenariosJSON string) bool {
	bindings, err := parse(scenariosJSON)
	if err != nil {
		return false
	}
	for _, b := range bindings {
		if b.Name == "" || b.Name == RemoteAgentScenario {
			continue
		}
		if b.Enabled == nil || *b.Enabled {
			return true
		}
	}
	return false
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

// SetScenarioEnabled returns scenariosJSON with the named scenario's mount set
// to enabled. If a binding with that name exists its enabled flag is updated in
// place; otherwise a new {name, enabled} binding is appended. All other
// bindings and their fields (chat_id, options, …) are preserved verbatim,
// because the operation is done on the raw object list rather than the typed
// Binding (which drops unknown fields into Options).
func SetScenarioEnabled(scenariosJSON, name string, enabled bool) (string, error) {
	var rows []map[string]json.RawMessage
	if trimmed := strings.TrimSpace(scenariosJSON); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
			return scenariosJSON, fmt.Errorf("parse scenarios: %w", err)
		}
	}

	enabledRaw, _ := json.Marshal(enabled)
	found := false
	for _, row := range rows {
		var n string
		if raw, ok := row["name"]; ok {
			_ = json.Unmarshal(raw, &n)
		}
		if n == name {
			row["enabled"] = enabledRaw
			found = true
			break
		}
	}
	if !found {
		nameRaw, _ := json.Marshal(name)
		rows = append(rows, map[string]json.RawMessage{"name": nameRaw, "enabled": enabledRaw})
	}

	out, err := json.Marshal(rows)
	if err != nil {
		return scenariosJSON, fmt.Errorf("marshal scenarios: %w", err)
	}
	return string(out), nil
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
			case "enabled":
				var e bool
				if err := json.Unmarshal(v, &e); err == nil {
					b.Enabled = &e
				}
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
