package server

import (
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/conversation"
	projectmod "github.com/tingly-dev/tingly-box/projects/internal/server/module/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/session"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/settings"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/thread"
)

// Deps are what the modules are built from.
type Deps struct {
	Store  *db.Store
	Layout project.Layout
	// Agents run thread turns; nil uses the Claude Code CLI.
	Agents *project.Agents
}

// ProjectModules builds the Projects modules (projects and their threads)
// on host. The standalone app and an embedding host (package projects) both
// mount exactly this set, under the same /projects paths.
func ProjectModules(d Deps, host project.Host) ([]module.Module, *project.Service) {
	agents := project.ClaudeAgents()
	if d.Agents != nil {
		agents = *d.Agents
	}
	svc := project.NewService(d.Store, d.Layout, agents, host)
	return []module.Module{
		projectmod.NewHandler(svc),
		thread.NewHandler(svc),
		session.NewHandler(svc),
	}, svc
}

// build is the only place that decides which modules the standalone app
// serves and how they are wired. The running server and OpenAPI generation
// both call it, so the spec can never drift from what is served. On top of
// Projects, the standalone app owns what an embedding host provides itself:
// the model connection (settings), Chats and the health check.
func build(d Deps) ([]module.Module, *project.Service) {
	settingsSvc := settings.NewService(d.Store)
	mods, svc := ProjectModules(d, settingsSvc)
	return append(mods,
		conversation.NewHandler(d.Store, settingsSvc),
		settings.NewHandler(settingsSvc),
		healthModule{},
	), svc
}
