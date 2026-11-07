// Package adapter is the host↔remote glue layer. It is the only place
// where main-module persistence types (internal/data/db) meet the pure
// remote library interfaces, keeping the db.Settings type on the host
// side of the boundary.
package adapter

import (
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/remote/binding"
)

// SettingsLister is the read surface the bridge needs from the imbot
// settings store. *db.ImBotSettingsStore satisfies it directly.
type SettingsLister interface {
	ListEnabledSettings() ([]db.Settings, error)
}

// BindingStore adapts a host settings store to binding.Store, mapping
// db.Settings onto the small binding.BotInfo record the resolver
// consumes. This is the single db↔remote bridge.
type BindingStore struct {
	store SettingsLister
}

// NewBindingStore wraps a host settings store as a binding.Store.
func NewBindingStore(store SettingsLister) *BindingStore { return &BindingStore{store: store} }

// ListEnabledBindings implements binding.Store.
func (b *BindingStore) ListEnabledBindings() ([]binding.BotInfo, error) {
	settings, err := b.store.ListEnabledSettings()
	if err != nil {
		return nil, err
	}
	out := make([]binding.BotInfo, 0, len(settings))
	for _, s := range settings {
		out = append(out, binding.BotInfo{
			UUID:      s.UUID,
			Platform:  s.Platform,
			Name:      s.Name,
			Scenarios: s.Scenarios,
		})
	}
	return out, nil
}

// Compile-time assertion that BindingStore satisfies binding.Store.
var _ binding.Store = (*BindingStore)(nil)
