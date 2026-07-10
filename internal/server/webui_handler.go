package server

import (
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	assets "github.com/tingly-dev/tingly-box/internal"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/middleware"
	"github.com/tingly-dev/tingly-box/pkg/auth"
	"github.com/tingly-dev/tingly-box/pkg/obs"
	"github.com/tingly-dev/tingly-box/remote/audit"
	remotescenario "github.com/tingly-dev/tingly-box/remote/scenario"
)

func init() {
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".png", "image/png")
}

// WebDeps declares exactly what the WebUI Management API's control
// handlers need from the host server. It is populated and passed in once,
// from server.NewServer, after all of *Server's fields have been constructed.
//
// This grows as each subsequent migration step moves a file in
// (server_control.go, guardrails_handler.go, etc.) and wires up the
// fields/methods it actually touches on *Server today.
type WebDeps struct {
	// MemoryLogMW backs the HTTP request log API (GetLogs/GetLogStats/ClearLogs).
	MemoryLogMW *middleware.MultiModeMemoryLogMiddleware

	// MultiLogger backs the system log, model-request trace and action
	// history APIs.
	MultiLogger *obs.MultiLogger

	// Config backs token generation/retrieval (model token persistence).
	Config *config.Config

	// JWTManager issues the JWT-backed model tokens.
	JWTManager *auth.JWTManager
}

// WebHandler is the aggregate handler for the WebUI Management API's
// server-control surface (status/start/stop, logs, guardrails admin, token
// management, etc). Individual method files will be moved here in later
// steps and become methods on *WebHandler.
type WebHandler struct {
	deps WebDeps
}

// NewWebHandler constructs the WebUI control handler from its dependencies.
func NewWebHandler(deps WebDeps) *WebHandler {
	return &WebHandler{deps: deps}
}

func UseIndexHTML(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	f, err := assets.WebDistAssets.Open("web/dist/index.html")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func UseWebStaticEndpoints(engine *gin.Engine) {
	// Load templates and static files on the main engine - try embedded first
	log.Printf("Using embedded assets on main server")

	// Serve static assets from embedded filesystem
	st, _ := fs.Sub(assets.WebDistAssets, "web/dist/assets")
	engine.StaticFS("/assets", http.FS(st))

	// SPA catch-all - must be registered LAST
	// Serves index.html for all non-API frontend routes, letting React Router handle navigation
	// NoRoute handles unmatched paths including nested routes like /provider/settings/detail/123
	engine.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes - let them return 404s
		path := c.Request.URL.Path
		// Check if this looks like an API route
		if path == "" || strings.HasPrefix(path, "/api/v") || strings.HasPrefix(path, "/v") || strings.HasPrefix(path, "/openai") || strings.HasPrefix(path, "/anthropic") || strings.HasPrefix(path, "/tingly") {
			// This looks like an API route, return 404
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "API endpoint not found",
					"type":    "invalid_request_error",
					"code":    "not_found",
				},
			})
			c.Abort()
		}
	}, middleware.Gzip(), UseIndexHTML)
}

// RuntimeAuditSink adapts an audit.Logger into the AuditFunc the
// scenario runtime hands to plugins. Plugin actions (e.g.
// claude_code.interactive.start / .done / .error) land here as audit
// entries with structured details.
func RuntimeAuditSink(log *audit.Logger) remotescenario.AuditFunc {
	if log == nil {
		return nil
	}
	return func(action string, fields map[string]any) {
		details := map[string]interface{}{}
		for k, v := range fields {
			details[k] = v
		}
		log.Log(audit.Entry{
			Timestamp: time.Now(),
			Level:     audit.LevelInfo,
			Action:    action,
			Success:   true,
			Details:   details,
		})
	}
}
