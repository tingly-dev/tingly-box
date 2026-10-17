// Package session serves folder sessions: Claude Code working in a plain
// folder, listed under the working folders in the sidebar. They run on the
// thread runtime without a project (see .design/projects-desk-absorption.md).
package session

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/swagger"
)

// WorkdirsResponse lists the working folders.
type WorkdirsResponse struct {
	Workdirs []rt.Workdir `json:"workdirs"`
}

// AddWorkdirRequest keeps a folder in the sidebar.
type AddWorkdirRequest struct {
	Path string `json:"path" binding:"required"`
}

// SessionsResponse lists sessions, most recently updated first.
type SessionsResponse struct {
	Sessions []db.Thread `json:"sessions"`
}

// SessionCreateRequest starts a session in a folder.
type SessionCreateRequest struct {
	Folder string `json:"folder" binding:"required"`
	// Content is the first message.
	Content string `json:"content" binding:"required"`
	Title   string `json:"title,omitempty"`
	// Model is empty for the host's default.
	Model string `json:"model,omitempty"`
	// PermissionMode is empty to inherit the default (acceptEdits).
	PermissionMode string `json:"permission_mode,omitempty"`
	// Profile is the host's launch profile; empty for its default. Only
	// hosts with profiles (Tingly Box) accept one.
	Profile string `json:"profile,omitempty"`
}

// SessionDetailResponse is a session with its transcript.
type SessionDetailResponse struct {
	Session         db.Thread           `json:"session"`
	Events          []db.ThreadEvent    `json:"events"`
	Running         bool                `json:"running"`
	BackgroundTasks []rt.BackgroundTask `json:"background_tasks"`
}

// SendRequest is a message to the session.
type SendRequest struct {
	Content string `json:"content" binding:"required"`
}

// SessionRespondRequest answers a pending approval or question.
type SessionRespondRequest struct {
	RequestID string `json:"request_id" binding:"required"`
	Allow     bool   `json:"allow"`
	// Scope "thread" also allows later calls of the same tool in this session.
	Scope  string `json:"scope,omitempty" enum:"once,thread"`
	Answer string `json:"answer,omitempty"`
}

// PermissionModeRequest sets the mode of the session's next turn.
type PermissionModeRequest struct {
	PermissionMode string `json:"permission_mode"`
}

// ProfileRequest sets the launch profile of the session's next turn.
type ProfileRequest struct {
	Profile string `json:"profile"`
}

// ModelRequest sets the model of the session's next turn ("" for the default).
type ModelRequest struct {
	Model string `json:"model"`
}

// SessionHandoffResponse is the command that continues the session in a terminal.
type SessionHandoffResponse struct {
	Command string `json:"command"`
}

// SessionPermissionModesResponse lists the selectable modes.
type SessionPermissionModesResponse struct {
	Modes []string `json:"modes"`
}

// SessionTaskOutputResponse is the end of a background task's output.
type SessionTaskOutputResponse struct {
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	Size      int64  `json:"size"`
}

// Handler is the folder-session HTTP module.
type Handler struct {
	svc *rt.Service
}

// NewHandler builds the module.
func NewHandler(svc *rt.Service) *Handler { return &Handler{svc: svc} }

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts the endpoints.
func (h *Handler) RegisterRoutes(r *module.Routes) {
	api := r.API
	tag := swagger.WithTags("sessions")
	api.GET("/projects/workdirs", h.workdirs, tag,
		swagger.WithDescription("Working folders: the saved ones and every folder that has a session, with what waits there"),
		swagger.WithResponseModel(WorkdirsResponse{}))
	api.POST("/projects/workdirs", h.addWorkdir, tag,
		swagger.WithDescription("Keep an existing folder in the sidebar"),
		swagger.WithRequestModel(AddWorkdirRequest{}),
		swagger.WithResponseModel(rt.Workdir{}))
	api.DELETE("/projects/workdirs", h.removeWorkdir, tag,
		swagger.WithDescription("Drop a folder from the sidebar; its sessions stay"),
		swagger.WithQuery("path", "string", "The folder"))
	api.GET("/projects/permission-modes", h.permissionModes, tag,
		swagger.WithDescription("The Claude Code permission modes a session can run in"),
		swagger.WithResponseModel(SessionPermissionModesResponse{}))
	api.GET("/projects/sessions", h.list, tag,
		swagger.WithDescription("Folder sessions, most recently updated first"),
		swagger.WithQuery("folder", "string", "Only this folder's sessions"),
		swagger.WithResponseModel(SessionsResponse{}))
	api.POST("/projects/sessions", h.create, tag,
		swagger.WithDescription("Start a session in a folder"),
		swagger.WithRequestModel(SessionCreateRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.GET("/projects/sessions/events", h.events, tag,
		swagger.WithDescription("Server-sent events for folder sessions (session and transcript changes)"))
	api.GET("/projects/sessions/:sid", h.get, tag,
		swagger.WithDescription("A session with its transcript"),
		swagger.WithResponseModel(SessionDetailResponse{}))
	api.POST("/projects/sessions/:sid/messages", h.send, tag,
		swagger.WithDescription("Send a message (runs after the current turn)"),
		swagger.WithRequestModel(SendRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/projects/sessions/:sid/respond", h.respond, tag,
		swagger.WithDescription("Answer a permission request or question the session is waiting on"),
		swagger.WithRequestModel(SessionRespondRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/projects/sessions/:sid/stop", h.stop, tag,
		swagger.WithDescription("Interrupt the running turn; the process and its background tasks survive"),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/projects/sessions/:sid/archive", h.archive, tag,
		swagger.WithDescription("Archive the session (it stays readable and can be reopened)"),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/projects/sessions/:sid/reopen", h.reopen, tag,
		swagger.WithDescription("Reopen an archived session"),
		swagger.WithResponseModel(db.Thread{}))
	api.PUT("/projects/sessions/:sid/permission-mode", h.setPermissionMode, tag,
		swagger.WithDescription("Set the permission mode of the session's next turn"),
		swagger.WithRequestModel(PermissionModeRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.PUT("/projects/sessions/:sid/profile", h.setProfile, tag,
		swagger.WithDescription("Set the launch profile of the session's next turn (hosts with profiles only)"),
		swagger.WithRequestModel(ProfileRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.PUT("/projects/sessions/:sid/model", h.setModel, tag,
		swagger.WithDescription("Set the model of the session's next turn"),
		swagger.WithRequestModel(ModelRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/projects/sessions/:sid/handoff", h.handoff, tag,
		swagger.WithDescription("Release the session to a terminal: closes its process and returns the command that resumes it"),
		swagger.WithResponseModel(SessionHandoffResponse{}))
	api.POST("/projects/sessions/:sid/tasks/:task_id/stop", h.stopTask, tag,
		swagger.WithDescription("Stop one background task"))
	api.GET("/projects/sessions/:sid/tasks/:task_id/output", h.taskOutput, tag,
		swagger.WithDescription("The end of a background task's output"),
		swagger.WithQuery("tail", "int", "Bytes from the end (default 8192)"),
		swagger.WithResponseModel(SessionTaskOutputResponse{}))
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		apierr.Store(c, err, "session")
	case errors.Is(err, rt.ErrResolved), errors.Is(err, rt.ErrNoPrompt), errors.Is(err, rt.ErrTaskNotRunning), errors.Is(err, rt.ErrBusy):
		apierr.Message(c, http.StatusConflict, err.Error())
	case errors.Is(err, rt.ErrTaskOutputUnset), errors.Is(err, os.ErrNotExist):
		// The output file may be gone; never echo its path.
		apierr.Message(c, http.StatusNotFound, rt.ErrTaskOutputUnset.Error())
	default:
		apierr.BadRequest(c, err.Error())
	}
}

func (h *Handler) session(c *gin.Context) (*db.Thread, bool) {
	t, err := h.svc.Store.GetSession(c.Param("sid"))
	if err != nil {
		fail(c, err)
		return nil, false
	}
	return t, true
}

func (h *Handler) workdirs(c *gin.Context) {
	w, err := h.svc.Workdirs()
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, WorkdirsResponse{Workdirs: w})
}

func (h *Handler) addWorkdir(c *gin.Context) {
	var req AddWorkdirRequest
	if !apierr.Bind(c, &req) {
		return
	}
	path, err := rt.CleanFolder(req.Path)
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.Store.SaveWorkdir(path); err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, rt.Workdir{Path: path, Saved: true})
}

func (h *Handler) removeWorkdir(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		apierr.BadRequest(c, "path is required")
		return
	}
	if err := h.svc.Store.RemoveWorkdir(path); err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) permissionModes(c *gin.Context) {
	c.JSON(http.StatusOK, SessionPermissionModesResponse{Modes: rt.PermissionModes})
}

func (h *Handler) list(c *gin.Context) {
	list, err := h.svc.Store.ListSessions(strings.TrimSpace(c.Query("folder")))
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []db.Thread{}
	}
	c.JSON(http.StatusOK, SessionsResponse{Sessions: list})
}

func (h *Handler) create(c *gin.Context) {
	var req SessionCreateRequest
	if !apierr.Bind(c, &req) {
		return
	}
	t, err := h.svc.Threads.StartSession(rt.SessionSpec{
		Folder: req.Folder, Task: req.Content, Title: req.Title, Model: req.Model, PermissionMode: req.PermissionMode, Profile: req.Profile,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) get(c *gin.Context) {
	t, ok := h.session(c)
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
	c.JSON(http.StatusOK, SessionDetailResponse{
		Session: *t, Events: events, Running: h.svc.Threads.Running(t.ID), BackgroundTasks: h.svc.Threads.BackgroundTasks(t.ID),
	})
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
	t, ok := h.session(c)
	if !ok {
		return
	}
	out, err := h.svc.Threads.Send(c.Request.Context(), t.ID, req.Content, "user")
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) respond(c *gin.Context) {
	var req SessionRespondRequest
	if !apierr.Bind(c, &req) {
		return
	}
	t, ok := h.session(c)
	if !ok {
		return
	}
	if err := h.svc.Threads.Respond(t.ID, req.RequestID, rt.Answer{Allow: req.Allow, Scope: req.Scope, Answer: req.Answer}); err != nil {
		fail(c, err)
		return
	}
	h.reply(c, t.ID)
}

func (h *Handler) stop(c *gin.Context) {
	t, ok := h.session(c)
	if !ok {
		return
	}
	h.svc.Threads.Stop(t.ID)
	h.reply(c, t.ID)
}

func (h *Handler) archive(c *gin.Context) {
	t, ok := h.session(c)
	if !ok {
		return
	}
	out, err := h.svc.Threads.Resolve(t.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) reopen(c *gin.Context) {
	t, ok := h.session(c)
	if !ok {
		return
	}
	out, err := h.svc.Threads.Reopen(t.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) setPermissionMode(c *gin.Context) {
	var req PermissionModeRequest
	if !apierr.Bind(c, &req) {
		return
	}
	t, ok := h.session(c)
	if !ok {
		return
	}
	out, err := h.svc.Threads.SetPermissionMode(t.ID, req.PermissionMode)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) setProfile(c *gin.Context) {
	var req ProfileRequest
	if !apierr.Bind(c, &req) {
		return
	}
	out, err := h.svc.Threads.SetProfile(c.Request.Context(), c.Param("sid"), req.Profile)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) setModel(c *gin.Context) {
	var req ModelRequest
	if !apierr.Bind(c, &req) {
		return
	}
	out, err := h.svc.Threads.SetModel(c.Request.Context(), c.Param("sid"), req.Model)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) handoff(c *gin.Context) {
	cmd, err := h.svc.Threads.Handoff(c.Param("sid"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, SessionHandoffResponse{Command: cmd})
}

func (h *Handler) stopTask(c *gin.Context) {
	t, ok := h.session(c)
	if !ok {
		return
	}
	if err := h.svc.Threads.StopTask(c.Request.Context(), t.ID, c.Param("task_id")); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) taskOutput(c *gin.Context) {
	t, ok := h.session(c)
	if !ok {
		return
	}
	tail := 0
	if v := c.Query("tail"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			apierr.BadRequest(c, "tail must be a non-negative integer")
			return
		}
		tail = n
	}
	out, err := h.svc.Threads.TaskOutput(t.ID, c.Param("task_id"), tail)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, SessionTaskOutputResponse{Content: out.Content, Truncated: out.Truncated, Size: out.Size})
}

// reply answers with the session as it is now.
func (h *Handler) reply(c *gin.Context, id string) {
	t, err := h.svc.Store.GetThread(id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) events(c *gin.Context) {
	ch, cancel := h.svc.Hub.Subscribe("")
	defer cancel()
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	w.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case b := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", b)
			w.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			w.Flush()
		}
	}
}
