// Package thread serves threads, by thread id whatever their project's type:
// a thread's state and events, messages to it (the one Send), approvals,
// settings, background tasks, attachments and review actions. Listing and
// creating threads is project-scoped (/projects/:id/threads).
package thread

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
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

// ThreadsResponse lists a project's threads, most recently updated first. The
// main thread is the project's conversation and is not in it.
type ThreadsResponse struct {
	Threads []db.Thread `json:"threads"`
}

// CreateThreadRequest starts a thread in a folder project. A project's own
// threads are started by its main thread.
type CreateThreadRequest struct {
	// Content is the first message; it may be empty when attachments are sent.
	Content string `json:"content"`
	// Attachments are paths returned by the project's attachments upload.
	Attachments []string `json:"attachments,omitempty"`
	Title       string   `json:"title,omitempty"`
	// Model is empty for the host's default.
	Model string `json:"model,omitempty"`
	// PermissionMode is empty to inherit the default (acceptEdits).
	PermissionMode string `json:"permission_mode,omitempty"`
	// Profile is the host's launch profile; empty for its default. Only
	// hosts with profiles (Tingly Box) accept one.
	Profile string `json:"profile,omitempty"`
}

// ThreadDetailResponse is a thread as it is now.
type ThreadDetailResponse struct {
	Thread          db.Thread           `json:"thread"`
	Running         bool                `json:"running"`
	BackgroundTasks []rt.BackgroundTask `json:"background_tasks"`
}

// EventsResponse is a thread's events, oldest first. For a main thread they
// are the project conversation.
type EventsResponse struct {
	Events []db.ThreadEvent `json:"events"`
	// More says older events were left out (with limit).
	More bool `json:"more"`
}

// SubagentsResponse lists a thread's subagents.
type SubagentsResponse struct {
	Subagents []db.Subagent `json:"subagents"`
}

// SendRequest is a message to the thread.
type SendRequest struct {
	// Content may be empty when attachments are sent.
	Content string `json:"content"`
	// Attachments are paths returned by the thread's attachments upload.
	Attachments []string `json:"attachments,omitempty"`
	// Mode is what a message does while a turn runs: "send" (the default)
	// adds it to the turn, "queue" holds it until the turn ends and
	// "stop_and_send" stops the turn first.
	Mode string `json:"mode,omitempty" enums:"send,queue,stop_and_send"`
}

// StartSuggestionsRequest starts proposed threads; empty indexes start all.
type StartSuggestionsRequest struct {
	Indexes []int `json:"indexes,omitempty"`
}

// ThreadRespondRequest answers a pending approval or question.
type ThreadRespondRequest struct {
	RequestID string `json:"request_id" binding:"required"`
	Allow     bool   `json:"allow"`
	// Scope "thread" also allows later calls of the same tool in this thread.
	Scope  string `json:"scope,omitempty" enum:"once,thread"`
	Answer string `json:"answer,omitempty"`
	// Answers answers a question with several parts: question text to
	// the chosen option's label (several joined by ", ") or typed words.
	Answers map[string]string `json:"answers,omitempty"`
}

// AutoAnswerRequest sets when the thread's questions with a suggested
// answer are answered without the user; empty is the default (delay for a
// project thread, off for a folder thread).
type AutoAnswerRequest struct {
	AutoAnswer string `json:"auto_answer" enum:",off,delay,now"`
}

// PermissionModeRequest sets the mode of the thread's next turn.
type PermissionModeRequest struct {
	PermissionMode string `json:"permission_mode"`
}

// ProfileRequest sets the launch profile of a folder thread's next turn.
type ProfileRequest struct {
	Profile string `json:"profile"`
}

// ModelRequest sets the model of a folder thread's next turn ("" for the default).
type ModelRequest struct {
	Model string `json:"model"`
}

// HandoffResponse is the command that continues the thread in a terminal.
type HandoffResponse struct {
	Command string `json:"command"`
}

// PermissionModesResponse lists the selectable modes.
type PermissionModesResponse struct {
	Modes []string `json:"modes"`
}

// TaskOutputResponse is the end of a background task's output.
type TaskOutputResponse struct {
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	Size      int64  `json:"size"`
}

// MergeBlockedResponse explains why a merge was not done.
type MergeBlockedResponse struct {
	Error string `json:"error"`
	// Command does the merge by hand.
	Command string `json:"command"`
}

// maxUpload caps one uploaded file.
const maxUpload = 200 << 20

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
	api.GET("/projects/:id/threads", h.list, tag,
		swagger.WithDescription("A project's threads, most recently updated first (not its main thread)"),
		swagger.WithResponseModel(ThreadsResponse{}))
	api.POST("/projects/:id/threads", h.create, tag,
		swagger.WithDescription("Start a thread in a folder project"),
		swagger.WithRequestModel(CreateThreadRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.GET("/threads/permission-modes", h.permissionModes, tag,
		swagger.WithDescription("The Claude Code permission modes a thread can run in"),
		swagger.WithResponseModel(PermissionModesResponse{}))

	api.GET("/threads/:id", h.get, tag,
		swagger.WithDescription("A thread: its state, whether a turn is running and its background tasks"),
		swagger.WithResponseModel(ThreadDetailResponse{}))
	api.GET("/threads/:id/events", h.events, tag,
		swagger.WithDescription("A thread's events, oldest first; for a main thread, the project conversation"),
		swagger.WithQuery("parent", "string", "An Agent call's request id: page through that subagent's steps instead of the thread's own conversation"),
		swagger.WithQuery("limit", "integer", "Latest N events (of those before `before`); on the conversation, every older usage event comes too"),
		swagger.WithQuery("before", "string", "An event id: only events older than it"),
		swagger.WithQuery("from", "string", "An event id: every event from it on (up to `before`), to extend a loaded page back to it in one read; limit does not apply"),
		swagger.WithQuery("call", "string", "A tool call's request id: that call's own events on the conversation (the call, its result, its task events); nothing else applies"),
		swagger.WithQuery("since", "string", "RFC 3339 time: every event added or changed at or after it (its updated_at), subagent steps included; parent, limit, before and from do not apply"),
		swagger.WithResponseModel(EventsResponse{}))
	api.GET("/threads/:id/subagents", h.subagents, tag,
		swagger.WithDescription("A thread's subagents (its Agent calls), oldest first: to find one in a long transcript without reading it"),
		swagger.WithResponseModel(SubagentsResponse{}))
	api.POST("/threads/:id/messages", h.send, tag,
		swagger.WithDescription("Send a message to the thread; while a turn runs, mode says whether it joins the turn, waits for it or replaces it. To a main thread it goes to the coordinator, whose reply arrives on the event stream"),
		swagger.WithRequestModel(SendRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.DELETE("/threads/:id/queue/:qid", h.unqueue, tag,
		swagger.WithDescription("Take back a message held for after the running turn"),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/threads/:id/events/:eid/start", h.startSuggestions, tag,
		swagger.WithDescription("Start threads the coordinator proposed in a suggestions event of a main thread"),
		swagger.WithRequestModel(StartSuggestionsRequest{}),
		swagger.WithResponseModel(db.ThreadEvent{}))
	api.POST("/threads/:id/respond", h.respond, tag,
		swagger.WithDescription("Answer a permission request or question the thread is waiting on"),
		swagger.WithRequestModel(ThreadRespondRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/threads/:id/stop", h.stop, tag,
		swagger.WithDescription("Interrupt the running turn; the process and its background tasks survive"),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/threads/:id/archive", h.archive, tag,
		swagger.WithDescription("Archive (resolve) the thread; it stays readable and can be reopened"),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/threads/:id/reopen", h.reopen, tag,
		swagger.WithDescription("Reopen an archived thread"),
		swagger.WithResponseModel(db.Thread{}))
	api.PUT("/threads/:id/auto-answer", h.setAutoAnswer, tag,
		swagger.WithDescription("Set when the thread answers a question with its suggested option by itself"),
		swagger.WithRequestModel(AutoAnswerRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.PUT("/threads/:id/permission-mode", h.setPermissionMode, tag,
		swagger.WithDescription("Set the permission mode of the thread's next turn"),
		swagger.WithRequestModel(PermissionModeRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.PUT("/threads/:id/profile", h.setProfile, tag,
		swagger.WithDescription("Set the launch profile of a folder thread's next turn (hosts with profiles only)"),
		swagger.WithRequestModel(ProfileRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.PUT("/threads/:id/model", h.setModel, tag,
		swagger.WithDescription("Set the model of a folder thread's next turn"),
		swagger.WithRequestModel(ModelRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.POST("/threads/:id/handoff", h.handoff, tag,
		swagger.WithDescription("Release the thread to a terminal: closes its process and returns the command that resumes it"),
		swagger.WithResponseModel(HandoffResponse{}))
	api.POST("/threads/:id/tasks/:task_id/stop", h.stopTask, tag,
		swagger.WithDescription("Stop one background task"))
	api.GET("/threads/:id/tasks/:task_id/output", h.taskOutput, tag,
		swagger.WithDescription("The end of a background task's output"),
		swagger.WithQuery("tail", "int", "Bytes from the end (default 8192)"),
		swagger.WithResponseModel(TaskOutputResponse{}))
	api.POST("/threads/:id/attachments", h.uploadAttachments, tag,
		swagger.WithDescription("Upload files for a message to the thread (multipart field \"files\")"),
		swagger.WithResponseModel(rt.AttachmentsResponse{}))
	api.GET("/threads/:id/attachments/file", h.downloadAttachment, tag,
		swagger.WithDescription("Download an attachment of the thread"),
		swagger.WithQuery("path", "string", "Path returned by the upload"))
	api.GET("/threads/:id/diff", h.diff, tag,
		swagger.WithDescription("The thread's changes against its base branch"),
		swagger.WithResponseModel(rt.Diff{}))
	api.POST("/threads/:id/merge", h.merge, tag,
		swagger.WithDescription("Merge the thread's branch into its base branch in the main checkout. 409 with a command to run by hand when the checkout is not ready."),
		swagger.WithResponseModel(db.Thread{}),
		swagger.WithErrorResponses(swagger.ErrorResponseConfig{Code: http.StatusConflict, Message: "merge blocked", Model: MergeBlockedResponse{}}))
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		apierr.Store(c, err, "thread")
	case errors.Is(err, rt.ErrPaused), errors.Is(err, rt.ErrArchived), errors.Is(err, rt.ErrResolved), errors.Is(err, rt.ErrNoPrompt),
		errors.Is(err, rt.ErrTaskNotRunning), errors.Is(err, rt.ErrBusy):
		apierr.Message(c, http.StatusConflict, err.Error())
	case errors.Is(err, rt.ErrTaskOutputUnset), errors.Is(err, os.ErrNotExist):
		// The output file may be gone; never echo its path.
		apierr.Message(c, http.StatusNotFound, rt.ErrTaskOutputUnset.Error())
	default:
		apierr.BadRequest(c, err.Error())
	}
}

// thread loads the thread named in the path.
func (h *Handler) thread(c *gin.Context) (*db.Thread, bool) {
	t, err := h.svc.Store.GetThread(c.Param("id"))
	if err != nil {
		fail(c, err)
		return nil, false
	}
	return t, true
}

// worker loads the thread named in the path for an action that only makes
// sense on a thread that runs an agent session. A project's main thread is its
// conversation with the coordinator: it cannot be archived, stopped, merged,
// answered or configured like a worker, so those requests are refused (400)
// instead of acting on the conversation.
func (h *Handler) worker(c *gin.Context) (*db.Thread, bool) {
	t, ok := h.thread(c)
	if !ok {
		return nil, false
	}
	if t.Agent == db.AgentLLM {
		apierr.BadRequest(c, "not available on a project's main thread; use the project's own settings and messages")
		return nil, false
	}
	return t, true
}

// owner is whose uploads the thread's messages read.
func (h *Handler) owner(t *db.Thread) (string, error) {
	p, err := h.svc.Store.GetProject(t.ProjectID)
	if err != nil {
		return "", err
	}
	return rt.UploadOwner(p, t), nil
}

func (h *Handler) list(c *gin.Context) {
	if _, err := h.svc.Store.GetProject(c.Param("id")); err != nil {
		apierr.Store(c, err, "project")
		return
	}
	threads, err := h.svc.Store.ListThreads(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	if threads == nil {
		threads = []db.Thread{}
	}
	c.JSON(http.StatusOK, ThreadsResponse{Threads: threads})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateThreadRequest
	if !apierr.Bind(c, &req) {
		return
	}
	atts, err := h.svc.Layout.ResolveAttachments(rt.DraftOwner, req.Attachments)
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	t, err := h.svc.Threads.StartFolderThread(rt.FolderThreadSpec{
		ProjectID: c.Param("id"), Task: req.Content, Attachments: atts, Title: req.Title, Model: req.Model,
		PermissionMode: req.PermissionMode, Profile: req.Profile,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, t)
}

func (h *Handler) permissionModes(c *gin.Context) {
	c.JSON(http.StatusOK, PermissionModesResponse{Modes: rt.PermissionModes})
}

func (h *Handler) get(c *gin.Context) {
	t, ok := h.thread(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, ThreadDetailResponse{Thread: *t, Running: h.svc.Threads.Running(t.ID), BackgroundTasks: h.svc.Threads.BackgroundTasks(t.ID)})
}

func (h *Handler) events(c *gin.Context) {
	t, ok := h.thread(c)
	if !ok {
		return
	}
	page := db.EventPage{Parent: c.Query("parent"), Before: c.Query("before"), From: c.Query("from"), Call: c.Query("call")}
	page.Limit, _ = strconv.Atoi(c.Query("limit"))
	if v := c.Query("since"); v != "" {
		since, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			apierr.BadRequest(c, "since must be an RFC 3339 time")
			return
		}
		page.Since = since
	}
	events, more, err := h.svc.Store.PageThreadEvents(t.ID, page)
	if errors.Is(err, db.ErrNotFound) {
		apierr.Store(c, err, "event")
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if events == nil {
		events = []db.ThreadEvent{}
	}
	c.JSON(http.StatusOK, EventsResponse{Events: events, More: more})
}

func (h *Handler) subagents(c *gin.Context) {
	t, ok := h.thread(c)
	if !ok {
		return
	}
	list, err := h.svc.Store.ThreadSubagents(t.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, SubagentsResponse{Subagents: list})
}

func (h *Handler) send(c *gin.Context) {
	var req SendRequest
	if !apierr.Bind(c, &req) {
		return
	}
	if strings.TrimSpace(req.Content) == "" && len(req.Attachments) == 0 {
		apierr.BadRequest(c, "message is empty")
		return
	}
	if !rt.SendMode(req.Mode).Valid() {
		apierr.BadRequest(c, "mode must be send, queue or stop_and_send")
		return
	}
	cur, ok := h.thread(c)
	if !ok {
		return
	}
	var atts []rt.Attachment
	if len(req.Attachments) > 0 {
		owner, err := h.owner(cur)
		if err == nil {
			atts, err = h.svc.Layout.ResolveAttachments(owner, req.Attachments)
		}
		if err != nil {
			apierr.BadRequest(c, err.Error())
			return
		}
	}
	t, err := h.svc.SendWith(c.Request.Context(), cur.ID, req.Content, atts, rt.Source{}, rt.SendMode(req.Mode))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) unqueue(c *gin.Context) {
	cur, ok := h.thread(c)
	if !ok {
		return
	}
	if !h.svc.Threads.RemoveQueued(cur.ID, c.Param("qid")) {
		apierr.Message(c, http.StatusConflict, "the message was already sent")
		return
	}
	h.reply(c, cur.ID)
}

func (h *Handler) startSuggestions(c *gin.Context) {
	var req StartSuggestionsRequest
	if !apierr.Bind(c, &req) {
		return
	}
	m, err := h.svc.Coordinator.StartSuggestions(c.Request.Context(), c.Param("id"), c.Param("eid"), req.Indexes)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *Handler) respond(c *gin.Context) {
	var req ThreadRespondRequest
	if !apierr.Bind(c, &req) {
		return
	}
	cur, ok := h.worker(c)
	if !ok {
		return
	}
	if err := h.svc.Threads.Respond(cur.ID, req.RequestID, rt.Answer{Allow: req.Allow, Scope: req.Scope, Answer: req.Answer, Answers: req.Answers}); err != nil {
		fail(c, err)
		return
	}
	h.reply(c, cur.ID)
}

func (h *Handler) stop(c *gin.Context) {
	cur, ok := h.worker(c)
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

func (h *Handler) archive(c *gin.Context) {
	cur, ok := h.worker(c)
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
	cur, ok := h.worker(c)
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
	cur, ok := h.worker(c)
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

func (h *Handler) setAutoAnswer(c *gin.Context) {
	var req AutoAnswerRequest
	if !apierr.Bind(c, &req) {
		return
	}
	t, ok := h.worker(c)
	if !ok {
		return
	}
	out, err := h.svc.Threads.SetAutoAnswer(t.ID, req.AutoAnswer)
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
	t, ok := h.worker(c)
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
	t, ok := h.worker(c)
	if !ok {
		return
	}
	out, err := h.svc.Threads.SetProfile(c.Request.Context(), t.ID, req.Profile)
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
	t, ok := h.worker(c)
	if !ok {
		return
	}
	out, err := h.svc.Threads.SetModel(c.Request.Context(), t.ID, req.Model)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) handoff(c *gin.Context) {
	t, ok := h.worker(c)
	if !ok {
		return
	}
	cmd, err := h.svc.Threads.Handoff(t.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, HandoffResponse{Command: cmd})
}

func (h *Handler) stopTask(c *gin.Context) {
	t, ok := h.thread(c)
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
	t, ok := h.thread(c)
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
	c.JSON(http.StatusOK, TaskOutputResponse{Content: out.Content, Truncated: out.Truncated, Size: out.Size})
}

func (h *Handler) uploadAttachments(c *gin.Context) {
	t, ok := h.thread(c)
	if !ok {
		return
	}
	owner, err := h.owner(t)
	if err != nil {
		fail(c, err)
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		apierr.BadRequest(c, `expected multipart form with field "files"`)
		return
	}
	atts, err := h.svc.Layout.SaveUploads(owner, form.File["files"], maxUpload)
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, rt.AttachmentsResponse{Attachments: atts})
}

func (h *Handler) downloadAttachment(c *gin.Context) {
	t, ok := h.thread(c)
	if !ok {
		return
	}
	owner, err := h.owner(t)
	if err != nil {
		fail(c, err)
		return
	}
	rel := filepath.ToSlash(filepath.Clean(c.Query("path")))
	if !strings.HasPrefix(rel, rt.UploadsDir+"/") {
		apierr.BadRequest(c, "invalid attachment path")
		return
	}
	path, err := h.svc.Layout.LibraryPath(owner, rel)
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		apierr.Message(c, http.StatusNotFound, "file not found")
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}
