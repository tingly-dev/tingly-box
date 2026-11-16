package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/command/options"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	projectsmodule "github.com/tingly-dev/tingly-box/internal/server/module/projects"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
	remotescenario "github.com/tingly-dev/tingly-box/remote/scenario"
	"github.com/tingly-dev/tingly-box/swagger"
)

// GlobalServerManager manages the global server instance for web UI control
var (
	globalServer     *Server
	globalServerLock sync.RWMutex
	shutdownChan     = make(chan struct{}, 1)
)

// SetGlobalServer sets the global server instance for web UI control
func SetGlobalServer(server *Server) {
	globalServerLock.Lock()
	defer globalServerLock.Unlock()
	globalServer = server
}

// GetGlobalServer gets the global server instance
func GetGlobalServer() *Server {
	globalServerLock.RLock()
	defer globalServerLock.RUnlock()
	return globalServer
}

// GetShutdownChannel returns the shutdown channel for the main process to listen on
func GetShutdownChannel() <-chan struct{} {
	return shutdownChan
}

func (s *Server) StopServer(c *gin.Context) {
	// Get the global server instance
	server := GetGlobalServer()
	if server == nil {
		apierr.Failure(c, http.StatusServiceUnavailable, "No server instance available to stop")
		return
	}

	// Stop the server gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		apierr.Failure(c, http.StatusInternalServerError, fmt.Sprintf("Failed to stop server: %v", err))
		return
	}

	// Log the action
	logrus.WithFields(logrus.Fields{
		"action": obs.ActionStopServer,
		"source": "web_ui",
	}).Info("Server stopped via web interface")

	// Send shutdown signal to main process
	select {
	case shutdownChan <- struct{}{}:
	default:
		// Channel already has a signal
	}

	response := ServerActionResponse{
		Success: true,
		Message: "Server stopped successfully. The application will now exit.",
	}

	c.JSON(http.StatusOK, response)
}

// Init sets up Server routes and templates on the main server engine
func (s *Server) UseUIEndpoints(ctx context.Context) {

	// API endpoints are handled separately and won't match this pattern
	// Admin/backend routes that need their own pages:
	// - /provider, /api-keys, /oauth, /routing, /system, /history etc.
	// All serve the same index.html, letting React Router handle the navigation

	// Exclude API routes from SPA catch-all by registering them first
	// The routes registered below (manager APIs, OAuth, usage, etc.) will take precedence

	// Engine-level routes first: the status line and notify hooks. The
	// modules mounted below, and which of them exist, are decided in
	// server_modules.go so this path and OpenAPI generation cannot drift.
	engineMods := s.engineModules(false)
	module.Mount(&module.Routes{Engine: s.engine}, engineMods...)

	manager := swagger.NewRouteManager(s.engine)
	rt := s.UseWebAPIEndpoints(manager)
	module.Mount(rt, s.apiModules(ctx, false)...)

	// Static files and templates - try embedded assets first, fallback to filesystem
	UseWebStaticEndpoints(s.engine)
}

// RuntimeAuditSink builds the AuditFunc the scenario runtime hands to
// plugins. Plugin actions (e.g. claude_code.interactive.start / .done /
// .error) land here as regular structured log lines — no separate audit
// trail is needed on top of the application log.
func RuntimeAuditSink() remotescenario.AuditFunc {
	return func(action string, fields map[string]any) {
		logrus.WithFields(logrus.Fields(fields)).WithField("action", action).Info(action)
	}
}

// newProjectsApp builds Projects with model access through this gateway.
// Schema generation gets an in-memory app (nothing on disk, never started);
// the running server keeps Projects' own database and project folders in
// dataDir (--projects-dir, default "<config dir>-project"), never inside the
// config dir, so a thread's working directory does not sit next to Tingly
// Box's credentials.
func newProjectsApp(cfg *config.Config, dataDir string, schema bool, chats *projectsmodule.ChatBridge, routes *statusline.Handler) *projectsapp.App {
	host := projectsmodule.NewHost(cfg).WithRoutePreview(routes)
	host.SetChatBridge(chats)
	opts := projectsapp.Options{Host: host}
	if !schema {
		if dataDir == "" {
			dataDir = options.DefaultProjectsDir(cfg.ConfigDir)
		}
		opts.DataDir = dataDir
	}
	app, err := projectsapp.New(opts)
	if err != nil {
		logrus.WithError(err).Warn("Failed to open Projects, its APIs will not be available")
		return nil
	}
	return app
}
