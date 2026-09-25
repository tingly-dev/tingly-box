package agent

import (
	"slices"
	"testing"
)

func TestClaudeCodeTiersFromEnv(t *testing.T) {
	unified := ClaudeCodeTiersFromEnv(map[string]string{
		"ANTHROPIC_MODEL": "tingly/cc[1m]", "ANTHROPIC_DEFAULT_OPUS_MODEL": "tingly/cc[1m]",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "tingly/cc[1m]", "ANTHROPIC_DEFAULT_HAIKU_MODEL": "tingly/cc[1m]",
	})
	if !unified.Unified || !slices.Equal(unified.Tiers, []ClaudeCodeTier{{Alias: "", Model: "tingly/cc"}}) {
		t.Fatalf("unified env = %+v, want one tier without the [1m] marker", unified)
	}

	separate := ClaudeCodeTiersFromEnv(map[string]string{
		"ANTHROPIC_MODEL": "tingly/cc-default", "ANTHROPIC_DEFAULT_OPUS_MODEL": "tingly/cc-opus",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL": "tingly/cc-haiku",
	})
	want := []ClaudeCodeTier{{Alias: "", Model: "tingly/cc-default"}, {Alias: "opus", Model: "tingly/cc-opus"}, {Alias: "haiku", Model: "tingly/cc-haiku"}}
	if separate.Unified || !slices.Equal(separate.Tiers, want) {
		t.Fatalf("separate env = %+v, want %v (a tier missing from the env is left out)", separate, want)
	}
}
