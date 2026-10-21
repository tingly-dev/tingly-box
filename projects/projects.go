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

	"github.com/tingly-dev/tingly-box/projects/internal/config"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/llm"
	"github.com/tingly-dev/tingly-box/swagger"
)

// Host is what Projects needs from the server embedding it.
type Host interface {
	// ClientFor returns the coordinator's model client and the model to
	// call: model when set, else the host's default.
	ClientFor(ctx context.Context, model string) (llm.Client, string, error)
	// Models lists the models project settings offer for the coordinator
	// and for threads.
	Models(ctx context.Context) ([]llm.Model, error)
	// ThreadEnv is the environment threads run Claude Code with (pointing
	// it at the host's gateway) and the model to use when a project sets
	// none. Empty values leave Claude Code on its own login and default.
	ThreadEnv(ctx context.Context) (env []string, defaultModel string)
}

var _ project.Host = Host(nil)

// Optional capabilities a Host can also implement for folder sessions.
type (
	// Launcher routes a session's turns by a profile the host names.
	Launcher = project.Launcher
	// Handoffer names the program that continues a session in a terminal.
	Handoffer = project.Handoffer
	// ChannelSink delivers text to an IM chat. Projects sends the
	// coordinator's replies and thread reports through it to the chats bound
	// to a project; a host without IM (the standalone binary) has none.
	ChannelSink = project.ChannelSink
	// ChannelPrompter is a ChannelSink that can also send a thread's
	// approvals and questions with buttons and restate them once settled.
	ChannelPrompter = project.ChannelPrompter
	// ChannelPrompt is an approval or a question as a chat shows it.
	ChannelPrompt = project.ChannelPrompt
	// ChannelFile is a file a chat sent with a message.
	ChannelFile = project.ChannelFile
	// ChannelChoice is one button of a ChannelPrompt.
	ChannelChoice = project.ChannelChoice
)

// ErrNoButtons is what a ChannelPrompter returns for a chat that cannot show
// buttons; the chat gets a line of text instead.
var ErrNoButtons = project.ErrNoButtons

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
	// Host provides model access. Required.
	Host Host
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
	mods, svc := server.ProjectModules(server.Deps{Store: store, Layout: project.Layout{Root: root}}, opts.Host)
	return &App{store: store, mods: mods, svc: svc}, nil
}

// Register mounts every Projects route on api, the host's /api/v1 group
// (behind whatever authentication the host applies to it). All paths start
// with /projects or /threads.
func (a *App) Register(api *swagger.RouteGroup) {
	module.Mount(&module.Routes{API: api}, a.mods...)
}

// PostFromChannel hands a message typed in an IM chat to the thread the chat
// is bound to (a project's main thread is its coordinator), as Send does (replies come back
// through the host's ChannelSink). handled is false when the chat is not
// bound, so the host carries on as it did before. files, which the host
// saved, go with the message as its attachments; the host may remove them
// once this returns.
func (a *App) PostFromChannel(platform, chatID, text string, files []ChannelFile) (handled bool, err error) {
	return a.svc.PostFromChannel(platform, chatID, text, files)
}

// ChannelCommand runs a chat command ("new", "stop", "clear", "focus",
// "back", "threads" or "verbose") for a bound IM chat; handled is false when
// the chat has no binding or the command is not one of these. reply is what to tell the chat.
func (a *App) ChannelCommand(platform, chatID, name, arg string) (handled bool, reply string, err error) {
	return a.svc.ChannelCommand(platform, chatID, name, arg)
}

// AnswerFromChannel answers a thread's approval or question with the button an
// IM chat pressed (see ChannelPrompter). reply, when set, is what to tell the
// chat; otherwise the prompt is restated with the outcome.
func (a *App) AnswerFromChannel(platform, chatID, threadID, requestID, value string) (reply string, err error) {
	return a.svc.AnswerFromChannel(platform, chatID, threadID, requestID, value)
}

// ChannelBound reports whether an IM chat has a place in Projects: a thread,
// or a folder its next message starts one in.
func (a *App) ChannelBound(platform, chatID string) bool {
	_, err := a.store.GetChannelBinding(platform, chatID)
	return err == nil
}

// ChannelToFolder moves an IM chat to a folder on this machine (an absolute
// path that exists): its next message starts a new session there, in Projects.
// With ifNew a chat that already has a place is left alone, which is how a
// chat gets its first folder. title names the chat, when known.
func (a *App) ChannelToFolder(platform, chatID, title, folder string, ifNew bool) error {
	return a.svc.ChannelToFolder(platform, chatID, title, folder, ifNew)
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
	return a.store.Close()
}
