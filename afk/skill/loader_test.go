package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoader_LoadSkills(t *testing.T) {
	cfg := &Config{
		EnableStandardPaths: false, // Disable standard paths for testing
		EnableDefaultSkills: true,
		Paths:               []string{"../../../.agent/skills"},
	}

	loader, err := NewLoader(cfg)
	require.NoError(t, err)
	require.NotNil(t, loader)

	skills := loader.Skills()
	assert.NotEmpty(t, skills, "should load at least one skill")

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
	assert.NotEmpty(t, testSkill.Body)
	assert.Contains(t, testSkill.Body, "Test Skill")

	// Check tags
	assert.Contains(t, testSkill.Tags, "test")
	assert.Contains(t, testSkill.Tags, "demo")

	t.Logf("Loaded skill: %s", testSkill.Name)
	t.Logf("Description: %s", testSkill.Description)
	t.Logf("Location: %s", testSkill.Location)
	t.Logf("Tags: %v", testSkill.Tags)
}

func TestActivateSkillTool(t *testing.T) {
	cfg := &Config{
		EnableStandardPaths: false, // Disable standard paths for testing
		EnableDefaultSkills: true,
		Paths:               []string{"../../../.agent/skills"},
	}

	loader, err := NewLoader(cfg)
	require.NoError(t, err)

	skills := loader.Skills()
	tool, err := NewActivateSkillTool(skills)
	require.NoError(t, err)
	require.NotNil(t, tool)

	// Test tool parameter
	param := tool.Param()
	assert.Equal(t, "activate_skill", param.Name)
	assert.NotNil(t, param.Description)

	// Test that input schema has enum with skill names
	props, ok := param.InputSchema.Properties.(map[string]any)
	require.True(t, ok, "properties should be a map")

	nameProp, ok := props["name"].(map[string]any)
	require.True(t, ok, "name property should exist")

	enum, ok := nameProp["enum"].([]any)
	require.True(t, ok, "name property should have enum")
	assert.NotEmpty(t, enum, "enum should not be empty")

	t.Logf("Tool description: %s", param.Description)
	t.Logf("Available skills in enum: %v", enum)
}

func TestSkillToTool(t *testing.T) {
	cfg := &Config{
		EnableStandardPaths: false, // Disable standard paths for testing
		EnableDefaultSkills: true,
		Paths:               []string{"../../../.agent/skills"},
	}

	loader, err := NewLoader(cfg)
	require.NoError(t, err)

	skill, ok := loader.GetSkill("test-skill")
	require.True(t, ok, "should find test-skill")

	tool, err := SkillToTool(skill)
	require.NoError(t, err)
	require.NotNil(t, tool)

	// Test tool parameter
	param := tool.Param()
	assert.Equal(t, "test-skill", param.Name)
	assert.NotNil(t, param.Description)

	t.Logf("Individual skill tool name: %s", param.Name)
	t.Logf("Individual skill tool description: %s", param.Description)
}

func TestLoader_FilterByTag(t *testing.T) {
	cfg := &Config{
		EnableStandardPaths: false, // Disable standard paths for testing
		EnableDefaultSkills: true,
		Paths:               []string{"../../../.agent/skills"},
	}

	loader, err := NewLoader(cfg)
	require.NoError(t, err)

	// Filter by test tag
	testSkills := loader.FilterByTag("test")
	assert.NotEmpty(t, testSkills, "should find skills with 'test' tag")

	for _, skill := range testSkills {
		assert.Contains(t, skill.Tags, "test")
		t.Logf("Found skill with test tag: %s", skill.Name)
	}

	// Filter by demo tag
	demoSkills := loader.FilterByTag("demo")
	assert.NotEmpty(t, demoSkills, "should find skills with 'demo' tag")

	// Filter by both tags
	bothSkills := loader.FilterByTag("test", "demo")
	assert.NotEmpty(t, bothSkills, "should find skills with both tags")

	for _, skill := range bothSkills {
		assert.Contains(t, skill.Tags, "test")
		assert.Contains(t, skill.Tags, "demo")
	}
}
