package smart_guide

import (
	"embed"
)

//go:embed prompts/*.txt
var PromptFS embed.FS

// LoadPrompt reads a prompt file from the embedded filesystem
func LoadPrompt(name string) (string, error) {
	content, err := PromptFS.ReadFile("prompts/" + name + ".txt")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// MustLoadPrompt reads a prompt file or panics
func MustLoadPrompt(name string) string {
	content, err := LoadPrompt(name)
	if err != nil {
		panic("smart_guide: failed to load prompt " + name + ": " + err.Error())
	}
	return content
}

// DefaultSystemPrompt returns the default system prompt for @tb
func DefaultSystemPrompt() string {
	return MustLoadPrompt("default_system_prompt.v4")
}

// HandoffToCCPrompt returns the handoff prompt when switching to Claude Code
func HandoffToCCPrompt() string {
	return MustLoadPrompt("handoff_to_cc_prompt")
}

// HandoffToTBPrompt returns the handoff prompt when returning to Smart Guide
func HandoffToTBPrompt() string {
	return MustLoadPrompt("handoff_to_tb_prompt")
}

// DefaultGreeting returns the default greeting for new users
func DefaultGreeting() string {
	return MustLoadPrompt("default_greeting")
}
