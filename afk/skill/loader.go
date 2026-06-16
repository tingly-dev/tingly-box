package skill

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// SkillStandard specifies which skill standard to follow.
type SkillStandard string

const (
	// ClaudeStandard follows Claude Code conventions:
	// - Uses .claude/skills/ and .agent/skills/ directories
	// - SKILL.md format with YAML frontmatter (name, description, tags)
	// - Progressive disclosure pattern
	// - Compatible with Claude Code skill ecosystem
	ClaudeStandard SkillStandard = "claude"

	// AgentStandard follows generic agent industry conventions:
	// - Uses .agents/skills/ directory
	// - Supports agents.md configuration files
	// - More flexible directory structure for different agent types
	// - Compatible with broader agent ecosystem
	AgentStandard SkillStandard = "agents"
)

// Config specifies the configuration for the loader.
//
// Claude Standard directory structure:
// .claude/skills/ or .agent/skills/
// ├── skill-name/
// │   └── SKILL.md
//
// Agent Standard directory structure:
// .agents/skills/
// ├── agent-name/
// │   └── skill-name/
// │       └── SKILL.md
type Config struct {
	// Standard specifies which skill standard to follow (ClaudeStandard or AgentStandard)
	// Defaults to ClaudeStandard if not specified
	Standard SkillStandard
	// Strict mode enforces that an error is returned if a skill is invalid or failed to parse.
	Strict bool
	// EnableDefaultSkills enables the default skills,
	// when the skill path does not include Agent name or Skill does not specify Agents field.
	EnableDefaultSkills bool
	// EnableStandardPaths enables to discover the standard paths for skills.
	EnableStandardPaths bool
	// Paths specifies additional paths to search for skills.
	// Later directories override earlier ones when two skills share the same name.
	Paths []string
	// ConfigFile specifies the path to configuration file:
	// - ClaudeStandard: claude.md
	// - AgentStandard: agents.md
	// If empty, will search in standard locations
	ConfigFile string
}

// Loader discovers and parses Agent Skills from the filesystem.
type Loader struct {
	cfg         *Config
	searchDirs  []string
	skills      map[string]*Skill   // skill name -> skill
	skillsByTag map[string][]*Skill // tag -> skills
	standard    SkillStandard
}

// NewLoader creates a new skill loader.
func NewLoader(cfg *Config) (*Loader, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user home directory")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get working directory")
	}

	var dirs []string

	// Set default standard if not specified
	if cfg.Standard == "" {
		cfg.Standard = ClaudeStandard
	}

	if cfg.EnableStandardPaths {
		switch cfg.Standard {
		case ClaudeStandard:
			// Claude Code standard: prioritize .claude/skills/
			dirs = append(dirs, filepath.Join(home, ".claude", "skills"))
			dirs = append(dirs, filepath.Join(cwd, ".claude", "skills"))
			dirs = append(dirs, filepath.Join(home, ".agent", "skills"))
			dirs = append(dirs, filepath.Join(cwd, ".agent", "skills"))

		case AgentStandard:
			// Agent industry standard: .agents/skills/
			dirs = append(dirs, filepath.Join(home, ".agents", "skills"))
			dirs = append(dirs, filepath.Join(cwd, ".agents", "skills"))
		}
	}

	// Caller-supplied explicit overrides (highest priority)
	for _, dir := range cfg.Paths {
		dirs = append(dirs, filepath.Clean(dir))
	}

	l := &Loader{
		cfg:         cfg,
		searchDirs:  dirs,
		skills:      make(map[string]*Skill),
		skillsByTag: make(map[string][]*Skill),
		standard:    cfg.Standard,
	}

	if err := l.load(); err != nil {
		return nil, err
	}

	logger.WithFields(logrus.Fields{
		"skills_found":   len(l.skills),
		"search_dirs":    len(l.searchDirs),
		"standard_paths": cfg.EnableStandardPaths,
		"standard":       cfg.Standard,
		"config_file":    cfg.ConfigFile,
	}).Info("skill loader initialized")

	return l, nil
}

// Skills returns all loaded skills sorted alphabetically by name.
func (l *Loader) Skills() Skills {
	skills := make(Skills, 0, len(l.skills))
	for _, skill := range l.skills {
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills
}

// GetSkill returns a skill by name.
func (l *Loader) GetSkill(name string) (*Skill, bool) {
	skill, ok := l.skills[name]
	return skill, ok
}

// FilterByTag returns skills that have all the specified tags.
func (l *Loader) FilterByTag(tags ...string) Skills {
	if len(tags) == 0 {
		return l.Skills()
	}

	var result Skills
	for _, skill := range l.Skills() {
		hasAllTags := true
		for _, tag := range tags {
			if !slices.Contains(skill.Tags, tag) {
				hasAllTags = false
				break
			}
		}
		if hasAllTags {
			result = append(result, skill)
		}
	}
	return result
}

// load scans all configured directories for SKILL.md files and parses them.
func (l *Loader) load() error {
	for _, dir := range l.searchDirs {
		if err := l.loadFolder(dir); err != nil {
			if l.cfg.Strict {
				return errors.Wrapf(err, "failed to load directory: %s", dir)
			}
			logger.WithError(err).WithField("dir", dir).Warn("failed to load skill directory")
		}
	}
	return nil
}

func (l *Loader) loadFolder(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "failed to read directory")
		}
		return nil // Directory doesn't exist, skip it
	}

	for _, entry := range entries {
		name := entry.Name()
		isAgentFolder := strings.HasPrefix(name, ".")
		entryPath := filepath.Join(dir, name)

		// entry can be a symlink as well as a directory
		// so we need to stat the entry to get the actual path
		info, err := os.Stat(entryPath)
		if err != nil {
			if !os.IsNotExist(err) {
				logger.WithError(err).WithField("path", entryPath).Warn("failed to stat entry")
			}
			continue
		}
		if !info.IsDir() {
			continue
		}

		// Recursively load agent-specific skills
		if isAgentFolder {
			if err := l.loadFolder(entryPath); err != nil {
				logger.WithError(err).WithField("agent_dir", entryPath).Warn("failed to load agent folder")
			}
			continue
		}

		// Check if this is a skill directory (contains SKILL.md)
		skillMdPath := filepath.Join(entryPath, "SKILL.md")
		if _, err := os.Stat(skillMdPath); os.IsNotExist(err) {
			continue // Not a skill directory
		}

		skill, err := parseSkillFile(skillMdPath)
		if err != nil {
			logger.WithError(err).WithField("path", skillMdPath).Warn("failed to parse skill file")
			if l.cfg.Strict {
				return errors.Wrapf(err, "failed to parse skill: %s", skillMdPath)
			}
			continue
		}

		// Calculate relative location
		rel, err := filepath.Rel(dir, skill.dir)
		if err != nil {
			rel = strings.TrimPrefix(skill.dir, dir)
		}
		skill.Location = filepath.ToSlash(rel)

		l.addSkill(skill)
	}

	return nil
}

// addSkill registers a parsed skill. Later skills with the same name override earlier ones.
func (l *Loader) addSkill(skill *Skill) {
	// Check if skill should be loaded based on agents configuration
	if !l.cfg.EnableDefaultSkills && len(skill.Agents) == 0 {
		return
	}

	// Add to skills map (override if exists)
	l.skills[skill.Name] = skill

	// Update tag index
	for _, tag := range skill.Tags {
		l.skillsByTag[tag] = append(l.skillsByTag[tag], skill)
	}

	logger.WithFields(logrus.Fields{
		"skill":    skill.Name,
		"location": skill.Location,
		"tags":     strings.Join(skill.Tags, ","),
		"agents":   strings.Join(skill.Agents, ","),
	}).Debug("skill loaded")
}

// parseSkillFile reads and parses a SKILL.md file.
// If the frontmatter name is absent, the parent directory name is used as a fallback.
func parseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read skill file")
	}

	dir := filepath.Dir(path)
	skill, err := parseSkillContent(string(data), filepath.Base(dir), path)
	if err != nil {
		return nil, err
	}
	skill.dir = dir
	skill.path = path
	return skill, nil
}
