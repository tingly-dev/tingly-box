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

// ---------- host folders ----------

// sendError maps the domain's sentinel errors to HTTP statuses. The
// sentinel's own text (": validation", ": forbidden", …) is dropped from the
// message: the status already says it, and people read these in the UI.
func sendError(c *gin.Context, err error) {
	for _, sentinel := range []error{managedagent.ErrNotFound, managedagent.ErrValidation, managedagent.ErrConflict, managedagent.ErrForbidden} {
		if errors.Is(err, sentinel) {
			if msg := strings.TrimSuffix(err.Error(), ": "+sentinel.Error()); msg != err.Error() {
				err = &httpErr{msg: msg, cause: err}
			}
			break
		}
	}
	switch {
	case errors.Is(err, managedagent.ErrNotFound):
		apierr.Send(c, http.StatusNotFound, err, "not_found_error")
	case errors.Is(err, managedagent.ErrValidation):
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
	case errors.Is(err, managedagent.ErrConflict):
		apierr.Send(c, http.StatusConflict, err, "conflict_error")
	case errors.Is(err, managedagent.ErrForbidden):
		apierr.Send(c, http.StatusForbidden, err, "permission_error")
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

func (h *Handler) BrowseDirs(c *gin.Context) {
	listing, err := h.svc.Browse(c.Request.Context(), c.Query("path"))
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, listing)
}

// ---------- folders ----------

func (h *Handler) ListFolders(c *gin.Context) {
	folders, err := h.svc.ListFolders(c.Request.Context())
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, FolderListResponse{Folders: folders})
}

func (h *Handler) AddFolder(c *gin.Context) {
	var req AddFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	f, err := h.svc.AddFolder(c.Request.Context(), req.Path)
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusCreated, f)
}

func (h *Handler) RemoveFolder(c *gin.Context) {
	id, ok := requireParam(c, "folder_id")
	if !ok {
		return
	}
	if err := h.svc.RemoveFolder(c.Request.Context(), id); err != nil {
		sendError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) PermissionModes(c *gin.Context) {
	c.JSON(http.StatusOK, PermissionModeListResponse{PermissionModes: managedagent.PermissionModes})
}

// ---------- sessions ----------

func (h *Handler) ListSessions(c *gin.Context) {
	var q SessionListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	list, err := h.svc.ListSessions(c.Request.Context(), managedagent.SessionFilter{
		FolderID: q.FolderID,
		Status:   managedagent.SessionStatus(q.Status),
		Active:   q.Active,
		Limit:    q.Limit,
	})
	if err != nil {
		sendError(c, err)
		return
	}
	c.JSON(http.StatusOK, SessionListResponse{Sessions: h.listItems(c, list)})
}

// listItems resolves each session's folder once per distinct id. Lookups are
// best-effort: a row whose folder was removed still lists.
func (h *Handler) listItems(c *gin.Context, list []managedagent.Session) []SessionListItem {
	ctx := c.Request.Context()
	folders := map[string]*managedagent.Folder{}
	items := make([]SessionListItem, 0, len(list))
	for i := range list {
		item := SessionListItem{Session: list[i]}
		f, seen := folders[list[i].FolderID]
		if !seen {
			f, _ = h.svc.GetFolder(ctx, list[i].FolderID)
			folders[list[i].FolderID] = f
		}
		if f != nil {
			item.Folder = &FolderRef{ID: f.ID, Name: f.Name, Path: f.Path}
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
		FolderID: req.FolderID, Path: req.Path, Prompt: req.Prompt, Title: req.Title,
		PermissionMode: req.PermissionMode, CreatedBy: "web",
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

// detail attaches the folder; a lookup failure degrades to a session-only
// response rather than hiding the session.
func (h *Handler) detail(c *gin.Context, sess *managedagent.Session) SessionDetail {
	d := SessionDetail{Session: *sess}
	if f, err := h.svc.GetFolder(c.Request.Context(), sess.FolderID); err == nil {
		d.Folder = f
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

// httpErr carries a cleaned message while keeping errors.Is on the cause.
type httpErr struct {
	msg   string
	cause error
}

func (e *httpErr) Error() string { return e.msg }
func (e *httpErr) Unwrap() error { return e.cause }
