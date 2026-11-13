package project

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// ForkNameSuffix is appended to the source's name when the fork is not given
// an explicit one.
const ForkNameSuffix = " (fork)"

// Fork copies a project's assets — its settings, library files and memory —
// into a new project and returns it. The fork starts like a new project: it
// has no threads and an empty conversation (a fresh main thread whose session
// the coordinator opens on the first message), and it is active even when the
// source was paused or archived. The source is left untouched. A working
// folder cannot be forked: its assets are the folder's own files, shared with
// everything else that works in them.
func (s *Service) Fork(sourceID, name string) (*db.Project, error) {
	src, err := s.Store.GetProject(sourceID)
	if err != nil {
		return nil, err
	}
	if src.Type == db.ProjectFolder {
		return nil, errors.New("a working folder cannot be forked")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSpace(src.Name) + ForkNameSuffix
	}
	fork := &db.Project{
		Name:                 name,
		Goal:                 src.Goal,
		Instructions:         src.Instructions,
		Repos:                src.Repos,
		PrimaryRepoPath:      src.PrimaryRepoPath,
		CoordinatorProfile:   src.CoordinatorProfile,
		ThreadProfile:        src.ThreadProfile,
		CoordinatorModel:     src.CoordinatorModel,
		ThreadModel:          src.ThreadModel,
		CoordinatorEffort:    src.CoordinatorEffort,
		ThreadEffort:         src.ThreadEffort,
		ThreadPermissionMode: src.ThreadPermissionMode,
		DisableWorktree:      src.DisableWorktree,
		MaxRunningThreads:    src.MaxRunningThreads,
	}
	// A row from before the default existed forks like a new project starts;
	// max_running_threads needs no such fill (migrate backfills rows to >= 1
	// at every open, and Create stamps new ones).
	if fork.ThreadPermissionMode == "" {
		fork.ThreadPermissionMode = DefaultPermissionMode
	}
	if err := s.validateDirectoryOverlap(fork); err != nil {
		return nil, err
	}
	if err := s.Store.CreateProjectWithMain(fork, &db.Thread{Origin: "user"}, s.Layout.Root); err != nil {
		return nil, err
	}
	if err := s.Layout.Ensure(fork.ID); err != nil {
		s.rollbackFork(fork)
		return nil, err
	}
	if err := s.Layout.CopyAssets(src.ID, fork.ID); err != nil {
		s.rollbackFork(fork)
		return nil, fmt.Errorf("copying the project's files: %w", err)
	}
	return fork, nil
}

// rollbackFork removes a fork that failed midway — its rows and its fresh
// directory — so no half-made project is left behind. It is best effort: the
// failure that caused the rollback is what the caller sees.
func (s *Service) rollbackFork(p *db.Project) {
	if err := s.Store.DeleteProjectCascade(p.ID); err != nil {
		logrus.WithError(err).WithField("project", p.ID).Error("roll back fork: remove rows")
	}
	if err := os.RemoveAll(s.Layout.Dir(p.ID)); err != nil {
		logrus.WithError(err).WithField("project", p.ID).Error("roll back fork: remove directory")
	}
}

// CopyAssets copies a project's files — its memory and its library — into
// another project's directories, which must exist (Ensure made them). Hidden
// entries (the library's .tmp scratch) and symlinks are left out, the same
// files the Library tab serves; every copied file becomes a regular 0644 one,
// like WriteLibraryFile writes them. The source's MEMORY.md overwrites the
// fresh index Ensure laid down.
func (l Layout) CopyAssets(srcProjectID, dstProjectID string) error {
	for _, pair := range [][2]string{
		{l.MemoryDir(srcProjectID), l.MemoryDir(dstProjectID)},
		{l.LibraryDir(srcProjectID), l.LibraryDir(dstProjectID)},
	} {
		if err := copyTree(pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies srcRoot's regular files into dstRoot, keeping their
// relative paths. Hidden directories (a thread's .tmp scratch) are skipped
// whole, and anything that is not a plain file (a symlink, a device) is left
// out: assets are files.
func copyTree(srcRoot, dstRoot string) error {
	return filepath.WalkDir(srcRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		dst := filepath.Join(dstRoot, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyFile(p, dst)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
