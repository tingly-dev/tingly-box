package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// UserTokenFromFile returns the user token recorded in <configDir>/config.json
// without building a Config — and so without opening the database.
//
// Commands that talk to an already-running server (`log`, `open`, the
// running half of `status`) need exactly two things: the live port, which
// comes from the runtime port file, and this token for the server's API.
// Going through Config for the token would open tingly.db alongside the
// server, which is the second-writer hazard described on app.AppManager.
//
// A missing or unreadable file yields "" — the same as a config that has no
// token — so callers fall back to the unauthenticated URL as they always did.
func UserTokenFromFile(configDir string) string {
	data, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		UserToken string `json:"user_token"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return cfg.UserToken
}
