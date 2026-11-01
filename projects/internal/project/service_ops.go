package project

import (
	"context"
	"errors"
	"fmt"
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
	repos, err := ResolveRepos(ctx, spec.Repos)
	if err != nil {
		return nil, err
	}
	p := &db.Project{Name: name, Goal: strings.TrimSpace(spec.Goal), Repos: repos, ThreadPermissionMode: DefaultPermissionMode}
	main := &db.Thread{Origin: "user"}
	if err := s.Store.CreateProjectWithMain(p, main, s.Layout.Root); err != nil {
		return nil, err
	}
	if err := s.Layout.Ensure(p.ID); err != nil {
		return nil, err
	}
	return p, nil
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
	var notice *db.ThreadEvent
	if fields := configurationChanges(previous, p); len(fields) > 0 && p.Type == db.ProjectProject && p.MainThreadID != "" {
		notice = &db.ThreadEvent{ThreadID: p.MainThreadID, Kind: db.EvSystem, Source: db.SourceSystem,
			Content: "Project settings changed: " + strings.Join(fields, ", ") + ". Use the current project settings instead of older values in the conversation. Repository changes apply to new threads; existing threads keep their worktrees.",
			Payload: jsonString(map[string]bool{"settings_changed": true}),
		}
	}
	if err := s.Store.SaveProjectWithEvent(p, notice); err != nil {
		return err
	}
	s.Hub.Publish(Event{Type: EventProject, ProjectID: p.ID, Data: p})
	if notice != nil {
		s.Coordinator.publishEvent(p.ID, notice)
		if !p.Paused && p.ArchivedAt == nil {
			s.Coordinator.schedule(p.ID, func(q *queue) { q.config = true })
		}
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
	} {
		if field.a != field.b {
			fields = append(fields, field.name)
		}
	}
	if !slices.Equal(a.Repos, b.Repos) {
		fields = append(fields, "repositories")
	}
	return fields
}

// SetPaused pauses (stopping every running thread) or resumes a project.
func (s *Service) SetPaused(p *db.Project, paused bool) error {
	p.Paused = paused
	if paused {
		s.Threads.StopProject(p.ID)
	}
	return s.Save(p)
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
	} else {
		p.ArchivedAt = nil
	}
	if err := s.Save(p); err != nil || !archived {
		return err
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
	if t.Agent == db.AgentLLM {
		return s.Send(ctx, threadID, text, atts, src)
	}
	return s.Threads.SendWith(ctx, threadID, text, atts, src, mode)
}
