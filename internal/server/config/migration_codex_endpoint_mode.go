package config

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
)

// migrate20260518 sets OpenAIEndpointMode=responses on existing Codex OAuth
// providers. Codex's API only exposes /responses (no /chat/completions); the
// mode is now declared on Codex providers at OAuth instantiation, but
// existing user configs from before this change don't carry it. Without the
// backfill, the new resolver's default-Chat semantics would silently send
// /chat/completions requests to Codex and fail.
//
// Idempotent: only flips the mode when issuer is Codex and the mode is unset.
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
		if p.OpenAIEndpointMode == ai.EndpointModeResponses {
			continue
		}
		p.OpenAIEndpointMode = ai.EndpointModeResponses
		needsSave = true
		logrus.WithField("provider_uuid", p.UUID).Info("Backfilled openai_endpoint_mode=responses on Codex provider")
	}

	c.markMigrationCompleted("20260518")
	if needsSave {
		_ = c.Save()
	}
}
