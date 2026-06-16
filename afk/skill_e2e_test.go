package afk_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/afk"
	"github.com/tingly-dev/tingly-box/afk/skill"
)

// fakeTool is a simple test tool that returns a fixed result
type fakeTool struct {
	name   string
	result string
}

// afkToolAdapter wraps fakeTool to implement afk.Tool interface
type afkToolAdapter struct {
	fakeTool *fakeTool
}

func (a afkToolAdapter) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        a.fakeTool.name,
		Description: anthropic.String("A test tool for E2E testing"),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "Input for the test tool",
				},
			},
		},
	}
}

func (a afkToolAdapter) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	return a.fakeTool.result, nil
}

// TestAgentWithSkill demonstrates an end-to-end workflow where:
// 1. An agent is created with skill loading enabled
// 2. Skills are discovered and loaded from .agent/skills/ directory
// 3. The agent can use the activate_skill tool to load skill instructions
// 4. The agent processes a user request that benefits from skill activation
//
// Prerequisites:
// - Set AFK_BASE_URL, AFK_API_KEY, AFK_MODEL environment variables
// - Or skip test with -short flag
//
// Usage:
//
//	go test -v -run TestAgentWithSkill
func TestAgentWithSkill(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Get configuration from environment
	baseURL := os.Getenv("AFK_BASE_URL")
	apiKey := os.Getenv("AFK_API_KEY")
	model := os.Getenv("AFK_MODEL")

	if baseURL == "" || apiKey == "" || model == "" {
		t.Skip("skipping E2E test: missing AFK_BASE_URL, AFK_API_KEY, or AFK_MODEL")
	}

	t.Logf("E2E Test Configuration:")
	t.Logf("  BaseURL: %s", baseURL)
	t.Logf("  Model: %s", model)

	// Step 1: Create skill loader
	cfg := &skill.Config{
		EnableStandardPaths: true,
		EnableDefaultSkills: true,
	}

	loader, err := skill.NewLoader(cfg)
	require.NoError(t, err, "should create skill loader")
	require.NotNil(t, loader)

	skills := loader.Skills()
	t.Logf("Loaded %d skills", len(skills))

	// Log available skills
	for _, s := range skills {
		t.Logf("  - %s: %s (tags: %v)", s.Name, s.Description, s.Tags)
	}

	// Step 2: Create activate_skill tool
	activateTool, err := skill.NewActivateSkillTool(skills)
	require.NoError(t, err, "should create activate skill tool")
	require.NotNil(t, activateTool)

	// Step 3: Create AFK engine with skill support
	// Create tool adapters
	wrappedEchoTool := afkToolAdapter{fakeTool: &fakeTool{name: "echo", result: "Tool executed successfully"}}

	engine, err := afk.NewEngine(afk.Config{
		BaseURL:       baseURL,
		APIKey:        apiKey,
		Model:         model,
		System:        "You are a helpful assistant with access to skills. Use the activate_skill tool when a user's request matches a skill's description.",
		MaxTokens:     1000,
		MaxIterations: 5,
		Tools:         []afk.Tool{activateTool, wrappedEchoTool},
		StreamText:    false,
	})
	require.NoError(t, err, "should create AFK engine")
	require.NotNil(t, engine)

	// Step 4: Test skill discovery and activation
	t.Run("SkillDiscovery", func(t *testing.T) {
		// Verify skills were loaded
		assert.Greater(t, len(skills), 0, "should have loaded at least one skill")

		// Check activate_skill tool parameters
		param := activateTool.Param()
		assert.Equal(t, "activate_skill", param.Name)

		// Verify tool description
		assert.NotNil(t, param.Description)
		t.Logf("activate_skill description: %s", param.Description)

		// Check that skill names are in the enum
		props, ok := param.InputSchema.Properties.(map[string]any)
		require.True(t, ok, "properties should be a map")
		nameProp, ok := props["name"].(map[string]any)
		require.True(t, ok, "name property should exist")
		enum, ok := nameProp["enum"].([]any)
		require.True(t, ok, "name property should have enum")
		assert.Greater(t, len(enum), 0, "enum should not be empty")
		t.Logf("Available skills in enum: %v", enum)
	})

	// Step 5: Test basic conversation without skill activation
	t.Run("BasicConversation", func(t *testing.T) {
		ctx := context.Background()
		history := []anthropic.BetaMessageParam{}

		messages, finalText, err := engine.Run(ctx, history, "Hello! Can you help me?", nil)
		require.NoError(t, err, "should complete conversation without error")
		assert.NotEmpty(t, finalText, "should have response text")

		t.Logf("Conversation completed successfully")
		t.Logf("Final response: %s", finalText)
		t.Logf("Total messages: %d", len(messages))
	})

	// Step 6: Test skill activation with skill-triggering prompt
	t.Run("SkillActivation", func(t *testing.T) {
		if len(skills) == 0 {
			t.Skip("no skills available for activation test")
		}

		// Use the first available skill for testing
		testSkill := skills[0]
		t.Logf("Testing with skill: %s", testSkill.Name)

		ctx := context.Background()
		history := []anthropic.BetaMessageParam{}

		// Create a prompt that should trigger skill activation based on skill description
		prompt := constructSkillPrompt(testSkill)
		t.Logf("Test prompt: %s", prompt)

		messages, finalText, err := engine.Run(ctx, history, prompt, nil)
		require.NoError(t, err, "should complete skill activation without error")

		t.Logf("Skill activation completed")
		t.Logf("Final response: %s", finalText)
		t.Logf("Total messages: %d", len(messages))

		// Verify conversation flow
		assert.Greater(t, len(messages), 2, "should have at least user + assistant messages")

		// Log the conversation structure for debugging
		for i, msg := range messages {
			t.Logf("Message %d: role=%s, content_blocks=%d", i, msg.Role, len(msg.Content))
		}
	})
}

// constructSkillPrompt creates a test prompt based on skill description
func constructSkillPrompt(s *skill.Skill) string {
	// Try to create a prompt that matches the skill's description
	// This is heuristic - real usage would depend on actual skill content
	return "I need help with: " + s.Description + ". Can you assist me?"
}

// TestEngineWithStandardPaths tests the standard skill discovery paths
func TestEngineWithStandardPaths(t *testing.T) {
	// Create a temporary skill directory for testing
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, ".agent", "skills")
	err := os.MkdirAll(skillDir, 0755)
	require.NoError(t, err)

	// Create a test skill
	testSkillPath := filepath.Join(skillDir, "e2e-test-skill")
	err = os.MkdirAll(testSkillPath, 0755)
	require.NoError(t, err)

	skillContent := `---
name: e2e-test-skill
description: A skill for E2E testing purposes
tags: test, e2e
---

# E2E Test Skill

This skill is used for end-to-end testing.

## Instructions

When this skill is activated:
1. Acknowledge the e2e-test-skill is loaded
2. Explain the testing capabilities
`
	err = os.WriteFile(filepath.Join(testSkillPath, "SKILL.md"), []byte(skillContent), 0644)
	require.NoError(t, err)

	// Change working directory to temp dir for this test
	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Create loader with standard paths enabled
	cfg := &skill.Config{
		EnableStandardPaths: true,
		EnableDefaultSkills: true,
	}

	loader, err := skill.NewLoader(cfg)
	require.NoError(t, err)

	skills := loader.Skills()
	assert.Greater(t, len(skills), 0, "should discover skills from standard paths")

	// Find our test skill
	var found bool
	for _, s := range skills {
		if s.Name == "e2e-test-skill" {
			found = true
			assert.Contains(t, s.Description, "E2E testing")
			assert.Contains(t, s.Tags, "test")
			break
		}
	}
	assert.True(t, found, "should find e2e-test-skill")

	t.Logf("Successfully discovered %d skills from standard paths", len(skills))
}

// TestEngineWithCustomPaths tests custom skill directory configuration
func TestEngineWithCustomPaths(t *testing.T) {
	// Create custom skill directories
	customDir1 := t.TempDir()
	customDir2 := t.TempDir()

	// Create skill in first directory
	skill1Path := filepath.Join(customDir1, "custom-skill-1")
	err := os.MkdirAll(skill1Path, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(skill1Path, "SKILL.md"),
		[]byte("---\nname: custom-skill-1\ndescription: First custom skill\n---\n# Custom Skill 1"), 0644)
	require.NoError(t, err)

	// Create skill in second directory
	skill2Path := filepath.Join(customDir2, "custom-skill-2")
	err = os.MkdirAll(skill2Path, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(skill2Path, "SKILL.md"),
		[]byte("---\nname: custom-skill-2\ndescription: Second custom skill\n---\n# Custom Skill 2"), 0644)
	require.NoError(t, err)

	// Create loader with custom paths
	cfg := &skill.Config{
		EnableStandardPaths: false, // Disable standard paths
		EnableDefaultSkills: true,
		Paths:               []string{customDir1, customDir2},
	}

	loader, err := skill.NewLoader(cfg)
	require.NoError(t, err)

	skills := loader.Skills()
	assert.GreaterOrEqual(t, len(skills), 2, "should find at least 2 custom skills")

	// Verify both skills are loaded
	skillNames := make(map[string]bool)
	for _, s := range skills {
		skillNames[s.Name] = true
	}

	assert.True(t, skillNames["custom-skill-1"], "should find custom-skill-1")
	assert.True(t, skillNames["custom-skill-2"], "should find custom-skill-2")

	t.Logf("Successfully loaded %d skills from custom paths", len(skills))
}
