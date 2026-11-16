package config

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MaxUIPrefValueBytes bounds a single UI preference value. Prefs are small
// flags and lists; anything bigger belongs in a real feature store.
const MaxUIPrefValueBytes = 16 * 1024

// GetUIPrefs returns a copy of all UI preferences.
func (c *Config) GetUIPrefs() map[string]json.RawMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]json.RawMessage, len(c.UIPrefs))
	for k, v := range c.UIPrefs {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

// PatchUIPrefs merges patch into the UI preferences and saves: a key with a
// JSON null value is deleted, any other value replaces the stored one. It
// returns the resulting preferences.
func (c *Config) PatchUIPrefs(patch map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	for k, v := range patch {
		if k == "" {
			return nil, fmt.Errorf("ui pref key must not be empty")
		}
		if len(v) > MaxUIPrefValueBytes {
			return nil, fmt.Errorf("ui pref %q is %d bytes, over the %d byte limit", k, len(v), MaxUIPrefValueBytes)
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("ui pref %q is not valid JSON", k)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.UIPrefs == nil {
		c.UIPrefs = make(map[string]json.RawMessage)
	}
	for k, v := range patch {
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			delete(c.UIPrefs, k)
			continue
		}
		c.UIPrefs[k] = append(json.RawMessage(nil), v...)
	}

	if err := c.saveLocked(); err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(c.UIPrefs))
	for k, v := range c.UIPrefs {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out, nil
}
