package skill

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var activateSkillLogger = logrus.WithField("component", "activate_skill_tool")

// ActivateSkillTool is an AFK Tool that activates skills by name.
// It implements the AFK Tool interface to integrate with the ReAct loop.
type ActivateSkillTool struct {
	skills map[string]*Skill
}

// NewActivateSkillTool creates a new skill activation tool.
func NewActivateSkillTool(skills Skills) (*ActivateSkillTool, error) {
	if len(skills) == 0 {
		return nil, errors.New("no skills provided")
	}

	skillsMap := make(map[string]*Skill)
	for _, skill := range skills {
		skillsMap[skill.Name] = skill
	}

	return &ActivateSkillTool{
		skills: skillsMap,
	}, nil
}

// Param returns the Anthropic tool parameter for this skill activator.
func (t *ActivateSkillTool) Param() anthropic.BetaToolParam {
	// Build enum of available skill names
	skillNames := make([]any, 0, len(t.skills))
	for name := range t.skills {
		skillNames = append(skillNames, name)
	}

	return anthropic.BetaToolParam{
		Name:        "activate_skill",
		Description: anthropic.String("Load the instructions for a skill by name. When the user's request matches a skill's description, activate it before executing task-specific instructions."),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The name of the skill to activate.",
					"enum":        skillNames,
				},
			},
			Required: []string{"name"},
		},
	}
}

// Call executes the skill activation tool.
func (t *ActivateSkillTool) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	var req struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal(rawInput, &req); err != nil {
		return "", errors.Wrap(err, "failed to parse skill activation request")
	}

	skill, ok := t.skills[req.Name]
	if !ok {
		available := make([]string, 0, len(t.skills))
		for name := range t.skills {
			available = append(available, name)
		}
		return "", errors.Errorf("skill %q not found. Available skills: %s", req.Name, fmt.Sprintf("[%s]", fmt.Sprintf("%s", available)))
	}

	activateSkillLogger.WithFields(logrus.Fields{
		"skill":    skill.Name,
		"location": skill.Location,
	}).Info("skill activated")

	// Build response with skill instructions
	response := map[string]any{
		"skill":        skill.Name,
		"location":     skill.Location,
		"instructions": skill.Body,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal skill response")
	}

	return string(responseJSON), nil
}

// SkillToTool converts a single skill into an AFK Tool.
// This creates a dedicated tool for a specific skill (as opposed to the general activator).
func SkillToTool(skill *Skill) (*SkillTool, error) {
	return &SkillTool{
		skill: skill,
	}, nil
}

// SkillTool is an AFK Tool that wraps a single skill.
type SkillTool struct {
	skill *Skill
}

// Param returns the Anthropic tool parameter for this skill.
func (st *SkillTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        st.skill.Name,
		Description: anthropic.String(st.skill.Description),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "The specific task or context for this skill invocation.",
				},
			},
			Required: []string{"task"},
		},
	}
}

// Call executes the skill tool with the provided task context.
func (st *SkillTool) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	var req struct {
		Task string `json:"task"`
	}

	if err := json.Unmarshal(rawInput, &req); err != nil {
		return "", errors.Wrap(err, "failed to parse skill tool request")
	}

	activateSkillLogger.WithFields(logrus.Fields{
		"skill": st.skill.Name,
		"task":  req.Task,
	}).Info("skill tool invoked")

	// Combine skill instructions with task context
	result := fmt.Sprintf("# Skill: %s\n\nLocation: %s\n\n## Instructions\n\n%s\n\n## Current Task\n\n%s",
		st.skill.Name, st.skill.Location, st.skill.Body, req.Task)

	return result, nil
}
