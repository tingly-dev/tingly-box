package config

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	tomlpkg "github.com/pelletier/go-toml/v2"
	"github.com/sirupsen/logrus"
)

// migrate20260517 rewrites `127.0.0.1` host references to `localhost` in agent
// config files that tingly-box previously wrote into the user's home directory.
//
// Background: commit 4569e8f flipped every tingly-emitted base URL from
// `127.0.0.1:<port>` to `localhost:<port>`. New `agent apply` runs already write
// the new value, but configs already on disk (settings.json / config.toml /
// opencode.json) keep pointing at the old host, and on dual-stack hosts where
// the server now binds only to `::1` the agents would silently fail.
//
// Scope: we only touch entries that we can confidently identify as tingly-owned
// (path under `/tingly/...` for Claude Code, well-known provider key
// `tingly-box` for Codex/OpenCode). User-authored proxy URLs, upstream API
// bases, etc. are left as-is.
//
// Files touched (best-effort, missing files are skipped):
//   - ~/.claude/settings.json     → env.ANTHROPIC_BASE_URL
//   - ~/.codex/config.toml        → [model_providers.tingly-box].base_url
//   - ~/.config/opencode/opencode.json → provider.tingly-box.options.baseURL
//
// One-shot via MigrationsCompleted, so subsequent restarts skip it even after
// the user re-introduces `127.0.0.1` somewhere.
func migrate20260517(c *Config) {
	if c.hasMigrationCompleted("20260517") {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		logrus.WithError(err).Warn("Migration 2026-05-17: skipped, cannot resolve home directory")
		c.markMigrationCompleted("20260517")
		_ = c.Save()
		return
	}

	var rewritten []string

	if path, ok := rewriteClaudeSettingsHost(filepath.Join(homeDir, ".claude", "settings.json")); ok {
		rewritten = append(rewritten, path)
	}
	if path, ok := rewriteCodexConfigHost(filepath.Join(homeDir, ".codex", "config.toml")); ok {
		rewritten = append(rewritten, path)
	}
	if path, ok := rewriteOpenCodeConfigHost(filepath.Join(homeDir, ".config", "opencode", "opencode.json")); ok {
		rewritten = append(rewritten, path)
	}

	if len(rewritten) > 0 {
		logrus.WithField("files", rewritten).
			Info("Migration 2026-05-17: rewrote 127.0.0.1 to localhost in tingly-owned agent configs")
	}

	c.markMigrationCompleted("20260517")
	_ = c.Save()
}

// rewriteTinglyLoopback returns the rewritten URL and true if `rawURL` is an
// `http(s)://127.0.0.1[:port]/tingly/...` URL emitted by tingly-box.
// For anything else (different host, different path, parse failure) it returns
// the original string and false so the caller leaves the value alone.
func rewriteTinglyLoopback(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, false
	}
	if u.Hostname() != "127.0.0.1" {
		return rawURL, false
	}
	if !strings.HasPrefix(u.Path, "/tingly/") {
		return rawURL, false
	}
	if port := u.Port(); port != "" {
		u.Host = "localhost:" + port
	} else {
		u.Host = "localhost"
	}
	return u.String(), true
}

// rewriteLoopbackHost rewrites the host portion of `rawURL` from 127.0.0.1 to
// localhost without checking the URL path. Used for callers where the
// surrounding key already proves the entry is tingly-owned (e.g. the
// `tingly-box` provider in Codex / OpenCode configs).
func rewriteLoopbackHost(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, false
	}
	if u.Hostname() != "127.0.0.1" {
		return rawURL, false
	}
	if port := u.Port(); port != "" {
		u.Host = "localhost:" + port
	} else {
		u.Host = "localhost"
	}
	return u.String(), true
}

func rewriteClaudeSettingsHost(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		logrus.WithError(err).WithField("path", path).
			Warn("Migration 2026-05-17: cannot parse Claude settings.json, skipping")
		return "", false
	}

	envSection, ok := settings["env"].(map[string]interface{})
	if !ok {
		return "", false
	}
	baseURL, ok := envSection["ANTHROPIC_BASE_URL"].(string)
	if !ok || baseURL == "" {
		return "", false
	}
	newURL, changed := rewriteTinglyLoopback(baseURL)
	if !changed {
		return "", false
	}
	envSection["ANTHROPIC_BASE_URL"] = newURL

	if _, err := backupFile(path); err != nil {
		logrus.WithError(err).WithField("path", path).
			Warn("Migration 2026-05-17: backup failed, skipping rewrite")
		return "", false
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", false
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		logrus.WithError(err).WithField("path", path).
			Warn("Migration 2026-05-17: failed to write Claude settings.json")
		return "", false
	}
	return path, true
}

func rewriteCodexConfigHost(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	var cfg map[string]interface{}
	if err := tomlpkg.Unmarshal(data, &cfg); err != nil {
		logrus.WithError(err).WithField("path", path).
			Warn("Migration 2026-05-17: cannot parse Codex config.toml, skipping")
		return "", false
	}

	providers, ok := cfg["model_providers"].(map[string]interface{})
	if !ok {
		return "", false
	}
	tb, ok := providers["tingly-box"].(map[string]interface{})
	if !ok {
		return "", false
	}
	baseURL, ok := tb["base_url"].(string)
	if !ok || baseURL == "" {
		return "", false
	}
	newURL, changed := rewriteLoopbackHost(baseURL)
	if !changed {
		return "", false
	}
	tb["base_url"] = newURL

	if _, err := backupFile(path); err != nil {
		logrus.WithError(err).WithField("path", path).
			Warn("Migration 2026-05-17: backup failed, skipping rewrite")
		return "", false
	}

	out, err := tomlpkg.Marshal(cfg)
	if err != nil {
		return "", false
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		logrus.WithError(err).WithField("path", path).
			Warn("Migration 2026-05-17: failed to write Codex config.toml")
		return "", false
	}
	return path, true
}

func rewriteOpenCodeConfigHost(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		logrus.WithError(err).WithField("path", path).
			Warn("Migration 2026-05-17: cannot parse opencode.json, skipping")
		return "", false
	}

	providers, ok := cfg["provider"].(map[string]interface{})
	if !ok {
		return "", false
	}
	tb, ok := providers["tingly-box"].(map[string]interface{})
	if !ok {
		return "", false
	}
	options, ok := tb["options"].(map[string]interface{})
	if !ok {
		return "", false
	}
	baseURL, ok := options["baseURL"].(string)
	if !ok || baseURL == "" {
		return "", false
	}
	newURL, changed := rewriteLoopbackHost(baseURL)
	if !changed {
		return "", false
	}
	options["baseURL"] = newURL

	if _, err := backupFile(path); err != nil {
		logrus.WithError(err).WithField("path", path).
			Warn("Migration 2026-05-17: backup failed, skipping rewrite")
		return "", false
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", false
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		logrus.WithError(err).WithField("path", path).
			Warn("Migration 2026-05-17: failed to write opencode.json")
		return "", false
	}
	return path, true
}
