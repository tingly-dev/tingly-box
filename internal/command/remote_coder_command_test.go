package command

import (
	"testing"
)

func TestBuildClaudeEnvUnified(t *testing.T) {
	env := buildClaudeEnv("unified", "http://base", "key")

	if env["ANTHROPIC_MODEL"] != claudeCodeUnifiedModel {
		t.Fatalf("expected unified model to be %q, got %q", claudeCodeUnifiedModel, env["ANTHROPIC_MODEL"])
	}
	if env["CLAUDE_CODE_SUBAGENT_MODEL"] != claudeCodeUnifiedModel {
		t.Fatalf("expected subagent model to match unified model")
	}
	if env["ANTHROPIC_BASE_URL"] != "http://base" {
		t.Fatalf("expected base url to match input")
	}
}

func TestBuildClaudeEnvSeparate(t *testing.T) {
	env := buildClaudeEnv("separate", "http://base", "key")

	if env["ANTHROPIC_MODEL"] != claudeCodeDefaultModel {
		t.Fatalf("expected default model, got %q", env["ANTHROPIC_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != claudeCodeHaikuModel {
		t.Fatalf("expected haiku model, got %q", env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != claudeCodeOpusModel {
		t.Fatalf("expected opus model, got %q", env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != claudeCodeSonnetModel {
		t.Fatalf("expected sonnet model, got %q", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if env["CLAUDE_CODE_SUBAGENT_MODEL"] != claudeCodeSubagentModel {
		t.Fatalf("expected subagent model, got %q", env["CLAUDE_CODE_SUBAGENT_MODEL"])
	}
}
