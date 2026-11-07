// Package projects embeds Projects in another Go server, such as Tingly Box.
//
// The host builds an App with its own model access (Host), mounts the routes
// on one of its swagger groups and starts the runtime. Everything under
// /projects and /threads is served the same way the standalone tingly-project binary
// serves it, so the web UI talks to either one unchanged.
//
// The dependency runs one way: the host imports this package; this module
// never imports the host. What Projects needs from the host comes in through
// Host at run time.
package projects

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/projects/internal/config"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	projectmod "github.com/tingly-dev/tingly-box/projects/internal/server/module/project"
	"github.com/tingly-dev/tingly-box/swagger"
)

// DirectoryAuthenticatedKey may be set only after the host has authenticated the request.
const DirectoryAuthenticatedKey = projectmod.DirectoryAuthenticatedKey

// Host is what Projects needs from the server embedding it. The coordinator
// and threads are Claude Code sessions; the host decides where they reach
// the model.
type Host interface {
	// Models lists the models project settings offer for the coordinator
	// and for threads.
	Models(ctx context.Context) ([]Model, error)
	// ThreadEnv is the environment the coordinator and threads run Claude
	// Code with (pointing it at the host's gateway) and the model to use when
	// a project sets none. Empty values leave Claude Code on its own login
	// and default.
	ThreadEnv(ctx context.Context) (env []string, defaultModel string)
}

var _ project.Host = Host(nil)

// Model is one model the project settings offer.
type Model = project.Model

// Optional capabilities a Host can also implement for folder sessions.
type (
	// Launcher routes a session's turns by a profile the host names.
	Launcher = project.Launcher
	// Handoffer names the program that continues a session in a terminal.
	Handoffer = project.Handoffer
	// OutboxHost lets a host that reaches IM (Tingly Box) give Projects the
	// agent.Outbox it speaks through: the coordinator's replies, thread
	// reports and approvals go to the chats bound to a project. The
	// standalone binary has none.
	OutboxHost = project.OutboxHost
	// ChannelFile is a file a chat sent with a message.
	ChannelFile = project.ChannelFile
)

// DefaultDataDir is where Projects keeps its data when the host does not say
// otherwise: $TINGLY_PROJECT_HOME, else ~/.tingly-project. It is deliberately
// outside any host's config directory, so a thread's working directory and its
// parents never hold the host's secrets.
func DefaultDataDir() string { return config.DefaultDataDir() }

// Options configure an App.
type Options struct {
	// DataDir holds the database and each project's memory, library and
	// thread worktrees. Empty keeps everything in memory and a temporary
	// directory, which is what OpenAPI generation needs.
	DataDir string
	// Host provides the models and the Claude Code environment. Required.
	Host Host
	// Agents runs coordinator and thread turns; nil uses the Claude Code
	// CLI. Tests pass a scripted stand-in (agentboot/engine/enginetest).
	Agents *agentboot.AgentService
}

// App is Projects running inside a host.
type App struct {
	store *db.Store
	mods  []module.Module
	svc   *project.Service
	start sync.Once
}

// New opens the Projects database and builds its routes. Nothing runs until
// Start.
func New(opts Options) (*App, error) {
	if opts.Host == nil {
		return nil, errors.New("projects: Options.Host is required")
	}
	dbPath, root := ":memory:", os.TempDir()
	if opts.DataDir != "" {
		cfg := config.Config{DataDir: opts.DataDir}
		dbPath, root = cfg.DBPath(), cfg.ProjectsDir()
	}
	store, err := db.Open(dbPath, db.WithProjectsDir(root))
	if err != nil {
		return nil, err
	}
	deps := server.Deps{Store: store, Layout: project.Layout{Root: root}}
	if opts.Agents != nil {
		deps.Agents = &project.Agents{Service: opts.Agents}
	}
	mods, svc := server.ProjectModules(deps, opts.Host)
	return &App{store: store, mods: mods, svc: svc}, nil
}

// Register mounts every Projects route on api, the host's /api/v1 group
// (behind whatever authentication the host applies to it). All paths start
// with /projects or /threads.
func (a *App) Register(api *swagger.RouteGroup) {
	module.Mount(&module.Routes{API: api}, a.mods...)
}

// PublishFile publishes a local output file to a thread's conversation. The
// caller supplies the trusted thread identity; the file is copied into the
// project's library and the returned ID identifies the conversation event.
func (a *App) PublishFile(ctx context.Context, threadID, path, caption string) (string, error) {
	e, err := a.svc.PublishFile(ctx, threadID, path, caption)
	if err != nil {
		return "", err
	}
	return e.ID, nil
}

// Start recovers threads interrupted by a restart and runs housekeeping
// until ctx ends. Later calls do nothing, so a host can call it whenever the
// feature is switched on.
func (a *App) Start(ctx context.Context) {
	a.start.Do(func() { a.svc.Start(ctx) })
}

// Close stops every thread's turn and Claude Code process, then closes the
// database.
func (a *App) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a.svc.Threads.Shutdown(ctx)
	_ = a.svc.CloseTools(ctx)
	return a.store.Close()
}
