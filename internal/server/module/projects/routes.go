package projects

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
)

// Handler mounts Projects' routes (all under /api/v1/projects) behind user
// auth and the feature flag.
type Handler struct {
	app *projectsapp.App
	// enabled is the feature flag. While it is off every route answers 404:
	// Projects runs Claude Code on this machine, so a disabled feature must
	// be unreachable, not just hidden from the nav.
	enabled func() bool
	// start runs when a request first finds the flag on; it starts the
	// Projects runtime (once).
	start func()
}

var _ module.Module = (*Handler)(nil)

func NewHandler(app *projectsapp.App, enabled func() bool, start func()) *Handler {
	return &Handler{app: app, enabled: enabled, start: start}
}

// RegisterRoutes gives Projects a /api/v1 group of its own, so the gate
// applies to its routes and nothing else.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	group := rt.Manager.NewGroup("api", "v1", "")
	if rt.UserAuth != nil {
		group.Router.Use(rt.UserAuth)
	}
	group.Router.Use(h.gate)
	h.app.Register(group)
}

func (h *Handler) gate(c *gin.Context) {
	if h.enabled == nil || !h.enabled() {
		apierr.Message(c, http.StatusNotFound, "projects is disabled")
		c.Abort()
		return
	}
	if h.start != nil {
		h.start()
	}
	c.Next()
}
