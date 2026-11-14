package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Create makes a project with its main thread and directories, or opens the
// folder a folder spec names (the folder's existing project when there is one).
func (s *Service) Create(ctx context.Context, spec NewProjectSpec) (*db.Project, error) {
	if spec.Type == db.ProjectFolder {
		path, err := CleanFolder(spec.Path)
		if err != nil {
			return nil, err
		}
		return s.Store.OpenFolder(path, spec.Name)
	}
	if spec.Type != "" && spec.Type != db.ProjectProject {
		return nil, fmt.Errorf("unknown project type %q", spec.Type)
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	var repos []db.Repo
	var err error
	if spec.Directories != nil {
		if len(spec.Repos) != 0 {
			return nil, errors.New("cannot specify both repos and directories")
		}
		repos, err = ResolveDirectories(ctx, *spec.Directories, nil, false)
	} else {
		repos, err = ResolveRepos(ctx, spec.Repos)
	}
	if err != nil {
		return nil, err
	}
	p := &db.Project{Name: name, Goal: strings.TrimSpace(spec.Goal), ThreadPermissionMode: DefaultPermissionMode, MaxRunningThreads: db.DefaultMaxRunningThreads}
	primary := ""
	if spec.PrimaryRepoPath != nil {
		primary = *spec.PrimaryRepoPath
	}
	if err := SetProjectDirectories(p, repos, primary); err != nil {
		return nil, err
	}
	if err := s.validateDirectoryOverlap(p); err != nil {
		return nil, err
	}
	main := &db.Thread{Origin: "user"}
	if err := s.Store.CreateProjectWithMain(p, main, s.Layout.Root); err != nil {
		return nil, err
	}
	if err := s.Layout.Ensure(p.ID); err != nil {
		return nil, err
	}
	return p, nil
}

// validateDirectoryOverlap prevents workspace files being placed inside the
// application-owned data tree, or making that tree part of a workspace.
func (s *Service) validateDirectoryOverlap(p *db.Project) error {
	if s.Layout.Root == "" {
		return nil
	}
	root, err := filepath.Abs(s.Layout.Root)
	if err != nil {
		return err
	}
	if resolved, e := filepath.EvalSymlinks(root); e == nil {
		root = resolved
	} else if !os.IsNotExist(e) {
		return e
	}
	inside := func(parent, child string) bool {
		rel, e := filepath.Rel(parent, child)
		return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	for _, repo := range p.Repos {
		if inside(root, repo.Path) || inside(repo.Path, root) {
			return fmt.Errorf("directory %q overlaps Projects data directory", repo.Path)
		}
	}
	return nil
}

// Save persists a project change and tells open pages and the coordinator.
func (s *Service) Save(p *db.Project) error {
	if len([]rune(p.Instructions)) > MaxInstructions {
		return fmt.Errorf("instructions exceed %d characters", MaxInstructions)
	}
	previous, err := s.Store.GetProject(p.ID)
	if err != nil {
		return err
	}
	// Unrelated edits to old projects remain possible even if their historical
	// directory overlaps the data root; newly selected directories are checked.
	if !slices.Equal(previous.Repos, p.Repos) {
		if err := s.validateDirectoryOverlap(p); err != nil {
			return err
		}
	}
	var notice *db.ThreadEvent
	if fields := configurationChanges(previous, p); len(fields) > 0 && p.Type == db.ProjectProject && p.MainThreadID != "" {
		notice = &db.ThreadEvent{ThreadID: p.MainThreadID, Kind: db.EvSystem, Source: db.SourceSystem,
			Content: "Project settings changed: " + strings.Join(fields, ", ") + ". Use the current project settings instead of older values in the conversation. Repository and worktree settings apply to new threads; existing threads keep their working directories.",
			Payload: jsonString(map[string]bool{"settings_changed": true}),
		}
	}
	if err := s.Store.SaveProjectWithEvent(p, notice); err != nil {
		return err
	}
	view := *p
	view.Repos = append([]db.Repo(nil), p.Repos...)
	view.ApplyDirectoryDefaults()
	s.Hub.Publish(Event{Type: EventProject, ProjectID: p.ID, Data: &view})
	if s.Threads != nil {
		s.Threads.ApplyLimit(p)
	}
	if notice != nil {
		s.Coordinator.publishEvent(p.ID, notice)
	}
	return nil
}

// Navigation metadata does not change how work runs and must not wake a model.
func configurationChanges(a, b *db.Project) []string {
	var fields []string
	for _, field := range []struct {
		name string
		a, b string
	}{
		{"name", a.Name, b.Name}, {"goal", a.Goal, b.Goal},
		{"instructions", a.Instructions, b.Instructions},
		{"coordinator profile", a.CoordinatorProfile, b.CoordinatorProfile},
		{"thread profile", a.ThreadProfile, b.ThreadProfile},
		{"coordinator model", a.CoordinatorModel, b.CoordinatorModel},
		{"thread model", a.ThreadModel, b.ThreadModel},
		{"coordinator effort", a.CoordinatorEffort, b.CoordinatorEffort},
		{"thread effort", a.ThreadEffort, b.ThreadEffort},
		{"thread permission mode", a.ThreadPermissionMode, b.ThreadPermissionMode},
		{"thread GitHub access", a.ThreadGitHubAccess, b.ThreadGitHubAccess},
	} {
		if field.a != field.b {
			fields = append(fields, field.name)
		}
	}
	if a.DisableWorktree != b.DisableWorktree {
		fields = append(fields, "thread worktree mode")
	}
	if a.MaxRunningThreads != b.MaxRunningThreads {
		fields = append(fields, "thread concurrency limit")
	}
	if !slices.Equal(a.Repos, b.Repos) || a.PrimaryRepoPath != b.PrimaryRepoPath {
		fields = append(fields, "repositories")
	}
	return fields
}

// SetPaused pauses (stopping every running thread) or resumes a project.
func (s *Service) SetPaused(p *db.Project, paused bool) error {
	p.Paused = paused
	if paused {
		s.Threads.StopProject(p.ID)
		s.Coordinator.stop(p)
	}
	if err := s.Save(p); err != nil {
		return err
	}
	if !paused && s.Coordinator.live() && p.MainThreadID != "" {
		s.Coordinator.wake(p.ID)
	}
	return nil
}

// SetArchived archives (stopping its threads) or unarchives a project or a
// folder. Archiving is the only way to remove either: threads, worktrees,
// branches and memory stay. Its IM chats are unbound, so they go back to what
// an unbound chat does instead of failing on every message; unarchiving does
// not bind them again.
func (s *Service) SetArchived(p *db.Project, archived bool) error {
	if archived {
		now := time.Now()
		p.ArchivedAt = &now
		s.Threads.StopProject(p.ID)
		s.Coordinator.stop(p)
	} else {
		p.ArchivedAt = nil
	}
	if err := s.Save(p); err != nil {
		return err
	}
	if archived {
		// The project now refuses messages, so nothing can start a session
		// behind this: close the threads' processes and end their tools.
		s.Threads.ReleaseProject(p.ID)
	}
	if !archived {
		if s.Coordinator.live() && p.MainThreadID != "" {
			s.Coordinator.wake(p.ID)
		}
		return nil
	}
	_, err := s.Store.DeleteProjectChannelBindings(p.ID)
	return err
}

// Send gives a thread a message, the one way anything talks to a thread: the
// web, an IM chat and another thread differ only in src. A main thread's
// message goes to its project's coordinator, any other thread's to its agent.
// Both run it after the thread's current turn, and both refuse an archived or
// paused project. It returns the thread as it is now.
func (s *Service) Send(ctx context.Context, threadID, text string, atts []Attachment, src Source) (*db.Thread, error) {
	t, err := s.Store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if cmd, ok := personCommand(text, src, len(atts)); ok {
		return s.runCommand(ctx, t, cmd, src)
	}
	if t.Agent == db.AgentLLM {
		if _, err := s.Coordinator.Post(t.ProjectID, text, atts, src); err != nil {
			return nil, err
		}
		return s.Store.GetThread(threadID)
	}
	return s.Threads.Send(ctx, threadID, text, src, atts)
}

// SendWith is Send for a user in the web: while a turn runs, mode says
// whether the message joins it, waits for it or replaces it. The coordinator
// takes every message the same way.
func (s *Service) SendWith(ctx context.Context, threadID, text string, atts []Attachment, src Source, mode SendMode) (*db.Thread, error) {
	t, err := s.Store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if _, isCommand := personCommand(text, src, len(atts)); isCommand || t.Agent == db.AgentLLM {
		return s.Send(ctx, threadID, text, atts, src)
	}
	return s.Threads.SendWith(ctx, threadID, text, atts, src, mode)
}

// runCommand runs a slash command a person typed to thread t. Unsupported
// ones are refused with a reason; none is ever sent on to the agent as text.
func (s *Service) runCommand(ctx context.Context, t *db.Thread, cmd command, src Source) (*db.Thread, error) {
	if cmd.kind == cmdUnsupported {
		return nil, &ErrCommandUnsupported{Name: cmd.name}
	}
	if cmd.kind == cmdClear {
		if cmd.args != "" {
			return nil, fmt.Errorf("/clear does not take arguments; send your next message separately")
		}
		if t.Agent == db.AgentLLM {
			return nil, fmt.Errorf("/clear is not available for the project coordinator")
		}
		return s.Threads.NewSession(t.ID, src)
	}
	if t.Agent == db.AgentLLM {
		// The conversation is compacted by its own session: the command is
		// queued like any message and the coordinator runs it in its turn.
		if _, err := s.Coordinator.Post(t.ProjectID, compactText(cmd.args), nil, src); err != nil {
			return nil, err
		}
		return s.Store.GetThread(t.ID)
	}
	return s.Threads.Compact(ctx, t.ID, cmd.args, src)
}
