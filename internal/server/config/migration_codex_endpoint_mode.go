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
	// Providers live in SQLite (the JSON c.Providers slice is legacy backup).
	// Backfill the DB-stored ones directly so the resolver sees the right mode.
	if c.providerStore != nil {
		if oauthProviders, err := c.providerStore.ListOAuth(); err == nil {
			for _, p := range oauthProviders {
				if p.OAuthDetail == nil || p.OAuthDetail.GetIssuer() != ai.IssuerCodex {
					continue
				}
				if p.OpenAIEndpointMode == ai.EndpointModeResponses {
					continue
				}
				p.OpenAIEndpointMode = ai.EndpointModeResponses
				if err := c.providerStore.Save(p); err != nil {
					logrus.WithError(err).WithField("provider_uuid", p.UUID).Warn("Failed to backfill openai_endpoint_mode on Codex provider")
					continue
				}
				logrus.WithField("provider_uuid", p.UUID).Info("Backfilled openai_endpoint_mode=responses on Codex provider (db)")
			}
		} else {
			logrus.WithError(err).Warn("Failed to list OAuth providers for openai_endpoint_mode backfill")
		}
	}
}
