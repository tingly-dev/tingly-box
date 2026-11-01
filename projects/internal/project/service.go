// Package project is the runtime behind Projects: a coordinator conversation
// per project that routes work to threads, each a Claude Code session on its
// own git worktree branch. See .design/projects.md.
package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

// Host is what the runtime needs from whoever runs it: a model client for
// the coordinator, the models the project settings offer, and the
// environment threads run Claude Code in. The standalone app implements it
// with its own connection settings; an embedding host (Tingly Box) with its
// gateway.
type Host interface {
	LLMSource
	ThreadEnv
	Models(ctx context.Context) ([]llm.Model, error)
}

// Service bundles the project runtime for the HTTP modules.
type Service struct {
	Store       *db.Store
	Layout      Layout
	Hub         *Hub
	Threads     *Threads
	Coordinator *Coordinator
	Relay       *Relay
	Host        Host
	fileMCP     *fileMCP
}

// NewService wires the runtime. Call Start once the server is up.
func NewService(store *db.Store, layout Layout, agents Agents, host Host) *Service {
	hub := NewHub()
	threads := NewThreads(store, layout, hub, agents, host)
	coord := NewCoordinator(store, layout, hub, threads, host)
	// The chats bound to a project hear the coordinator when the host can
	// reach IM (Tingly Box); the standalone app has no sink.
	sink, _ := host.(ChannelSink)
	relay := NewRelay(store, sink)
	relay.OnFileError = func(threadID, platform, chatID, name string, err error) {
		t, getErr := store.GetThread(threadID)
		if getErr != nil {
			return
		}
		threads.addEvent(t, &db.ThreadEvent{Kind: db.EvError, Content: fmt.Sprintf("Could not send %s to %s (%s): %v", name, platform, chatID, err)})
	}
	coord.OnFinal = relay.Message
	threads.OnWait = relay.NeedsUser
	threads.OnSettled = relay.Settled
	threads.OnReply = relay.ThreadReply
	threads.OnStep = relay.Step
	svc := &Service{
		Store: store, Layout: layout, Hub: hub, Threads: threads,
		Coordinator: coord, Relay: relay,
		Host: host,
	}
	svc.fileMCP = &fileMCP{service: svc}
	threads.FileTool = svc.fileMCP.config
	return svc
}

// Start recovers interrupted threads and runs housekeeping until ctx ends,
// then stops every thread's turn and process.
func (s *Service) Start(ctx context.Context) {
	s.Threads.Recover()
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
				s.Threads.Shutdown(shutdown)
				_ = s.CloseTools(shutdown)
				cancel()
				return
			case <-tick.C:
			}
		}
	}()
}

// CloseTools stops the local MCP service after the thread processes stop.
func (s *Service) CloseTools(ctx context.Context) error {
	if s.fileMCP != nil {
		return s.fileMCP.close(ctx)
	}
	return nil
}

// NewProjectSpec is the New project dialog (a project), or a folder to open.
type NewProjectSpec struct {
	// Type is project (the default) or folder.
	Type  string
	Name  string
	Goal  string
	Repos []string
	// Path is the folder of a folder project.
	Path string
}

// ResolveRepos validates repository paths and records their current branch
// as the base threads branch from.
func ResolveRepos(ctx context.Context, paths []string) ([]db.Repo, error) {
	seen := map[string]bool{}
	repos := []db.Repo{}
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[2:])
			}
		}
		info, err := InspectRepo(ctx, path)
		if err != nil {
			return nil, err
		}
		if seen[info.Root] {
			continue
		}
		seen[info.Root] = true
		repos = append(repos, db.Repo{Path: info.Root, BaseBranch: info.Branch})
	}
	return repos, nil
}
