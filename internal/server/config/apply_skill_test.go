package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBundledSkillsIncludesImagegen(t *testing.T) {
	names, err := BundledSkills()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names, "imagegen") {
		t.Fatalf("bundled skills = %v, want imagegen", names)
	}
}

func TestInstallSkill(t *testing.T) {
	root := t.TempDir()

	dir, created, err := InstallSkill("imagegen", root)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first install should report created")
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md not installed: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "scripts", "image.py"))
	if err != nil {
		t.Fatalf("script not installed: %v", err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Fatalf("script not executable: %v", info.Mode())
	}

	// A user-added file survives a reinstall; a second install creates nothing.
	extra := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(extra, []byte("mine"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, created, err = InstallSkill("imagegen", root); err != nil || created {
		t.Fatalf("reinstall: created=%v err=%v", created, err)
	}
	if _, err := os.Stat(extra); err != nil {
		t.Fatalf("user file removed: %v", err)
	}

	if _, _, err := InstallSkill("does-not-exist", root); err == nil {
		t.Fatal("unknown skill should fail")
	}
}
