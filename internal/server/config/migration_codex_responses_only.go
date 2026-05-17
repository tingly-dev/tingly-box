package config

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
)

// migrate20260518 backfills ResponsesOnly=true on existing Codex OAuth
// providers. Codex's API only exposes /responses (no /chat/completions); the
// flag is now declared on Codex providers at OAuth instantiation, but
// existing user configs from before this change don't carry it. Without the
// backfill, the new endpoint resolver would mirror the incoming API and send
// Chat requests to Codex, which would fail.
//
// Idempotent: only flips the flag when issuer is Codex and the flag is false.
func migrate20260518(c *Config) {
	if c.hasMigrationCompleted("20260518") {
		return
	}

	needsSave := false
	for _, p := range c.Providers {
		if p.OAuthDetail == nil {
			continue
		}
		if p.OAuthDetail.GetIssuer() != ai.IssuerCodex {
			continue
		}
		if p.ResponsesOnly {
			continue
		}
		p.ResponsesOnly = true
		needsSave = true
		logrus.WithField("provider_uuid", p.UUID).Info("Backfilled responses_only=true on Codex provider")
	}

	c.markMigrationCompleted("20260518")
	if needsSave {
		_ = c.Save()
	}
}
