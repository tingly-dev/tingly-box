// Package thread serves one thread: its transcript, messages to it,
// approvals, and review actions (diff, merge, resolve).
package thread

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/swagger"
)

// ThreadDetailResponse is a thread with its transcript.
type ThreadDetailResponse struct {
	Thread  db.Thread        `json:"thread"`
	Events  []db.ThreadEvent `json:"events"`
	Running bool             `json:"running"`
}

// SendRequest is a message to the thread.
type SendRequest struct {
	Content string `json:"content" binding:"required"`
}

// ThreadRespondRequest answers a pending approval or question.
type ThreadRespondRequest struct {
	RequestID string `json:"request_id" binding:"required"`
	Allow     bool   `json:"allow"`
	// Scope "thread" also allows later calls of the same tool in this thread.
	Scope  string `json:"scope,omitempty" enum:"once,thread"`
	Answer string `json:"answer,omitempty"`
}

// MergeBlockedResponse explains why a merge was not done.
type MergeBlockedResponse struct {
	Error string `json:"error"`
	// Command does the merge by hand.
	Command string `json:"command"`
}

// Handler is the thread HTTP module.
type Handler struct {
	svc *rt.Service
}

// NewHandler builds the module.
func NewHandler(svc *rt.Service) *Handler { return &Handler{svc: svc} }

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts thread endpoints.
func (h *Handler) RegisterRoutes(r *module.Routes) {
	api := r.API
	tag := swagger.WithTags("threads")
	api.GET("/projects/:id/threads/:tid", h.get, tag,
		swagger.WithDescription("A thread with its transcript"),
		swagger.WithResponseModel(ThreadDetailResponse{}))
	api.POST("/projects/:id/threads/:tid/messages", h.send, tag,
		swagger.WithDescription("Send a message straight to the thread (runs after its current turn)"),
		swagger.WithRequestModel(SendRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/projects/:id/threads/:tid/respond", h.respond, tag,
		swagger.WithDescription("Answer a permission request or question the thread is waiting on"),
		swagger.WithRequestModel(ThreadRespondRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/projects/:id/threads/:tid/stop", h.stop, tag,
		swagger.WithDescription("Interrupt the thread's running turn"),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/projects/:id/threads/:tid/resolve", h.resolve, tag,
		swagger.WithDescription("Mark the thread resolved"),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/projects/:id/threads/:tid/reopen", h.reopen, tag,
		swagger.WithDescription("Reopen a resolved thread"),
		swagger.WithResponseModel(db.Thread{}))
	api.GET("/projects/:id/threads/:tid/diff", h.diff, tag,
		swagger.WithDescription("The thread's changes against its base branch"),
		swagger.WithResponseModel(rt.Diff{}))
	api.POST("/projects/:id/threads/:tid/merge", h.merge, tag,
		swagger.WithDescription("Merge the thread's branch into its base branch in the main checkout. 409 with a command to run by hand when the checkout is not ready."),
		swagger.WithResponseModel(db.Thread{}),
		swagger.WithErrorResponses(swagger.ErrorResponseConfig{Code: http.StatusConflict, Message: "merge blocked", Model: MergeBlockedResponse{}}))
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		apierr.Store(c, err, "thread")
	case errors.Is(err, rt.ErrPaused), errors.Is(err, rt.ErrArchived), errors.Is(err, rt.ErrResolved), errors.Is(err, rt.ErrNoPrompt):
		apierr.Message(c, http.StatusConflict, err.Error())
	default:
		apierr.BadRequest(c, err.Error())
	}
}

// thread loads the thread named in the path. A thread asked for under
// another project is reported as not found, so a URL can never reach a
// thread through a project it does not belong to.
func (h *Handler) thread(c *gin.Context) (*db.Thread, bool) {
	t, err := h.svc.Store.GetThread(c.Param("tid"))
	if err == nil && t.ProjectID != c.Param("id") {
		err = db.ErrNotFound
	}
	if err != nil {
		fail(c, err)
		return nil, false
	}
	return t, true
}

func (h *Handler) get(c *gin.Context) {
	t, ok := h.thread(c)
	if !ok {
		return
	}
	events, err := h.svc.Store.ListThreadEvents(t.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if events == nil {
		events = []db.ThreadEvent{}
	}
	c.JSON(http.StatusOK, ThreadDetailResponse{Thread: *t, Events: events, Running: h.svc.Threads.Running(t.ID)})
}

func (h *Handler) send(c *gin.Context) {
	var req SendRequest
	if !apierr.Bind(c, &req) {
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		apierr.BadRequest(c, "message is empty")
		return
	}
	cur, ok := h.thread(c)
	if !ok {
		return
	}
	t, err := h.svc.Threads.Send(c.Request.Context(), cur.ID, req.Content, "user")
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) respond(c *gin.Context) {
	var req ThreadRespondRequest
	if !apierr.Bind(c, &req) {
		return
	}
	cur, ok := h.thread(c)
	if !ok {
		return
	}
	if err := h.svc.Threads.Respond(cur.ID, req.RequestID, rt.Answer{Allow: req.Allow, Scope: req.Scope, Answer: req.Answer}); err != nil {
		fail(c, err)
		return
	}
	h.reply(c, cur.ID)
}

func (h *Handler) stop(c *gin.Context) {
	cur, ok := h.thread(c)
	if !ok {
		return
	}
	h.svc.Threads.Stop(cur.ID)
	h.reply(c, cur.ID)
}

// reply answers with the thread as it is now.
func (h *Handler) reply(c *gin.Context, id string) {
	t, err := h.svc.Store.GetThread(id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) resolve(c *gin.Context) {
	cur, ok := h.thread(c)
	if !ok {
		return
	}
	t, err := h.svc.Threads.Resolve(cur.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) reopen(c *gin.Context) {
	cur, ok := h.thread(c)
	if !ok {
		return
	}
	t, err := h.svc.Threads.Reopen(cur.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) diff(c *gin.Context) {
	cur, ok := h.thread(c)
	if !ok {
		return
	}
	d, err := h.svc.Threads.Diff(c.Request.Context(), cur.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) merge(c *gin.Context) {
	cur, ok := h.thread(c)
	if !ok {
		return
	}
	t, err := h.svc.Threads.Merge(c.Request.Context(), cur.ID)
	var blocked *rt.MergeBlockedError
	if errors.As(err, &blocked) {
		c.AbortWithStatusJSON(http.StatusConflict, MergeBlockedResponse{Error: blocked.Reason, Command: blocked.Command})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}
