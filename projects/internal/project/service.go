// Package project is the runtime behind Projects: a coordinator conversation
// per project that routes work to threads, each a Claude Code session on its
// own git worktree branch. See .design/projects.md.
package project

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/remote/agent"
)

// Host is what the runtime needs from whoever runs it: the models the project
// settings offer and the environment the coordinator and threads run Claude
// Code in. The standalone app implements it with its own connection settings;
// an embedding host (Tingly Box) with its gateway.
type Host interface {
	ThreadEnv
	Models(ctx context.Context) ([]Model, error)
}

// Model is one model the project settings offer.
type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	// ProviderModel / ProviderName are what the host routes ID to — the real
	// model an alias stands for, and the provider that serves it. Set by an
	// embedding host that can resolve its routing (Tingly Box does, the way
	// its status line does); empty when the host says nothing.
	ProviderModel string `json:"provider_model,omitempty"`
	ProviderName  string `json:"provider_name,omitempty"`
}

// Service bundles the project runtime for the HTTP modules.
type Service struct {
	Store           *db.Store
	Layout          Layout
	Hub             *Hub
	Threads         *Threads
	Coordinator     *Coordinator
	Relay           *Relay
	Host            Host
	sessionToolsMCP *sessionToolsMCP
	stopOnce        sync.Once
}

// NewService wires the runtime. Call Start once the server is up.
func NewService(store *db.Store, layout Layout, agents Agents, host Host) *Service {
	hub := NewHub()
	threads := NewThreads(store, layout, hub, agents, host)
	coord := NewCoordinator(store, layout, hub, threads)
	// The chats bound to a project hear the coordinator when the host can
	// reach IM (Tingly Box); the standalone app has no sink.
	var sink agent.Outbox
	if h, ok := host.(OutboxHost); ok {
		sink = h.Outbox()
	}
	relay := NewRelay(store, sink)
	relay.OnFileError = func(threadID, platform, chatID, name string, err error) {
		t, getErr := store.GetThread(threadID)
		if getErr != nil {
			return
		}
		threads.addEvent(t, &db.ThreadEvent{Kind: db.EvError, Content: fmt.Sprintf("Could not send %s to %s (%s): %v", name, platform, chatID, err)})
	}
	coord.OnFinal = relay.Message
	coord.OnDecided = relay.Decided
	threads.OnWait = relay.NeedsUser
	threads.OnSettled = relay.Settled
	threads.OnReply = relay.ThreadReply
	threads.OnStep = relay.Step
	svc := &Service{
		Store: store, Layout: layout, Hub: hub, Threads: threads,
		Coordinator: coord, Relay: relay,
		Host: host,
	}
	svc.sessionToolsMCP = &sessionToolsMCP{service: svc}
	threads.SessionToolsConfig = svc.sessionToolsMCP.config
	threads.RevokeTools = svc.sessionToolsMCP.revoke
	coord.sessions = newCoordinatorSessions(coord, host, svc.sessionToolsMCP, agents)
	return svc
}

// Start recovers interrupted threads and runs housekeeping until ctx ends,
// then stops every thread's turn and process.
func (s *Service) Start(ctx context.Context) {
	s.Threads.Recover()
	if err := s.Store.RecoverCoordinatorInputs(); err != nil {
		logrus.WithError(err).Error("recover coordinator inputs")
	} else if ids, err := s.Store.CoordinatorProjectsWithInputs(); err != nil {
		logrus.WithError(err).Error("list coordinator inputs")
	} else {
		for _, id := range ids {
			s.Coordinator.wake(id)
		}
	}
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				s.Threads.CheckStalled(now)
			}
		}
	}()
	go func() {
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		for {
			s.Threads.AutoResolve(7 * 24 * time.Hour)
			select {
			case <-ctx.Done():
				shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				s.Stop(shutdown)
				cancel()
				return
			case <-tick.C:
			}
		}
	}()
}

// Stop ends every coordinator and thread turn, waits until their bookkeeping
// is done (or ctx ends) and closes the local tools. Start calls it once its
// ctx ends; calling it again waits for that first stop to finish.
func (s *Service) Stop(ctx context.Context) {
	s.stopOnce.Do(func() {
		s.Coordinator.shutdown(ctx)
		s.Threads.Shutdown(ctx)
		s.Threads.waitIdle(ctx)
		_ = s.CloseTools(ctx)
	})
}

// CloseTools stops the local MCP service after the thread processes stop.
func (s *Service) CloseTools(ctx context.Context) error {
	if s.Coordinator != nil {
		s.Coordinator.shutdown(ctx)
	}
	if s.sessionToolsMCP != nil {
		return s.sessionToolsMCP.close(ctx)
	}
	return nil
}

// NewProjectSpec is the New project dialog (a project), or a folder to open.
type NewProjectSpec struct {
	// Type is project (the default) or folder.
	Type            string
	Name            string
	Goal            string
	Repos           []string
	Directories     *[]DirectorySpec
	PrimaryRepoPath *string
	// Path is the folder of a folder project.
	Path string
}

// LegacyDirectorySpecs preserves the old path-list API's relative paths and blanks.
func LegacyDirectorySpecs(paths []string) ([]DirectorySpec, error) {
	specs := make([]DirectorySpec, 0, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) && path != "~" && !strings.HasPrefix(path, "~/") {
			var err error
			path, err = filepath.Abs(path)
			if err != nil {
				return nil, err
			}
		}
		specs = append(specs, DirectorySpec{Path: path})
	}
	return specs, nil
}

func ResolveRepos(ctx context.Context, paths []string) ([]db.Repo, error) {
	specs, err := LegacyDirectorySpecs(paths)
	if err != nil {
		return nil, err
	}
	return ResolveDirectories(ctx, specs, nil, false)
}

// DirectorySpec is the user-editable part of a project directory.
type DirectorySpec struct {
	Path          string `json:"path"`
	WorkspaceMode string `json:"workspace_mode,omitempty" enum:",shared,worktree"`
}

// DirectoryInfo describes an existing directory and, when applicable, its Git root.
type DirectoryInfo struct {
	Root   string `json:"root"`
	Kind   string `json:"kind"`
	Branch string `json:"branch,omitempty"`
	Dirty  bool   `json:"dirty,omitempty"`
}

func InspectDirectory(ctx context.Context, raw string) (DirectoryInfo, error) {
	path, err := CleanFolder(raw)
	if err != nil {
		return DirectoryInfo{}, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return DirectoryInfo{}, err
	}
	root, err := git(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return DirectoryInfo{Root: path, Kind: db.RepoDirectory}, nil
		}
		return DirectoryInfo{}, err
	}
	info, err := InspectRepo(ctx, root)
	if err != nil {
		return DirectoryInfo{}, err
	}
	root, err = filepath.EvalSymlinks(info.Root)
	if err != nil {
		return DirectoryInfo{}, err
	}
	return DirectoryInfo{Root: root, Kind: db.RepoGit, Branch: info.Branch, Dirty: info.Dirty}, nil
}

// ResolveDirectories checks paths and applies defaults, preserving previously
// configured modes and base branches for unchanged directories.
func ResolveDirectories(ctx context.Context, specs []DirectorySpec, previous []db.Repo, disabled bool) ([]db.Repo, error) {
	seen := map[string]bool{}
	result := []db.Repo{}
	oldProject := db.Project{DisableWorktree: disabled}
	for _, spec := range specs {
		info, err := InspectDirectory(ctx, spec.Path)
		if err != nil {
			return nil, err
		}
		key := db.PathKey(info.Root)
		if seen[key] {
			if spec.WorkspaceMode != "" {
				for _, existing := range result {
					if db.PathKey(existing.Path) == key && existing.WorkspaceMode != spec.WorkspaceMode {
						return nil, fmt.Errorf("conflicting modes for directory %q", info.Root)
					}
				}
			}
			continue
		}
		seen[key] = true
		r := db.Repo{Path: info.Root, BaseBranch: info.Branch, Kind: info.Kind}
		for _, old := range previous {
			if db.PathKey(old.Path) == key && oldProject.EffectiveRepo(old).Kind == r.Kind {
				old = oldProject.EffectiveRepo(old)
				r.BaseBranch, r.WorkspaceMode = old.BaseBranch, old.WorkspaceMode
				break
			}
		}
		if spec.WorkspaceMode != "" {
			r.WorkspaceMode = spec.WorkspaceMode
		}
		if r.WorkspaceMode == "" {
			if r.Kind == db.RepoDirectory || disabled {
				r.WorkspaceMode = db.WorkspaceShared
			} else {
				r.WorkspaceMode = db.WorkspaceWorktree
			}
		}
		if r.WorkspaceMode != db.WorkspaceShared && r.WorkspaceMode != db.WorkspaceWorktree {
			return nil, fmt.Errorf("invalid workspace mode %q", r.WorkspaceMode)
		}
		if r.Kind == db.RepoDirectory && r.WorkspaceMode != db.WorkspaceShared {
			return nil, fmt.Errorf("non-Git directory %q must use shared mode", r.Path)
		}
		result = append(result, r)
	}
	return result, nil
}

// SetProjectDirectories atomically validates the selected primary before mutation.
func SetProjectDirectories(p *db.Project, repos []db.Repo, primary string) error {
	if len(repos) == 0 {
		if primary != "" {
			return fmt.Errorf("primary directory is not part of this project")
		}
		p.Repos, p.PrimaryRepoPath = repos, ""
		return nil
	}
	if primary == "" {
		primary = repos[0].Path
	}
	for _, r := range repos {
		if db.PathKey(r.Path) == db.PathKey(primary) {
			p.Repos, p.PrimaryRepoPath = repos, r.Path
			return nil
		}
	}
	return fmt.Errorf("primary directory %q is not part of this project", primary)
}
