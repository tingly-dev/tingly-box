package main

import "testing"

// `--providers` names the credentials file for agent and replay; the old
// `--config` spelling still parses there (deprecated), while matrix keeps
// `--config` for model definitions.
func TestProvidersFlagAndDeprecatedConfigAlias(t *testing.T) {
	for _, flag := range []string{"--providers", "--config"} {
		t.Run("agent "+flag, func(t *testing.T) {
			cli, parser := newTestParser(t)
			if _, err := parser.Parse([]string{"agent", "claude", flag, "p.yaml"}); err != nil {
				t.Fatal(err)
			}
			if cli.Agent.Providers != "p.yaml" {
				t.Fatalf("Providers = %q", cli.Agent.Providers)
			}
		})
		t.Run("replay "+flag, func(t *testing.T) {
			cli, parser := newTestParser(t)
			if _, err := parser.Parse([]string{"replay", "claude", "--upstream", "real", flag, "p.yaml"}); err != nil {
				t.Fatal(err)
			}
			if cli.Replay.Providers != "p.yaml" {
				t.Fatalf("Providers = %q", cli.Replay.Providers)
			}
		})
	}
}
