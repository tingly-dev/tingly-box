// Package skill implements Agent Skills support for the AFK framework.
//
// A skill is a directory containing a SKILL.md file with YAML front-matter
// (name, description) and markdown instructions. Skills are discovered at
// session start via filesystem scan and loaded on demand.
package skill

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/sirupsen/logrus"
)

var logger = logrus.WithField("package", "afk.skill")

// Frontmatter holds the YAML metadata parsed from the header of a SKILL.md file.
type Frontmatter struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	License       string `yaml:"license"`
	Compatibility string `yaml:"compatibility"`
	// AllowedTools is a space or comma separated list of tool names that are allowed to be used in the skill.
	AllowedTools string `yaml:"allowed-tools"`
	// Agents is a space or comma separated list of agent names that are allowed to use this skill.
	Agents string `yaml:"agents"`
	// Tags is a space or comma separated list of tags for filtering skills.
	Tags string `yaml:"tags"`
	// Metadata is a map of key-value pairs for additional metadata.
	Metadata map[string]string `yaml:"metadata"`
}

// Skill represents a parsed agent skill.
type Skill struct {
	Name         string
	Description  string
	AllowedTools []string
	Agents       []string
	Tags         []string
	Metadata     map[string]string
	// Body is the markdown body after the frontmatter block
	Body string
	// Location is the relative path to the skill directory
	Location string

	// dir is the absolute path to the skill directory
	dir string
	// path is the absolute path to the SKILL.md file
	path string
	// resourceNames is a list of resource names in the skill directory
	resourceNames []string
	// resources is a map of resource names to their content
	resources map[string][]byte
}

type Skills []*Skill

func (s Skills) Names() []string {
	if len(s) == 0 {
		return nil
	}
	names := make([]string, 0, len(s))
	for _, skill := range s {
		names = append(names, skill.Name)
	}
	return names
}

// Filter returns a new Skills list filtered by the given name and tags.
// If name is empty, the skills are matched by tags only.
func (s Skills) Filter(name string, tags ...string) Skills {
	if name == "" && len(tags) == 0 {
		return s
	}
	filtered := make(Skills, 0, len(s))
	for _, skill := range s {
		if name != "" && skill.Name != name {
			continue
		}

		tagsFound := true
		for _, tag := range tags {
			if !slices.Contains(skill.Tags, tag) {
				tagsFound = false
				break
			}
		}
		if tagsFound {
			filtered = append(filtered, skill)
		}
	}
	return filtered
}

// ListResources returns relative paths to all non-SKILL.md files in the
// skill directory, including files in subdirectories (scripts/, references/, assets/).
func (s *Skill) ListResources() []string {
	if len(s.resourceNames) > 0 {
		return s.resourceNames
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			subEntries, _ := os.ReadDir(filepath.Join(s.dir, entry.Name()))
			for _, sub := range subEntries {
				if !sub.IsDir() {
					files = append(files, filepath.Join(entry.Name(), sub.Name()))
				}
			}
		} else if entry.Name() != "SKILL.md" {
			files = append(files, entry.Name())
		}
	}
	s.resourceNames = files
	return files
}

// LoadResources loads all resource files into memory.
func (s *Skill) LoadResources() map[string][]byte {
	if s.resources != nil {
		return s.resources
	}
	resources := make(map[string][]byte)

	for _, resName := range s.ListResources() {
		content, err := os.ReadFile(filepath.Join(s.dir, resName))
		if err != nil {
			continue
		}
		resources[resName] = content
	}
	s.resources = resources
	return resources
}
