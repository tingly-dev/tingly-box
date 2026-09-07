package managedagent

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
)

// Handler adapts managedagent.Service to gin. It holds no state of its own.
type Handler struct {
	svc *managedagent.Service
	// ssePoll is how often the event stream re-reads the store while no new
	// events arrive. A later step replaces polling with a launcher-fed
	// broadcast; the wire format does not change.
	ssePoll time.Duration
}

// NewHandler builds a Handler over a Service.
func NewHandler(svc *managedagent.Service) *Handler {
	return &Handler{svc: svc, ssePoll: time.Second}
}

// sendError maps the domain's sentinel errors to HTTP statuses.
func sendError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, managedagent.ErrNotFound):
		apierr.Send(c, http.StatusNotFound, err, "not_found_error")
	case errors.Is(err, managedagent.ErrValidation):
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
	case errors.Is(err, managedagent.ErrConflict):
		apierr.Send(c, http.StatusConflict, err, "conflict_error")
	default:
		apierr.Send(c, http.StatusInternalServerError, err, "api_error")
	}
}

func requireParam(c *gin.Context, name string) (string, bool) {
	v := c.Param(name)
	if v == "" {
		apierr.Send(c, http.StatusBadRequest, fmt.Errorf("%s is required", name), "invalid_request_error")
		return "", false
	}
	return v, true
}

// ---------- sources ----------

func (h *Handler) ListSources(c *gin.Context) {
	list, err := h.svc.ListSources(c.Request.Context())
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, SourceListResponse{Sources: list})
}

func (h *Handler) CreateSource(c *gin.Context) {
	var req SourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	src, err := h.svc.CreateSource(c.Request.Context(), sourceInput(req))
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusCreated, src)
}

func (h *Handler) GetSource(c *gin.Context) {
	id, ok := requireParam(c, "source_id")
	if !ok {
		return
	}
	src, err := h.svc.GetSource(c.Request.Context(), id)
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, src)
}

func (h *Handler) UpdateSource(c *gin.Context) {
	id, ok := requireParam(c, "source_id")
	if !ok {
		return
	}
	var req SourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	src, err := h.svc.UpdateSource(c.Request.Context(), id, sourceInput(req))
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, src)
}

func (h *Handler) DeleteSource(c *gin.Context) {
	id, ok := requireParam(c, "source_id")
	if !ok {
		return
	}
	if err := h.svc.DeleteSource(c.Request.Context(), id); err != nil {
		sendError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func sourceInput(r SourceRequest) managedagent.SourceInput {
	return managedagent.SourceInput{Name: r.Name, URL: r.URL, DefaultBranch: r.DefaultBranch, CredentialID: r.CredentialID}
}

// ---------- environments ----------

func (h *Handler) ListEnvironments(c *gin.Context) {
	list, err := h.svc.ListEnvironments(c.Request.Context())
	if err != nil {
		sendError(c, err)
		return
	}
	supported := make([]managedagent.Runtime, 0, len(managedagent.SupportedRuntimes))
	for rt, ok := range managedagent.SupportedRuntimes {
		if ok {
			supported = append(supported, rt)
		}
	}
	c.JSON(http.StatusOK, EnvironmentListResponse{Environments: list, SupportedRuntimes: supported, PermissionModes: managedagent.PermissionModes})
}

func (h *Handler) CreateEnvironment(c *gin.Context) {
	var req EnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	env, err := h.svc.CreateEnvironment(c.Request.Context(), environmentInput(req))
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusCreated, env)
}

func (h *Handler) GetEnvironment(c *gin.Context) {
	id, ok := requireParam(c, "environment_id")
	if !ok {
		return
	}
	env, err := h.svc.GetEnvironment(c.Request.Context(), id)
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, env)
}

func (h *Handler) UpdateEnvironment(c *gin.Context) {
	id, ok := requireParam(c, "environment_id")
	if !ok {
		return
	}
	var req EnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	env, err := h.svc.UpdateEnvironment(c.Request.Context(), id, environmentInput(req))
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, env)
}

func (h *Handler) DeleteEnvironment(c *gin.Context) {
	id, ok := requireParam(c, "environment_id")
	if !ok {
		return
	}
	if err := h.svc.DeleteEnvironment(c.Request.Context(), id); err != nil {
		sendError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func environmentInput(r EnvironmentRequest) managedagent.EnvironmentInput {
	return managedagent.EnvironmentInput{
		Name: r.Name, Runtime: r.Runtime, Image: r.Image, SetupScript: r.SetupScript,
		Env: r.Env, SecretRefs: r.SecretRefs, Network: r.Network, Resources: r.Resources,
		CCProfile: r.CCProfile, PermissionMode: r.PermissionMode,
	}
}

// ---------- sessions ----------

func (h *Handler) ListSessions(c *gin.Context) {
	var q SessionListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	list, err := h.svc.ListSessions(c.Request.Context(), managedagent.SessionFilter{
		WorkspaceID: q.WorkspaceID,
		Status:      managedagent.SessionStatus(q.Status),
		Active:      q.Active,
		Limit:       q.Limit,
	})
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, SessionListResponse{Sessions: h.listItems(c, list)})
}

// listItems resolves each session's workspace and source once per distinct
// id. Lookups are best-effort: a row whose workspace is gone still lists.
func (h *Handler) listItems(c *gin.Context, list []managedagent.Session) []SessionListItem {
	ctx := c.Request.Context()
	workspaces := map[string]*managedagent.Workspace{}
	sources := map[string]*managedagent.Source{}
	items := make([]SessionListItem, 0, len(list))
	for i := range list {
		item := SessionListItem{Session: list[i]}
		ws, seen := workspaces[list[i].WorkspaceID]
		if !seen {
			ws, _ = h.svc.GetWorkspace(ctx, list[i].WorkspaceID)
			workspaces[list[i].WorkspaceID] = ws
		}
		if ws != nil {
			item.Branch = ws.Branch
			src, seen := sources[ws.SourceID]
			if !seen {
				src, _ = h.svc.GetSource(ctx, ws.SourceID)
				sources[ws.SourceID] = src
			}
			if src != nil {
				item.Source = &SourceRef{ID: src.ID, Name: src.Name}
			}
		}
		items = append(items, item)
	}
	return items
}

func (h *Handler) CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	sess, err := h.svc.CreateSession(c.Request.Context(), managedagent.CreateSessionInput{
		SourceID: req.SourceID, EnvironmentID: req.EnvironmentID, WorkspaceID: req.WorkspaceID,
		BaseRef: req.BaseRef, Prompt: req.Prompt, Title: req.Title, CreatedBy: "web",
		PermissionMode: req.PermissionMode,
	})
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusCreated, h.detail(c, sess))
}

func (h *Handler) GetSession(c *gin.Context) {
	id, ok := requireParam(c, "session_id")
	if !ok {
		return
	}
	sess, err := h.svc.GetSession(c.Request.Context(), id)
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.detail(c, sess))
}

// detail attaches the workspace; a workspace lookup failure degrades to a
// session-only response rather than hiding the session.
func (h *Handler) detail(c *gin.Context, sess *managedagent.Session) SessionDetail {
	d := SessionDetail{Session: *sess}
	if ws, err := h.svc.GetWorkspace(c.Request.Context(), sess.WorkspaceID); err == nil {
		d.Workspace = ws
	}
	return d
}

func (h *Handler) SendMessage(c *gin.Context) {
	id, ok := requireParam(c, "session_id")
	if !ok {
		return
	}
	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	if err := h.svc.SendMessage(c.Request.Context(), id, req.Text); err != nil {
		sendError(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}

func (h *Handler) Respond(c *gin.Context) {
	id, ok := requireParam(c, "session_id")
	if !ok {
		return
	}
	var req RespondRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	err := h.svc.Respond(c.Request.Context(), id, managedagent.Response{
		RequestID: req.RequestID, Approved: req.Approved, Answer: req.Answer,
	})
	if err != nil {
		sendError(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}

func (h *Handler) SetPermissionMode(c *gin.Context) {
	id, ok := requireParam(c, "session_id")
	if !ok {
		return
	}
	var req SetPermissionModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	sess, err := h.svc.SetPermissionMode(c.Request.Context(), id, req.PermissionMode)
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.detail(c, sess))
}

func (h *Handler) Interrupt(c *gin.Context) {
	id, ok := requireParam(c, "session_id")
	if !ok {
		return
	}
	if err := h.svc.Interrupt(c.Request.Context(), id); err != nil {
		sendError(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}

func (h *Handler) Archive(c *gin.Context) {
	id, ok := requireParam(c, "session_id")
	if !ok {
		return
	}
	sess, err := h.svc.Archive(c.Request.Context(), id)
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.detail(c, sess))
}

// ---------- workspaces ----------

func (h *Handler) ListWorkspaces(c *gin.Context) {
	list, err := h.svc.ListWorkspaces(c.Request.Context(), managedagent.WorkspaceFilter{
		SourceID:      c.Query("source_id"),
		EnvironmentID: c.Query("environment_id"),
		State:         managedagent.WorkspaceState(c.Query("state")),
	})
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, WorkspaceListResponse{Workspaces: list})
}

func (h *Handler) ReclaimWorkspace(c *gin.Context) {
	id, ok := requireParam(c, "workspace_id")
	if !ok {
		return
	}
	ws, err := h.svc.ReclaimWorkspace(c.Request.Context(), id)
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (h *Handler) Diff(c *gin.Context) {
	id, ok := requireParam(c, "session_id")
	if !ok {
		return
	}
	d, err := h.svc.Diff(c.Request.Context(), id)
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) Push(c *gin.Context) {
	id, ok := requireParam(c, "session_id")
	if !ok {
		return
	}
	sess, err := h.svc.Push(c.Request.Context(), id)
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.detail(c, sess))
}

// Events serves a session's log. With `Accept: text/event-stream` it keeps
// the connection open and pushes new events as they are appended; otherwise
// it returns one JSON page. Same data, same `after` cursor, so a client can
// fall back from streaming to polling without a second endpoint.
func (h *Handler) Events(c *gin.Context) {
	id, ok := requireParam(c, "session_id")
	if !ok {
		return
	}
	var q EventListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	if strings.Contains(c.GetHeader("Accept"), "text/event-stream") {
		h.streamEvents(c, id, q.After)
		return
	}
	events, err := h.svc.ListEvents(c.Request.Context(), id, q.After, q.Limit)
	if err != nil {
		sendError(c, err)
		return
	}
	next := q.After
	if n := len(events); n > 0 {
		next = events[n-1].Seq
	}
	c.JSON(http.StatusOK, EventListResponse{Events: events, Next: next})
}

func (h *Handler) streamEvents(c *gin.Context, sessionID string, after int64) {
	// Validate before committing to a stream so a bad id is still a 404.
	if _, err := h.svc.GetSession(c.Request.Context(), sessionID); err != nil {
		sendError(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	ctx := c.Request.Context()
	ticker := time.NewTicker(h.ssePoll)
	defer ticker.Stop()
	cursor := after
	// A hand-rolled loop rather than gin's c.Stream: that helper requires
	// http.CloseNotifier, which httptest and some proxies do not provide;
	// the request context already reports disconnects.
	for {
		events, err := h.svc.ListEvents(ctx, sessionID, cursor, 0)
		if err != nil {
			c.SSEvent("error", gin.H{"message": err.Error()})
			c.Writer.Flush()
			return
		}
		for i := range events {
			c.SSEvent("event", events[i])
			cursor = events[i].Seq
		}
		if len(events) == 0 {
			c.SSEvent("ping", cursor)
		}
		c.Writer.Flush()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
