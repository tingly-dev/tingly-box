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

func TestClaudeCodeTiersFromEnv_Fable(t *testing.T) {
	got := ClaudeCodeTiersFromEnv(map[string]string{
		"ANTHROPIC_MODEL": "tingly/cc-default", "ANTHROPIC_DEFAULT_FABLE_MODEL": "tingly/cc-fable",
	})
	want := []ClaudeCodeTier{{Alias: "", Model: "tingly/cc-default"}, {Alias: "fable", Model: "tingly/cc-fable"}}
	if got.Unified || !slices.Equal(got.Tiers, want) {
		t.Fatalf("env = %+v, want %v", got, want)
	}
}

func TestClaudeCodeTiersFromEnv_NoDefaultModelFallsBackToSonnet(t *testing.T) {
	tiers := ClaudeCodeTiersFromEnv(map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "cc", "ANTHROPIC_DEFAULT_OPUS_MODEL": "cc",
	})
	if !tiers.Unified || len(tiers.Tiers) != 1 || tiers.Tiers[0].Model != "cc" {
		t.Fatalf("tiers = %+v, want one unified tier on cc", tiers)
	}
}

func TestOmitFollowedDefaultModel(t *testing.T) {
	followed := map[string]string{"ANTHROPIC_MODEL": "cc", "ANTHROPIC_DEFAULT_SONNET_MODEL": "cc"}
	OmitFollowedDefaultModel(followed)
	if _, ok := followed["ANTHROPIC_MODEL"]; ok {
		t.Error("ANTHROPIC_MODEL kept although the sonnet slot follows it")
	}
	split := map[string]string{"ANTHROPIC_MODEL": "cc", "ANTHROPIC_DEFAULT_SONNET_MODEL": "sonnet"}
	OmitFollowedDefaultModel(split)
	if split["ANTHROPIC_MODEL"] != "cc" {
		t.Error("ANTHROPIC_MODEL dropped although the sonnet slot has its own model")
	}
}
