package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	desksvc "github.com/tingly-dev/tingly-box/internal/desk"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/remote/control"
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
	engineMods, statusHandler := s.engineModules(false)
	module.Mount(&module.Routes{Engine: s.engine}, engineMods...)

	manager := swagger.NewRouteManager(s.engine)
	rt := s.UseWebAPIEndpoints(manager)
	module.Mount(rt, s.apiModules(ctx, false, statusHandler)...)

	// Static files and templates - try embedded assets first, fallback to filesystem
	UseWebStaticEndpoints(s.engine)
}

// deskSessionPoolConfig mirrors imbot's sessionPoolConfig
// (internal/server/module/imbot/manager.go) — same capacity/idle-timeout
// reasoning applies to a browser-driven Claude Code session as to a bot one.
var deskSessionPoolConfig = pool.Config{
	MaxSessions: 10,
	IdleTimeout: 10 * time.Minute,
}

// newDeskService builds the Desk service — a web front
// door onto the same remote/session + agentboot machinery @cc already
// drives, not a separate domain model (.design/desk.md). It builds
// its own control.Core, an independent in-memory session.Manager cache over
// the SAME session store @cc uses (sm.RemoteSessions()) — the sanctioned
// pattern every remote-host entry point follows (see remote/control.Core).
//
// Only the running server may call this: the service reconciles stored
// sessions on construction, so building one from schema generation would
// rewrite a live server's sessions. Returns nil if it cannot be built.
func newDeskService(sm *db.StoreManager, cfg *config.Config) *desksvc.Service {
	if sm == nil {
		return nil
	}
	maCore, err := control.NewCore(sm.RemoteSessions())
	if err != nil {
		logrus.WithError(err).Warn("Failed to create desk control core, desk APIs will not be available")
		return nil
	}
	return desksvc.NewService(desksvc.Config{
		Sessions: maCore.Session,
		Agent:    maCore.Agent,
		Routing:  tbclient.NewTBClient(cfg),
		Pool:     pool.New(deskSessionPoolConfig),
	})
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
