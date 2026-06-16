package skill

import (
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// parseSkillContent parses the raw content of a SKILL.md file.
// fallbackName is used as the skill name when the frontmatter omits one.
// source is a human readable identifier (file path or tar entry) used in error messages.
func parseSkillContent(data, fallbackName, source string) (*Skill, error) {
	skill := &Skill{
		Name: fallbackName, // fallback if frontmatter name is absent
	}

	// Normalise line endings
	content := strings.ReplaceAll(data, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		skill.Body = strings.TrimSpace(content)
		return skill, nil
	}

	// Find the closing --- delimiter
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		skill.Body = strings.TrimSpace(content)
		return skill, nil
	}

	// Parse YAML frontmatter
	frontmatter := strings.Join(lines[1:closeIdx], "\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return nil, errors.Wrapf(err, "failed to parse frontmatter: %s", source)
	}

	if fm.Description == "" {
		return nil, errors.Errorf("invalid frontmatter: description is required: %s", source)
	}
	if fm.Name != "" {
		skill.Name = fm.Name
	}
	skill.Description = fm.Description

	// Body is everything after the closing ---
	skill.Body = strings.TrimSpace(strings.Join(lines[closeIdx+1:], "\n"))

	skill.AllowedTools = splitBySpaceOrComma(fm.AllowedTools)
	skill.Agents = splitBySpaceOrComma(fm.Agents)
	skill.Tags = splitBySpaceOrComma(fm.Tags)

	return skill, nil
}

func splitBySpaceOrComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ','
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
