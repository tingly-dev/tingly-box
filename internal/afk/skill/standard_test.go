package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoader_ClaudeStandard(t *testing.T) {
	cfg := &Config{
		Standard:            ClaudeStandard,
		EnableStandardPaths: false,
		EnableDefaultSkills: true,
		Paths:               []string{"../../../.agent/skills"},
	}

	loader, err := NewLoader(cfg)
	require.NoError(t, err)
	require.NotNil(t, loader)

	assert.Equal(t, ClaudeStandard, loader.standard)

	skills := loader.Skills()
	assert.NotEmpty(t, skills, "should load skills with Claude standard")

	// Find the test skill
	var testSkill *Skill
	for _, skill := range skills {
		if skill.Name == "test-skill" {
			testSkill = skill
			break
		}
	}

	require.NotNil(t, testSkill, "should find test-skill")
	assert.Equal(t, "test-skill", testSkill.Name)
	assert.Contains(t, testSkill.Description, "test skill")

	t.Logf("Claude standard loaded skill: %s", testSkill.Name)
}

func TestLoader_AgentStandard(t *testing.T) {
	// Create a test environment for agent standard
	tempDir := t.TempDir()
	agentSkillDir := filepath.Join(tempDir, ".agents", "skills")
	err := os.MkdirAll(agentSkillDir, 0755)
	require.NoError(t, err)

	// Create a test skill following agent standard
	testSkillPath := filepath.Join(agentSkillDir, "agent-test-skill")
	err = os.MkdirAll(testSkillPath, 0755)
	require.NoError(t, err)

	skillContent := `---
name: agent-test-skill
description: A test skill for agent standard
tags: agent, test
---

# Agent Test Skill

This skill follows the agent industry standard.
`
	err = os.WriteFile(filepath.Join(testSkillPath, "SKILL.md"), []byte(skillContent), 0644)
	require.NoError(t, err)

	// Change to temp dir for testing
	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()
	_ = os.Chdir(tempDir)

	cfg := &Config{
		Standard:            AgentStandard,
		EnableStandardPaths: false,
		EnableDefaultSkills: true,
		Paths:               []string{".agents/skills"},
	}

	loader, err := NewLoader(cfg)
	require.NoError(t, err)
	require.NotNil(t, loader)

	assert.Equal(t, AgentStandard, loader.standard)

	skills := loader.Skills()
	assert.NotEmpty(t, skills, "should load skills with Agent standard")

	// Find the test skill
	var found bool
	for _, skill := range skills {
		if skill.Name == "agent-test-skill" {
			found = true
			assert.Contains(t, skill.Description, "agent standard")
			assert.Contains(t, skill.Tags, "agent")
			break
		}
	}

	assert.True(t, found, "should find agent-test-skill")
	t.Logf("Agent standard loaded %d skills", len(skills))
}

func TestLoader_StandardDefaults(t *testing.T) {
	cfg := &Config{
		EnableStandardPaths: false,
		EnableDefaultSkills: true,
		Paths:               []string{"../../../.agent/skills"},
	}

	loader, err := NewLoader(cfg)
	require.NoError(t, err)
	require.NotNil(t, loader)

	// Should default to Claude standard
	assert.Equal(t, ClaudeStandard, loader.standard)

	skills := loader.Skills()
	assert.NotEmpty(t, skills, "should load skills with default Claude standard")
}

func TestLoader_ClaudeStandardPaths(t *testing.T) {
	tempDir := t.TempDir()

	// Create Claude-style skills
	claudeSkillDir := filepath.Join(tempDir, ".claude", "skills")
	err := os.MkdirAll(claudeSkillDir, 0755)
	require.NoError(t, err)

	claudeTestSkill := filepath.Join(claudeSkillDir, "claude-skill")
	err = os.MkdirAll(claudeTestSkill, 0755)
	require.NoError(t, err)

	skillContent := `---
name: claude-skill
description: Claude style skill
tags: claude
---
# Claude Skill`
	err = os.WriteFile(filepath.Join(claudeTestSkill, "SKILL.md"), []byte(skillContent), 0644)
	require.NoError(t, err)

	// Change to temp dir for testing
	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()
	_ = os.Chdir(tempDir)

	cfg := &Config{
		Standard:            ClaudeStandard,
		EnableStandardPaths: true,
		EnableDefaultSkills: true,
	}

	loader, err := NewLoader(cfg)
	require.NoError(t, err)

	skills := loader.Skills()

	// Should find skill in .claude/skills/
	var found bool
	for _, skill := range skills {
		if skill.Name == "claude-skill" {
			found = true
			break
		}
	}

	assert.True(t, found, "should find skill in .claude/skills/")
	t.Logf("Claude standard found %d skills including claude-skill", len(skills))
}

func TestLoader_AgentStandardPaths(t *testing.T) {
	tempDir := t.TempDir()

	// Create Agent-style skills
	agentSkillDir := filepath.Join(tempDir, ".agents", "skills")
	err := os.MkdirAll(agentSkillDir, 0755)
	require.NoError(t, err)

	agentTestSkill := filepath.Join(agentSkillDir, "agent-skill")
	err = os.MkdirAll(agentTestSkill, 0755)
	require.NoError(t, err)

	skillContent := `---
name: agent-skill
description: Agent style skill
tags: agent
---
# Agent Skill`
	err = os.WriteFile(filepath.Join(agentTestSkill, "SKILL.md"), []byte(skillContent), 0644)
	require.NoError(t, err)

	// Change to temp dir for testing
	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()
	_ = os.Chdir(tempDir)

	cfg := &Config{
		Standard:            AgentStandard,
		EnableStandardPaths: true,
		EnableDefaultSkills: true,
	}

	loader, err := NewLoader(cfg)
	require.NoError(t, err)

	skills := loader.Skills()

	// Should find skill in .agents/skills/
	var found bool
	for _, skill := range skills {
		if skill.Name == "agent-skill" {
			found = true
			break
		}
	}

	assert.True(t, found, "should find skill in .agents/skills/")
	t.Logf("Agent standard found %d skills including agent-skill", len(skills))
}
