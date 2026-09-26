package config

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/internal"
)

// Agent skills bundled in the binary (internal.SkillAssets) and their
// installation into an agent's skills directory, e.g. ~/.claude/skills.

// BundledSkills lists the names of the skills embedded in the binary.
func BundledSkills() ([]string, error) {
	entries, err := fs.ReadDir(internal.SkillAssets, "skills")
	if err != nil {
		return nil, fmt.Errorf("failed to read bundled skills: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// InstallClaudeSkill installs a bundled skill into ~/.claude/skills/<name>.
func InstallClaudeSkill(name string) (skillDir string, created bool, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("failed to get home directory: %w", err)
	}
	return InstallSkill(name, filepath.Join(homeDir, ".claude", "skills"))
}

// InstallSkill copies the bundled skill <name> into <skillsRoot>/<name>,
// rewriting only files whose content differs; created reports whether any file
// was new. Files under scripts/ are made
// executable. Files the user added to the directory are left alone.
func InstallSkill(name, skillsRoot string) (skillDir string, created bool, err error) {
	src := path.Join("skills", name)
	if _, err := fs.Stat(internal.SkillAssets, path.Join(src, "SKILL.md")); err != nil {
		return "", false, fmt.Errorf("unknown bundled skill %q", name)
	}
	skillDir = filepath.Join(skillsRoot, name)

	err = fs.WalkDir(internal.SkillAssets, src, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		content, err := internal.SkillAssets.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, src+"/")
		target := filepath.Join(skillDir, filepath.FromSlash(rel))
		if err := ensureDir(target); err != nil {
			return err
		}
		perm := os.FileMode(0644)
		if strings.HasPrefix(rel, "scripts/") {
			perm = 0755
		}
		fileCreated, err := writeManagedFileIfChanged(target, content, perm)
		if err != nil {
			return fmt.Errorf("failed to write %s: %w", target, err)
		}
		created = created || fileCreated
		return nil
	})
	if err != nil {
		return "", false, err
	}
	return skillDir, created, nil
}
