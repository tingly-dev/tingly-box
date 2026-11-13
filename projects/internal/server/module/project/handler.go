// Package project serves Projects: the project list and settings, the
// project conversation with its coordinator, the live event stream, memory,
// library and usage. Threads are served by the thread module.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/swagger"
)

// ProjectSummary is a project card.
type ProjectSummary struct {
	db.Project
	Threads db.ThreadCounts `json:"threads"`
}

// ProjectListResponse lists projects.
type ProjectListResponse struct {
	Projects []ProjectSummary `json:"projects"`
}

// ProjectDetailResponse is everything the project page needs at load.
type ProjectDetailResponse struct {
	Project db.Project      `json:"project"`
	Threads []db.Thread     `json:"threads"`
	Counts  db.ThreadCounts `json:"counts"`
	// CoordinatorBusy is true while the coordinator is working on a turn.
	CoordinatorBusy     bool   `json:"coordinator_busy"`
	CoordinatorRecovery string `json:"coordinator_recovery,omitempty"`
	// Coordinator is the project's main thread: the conversation's session,
	// with its turns, token use and context fill, for the status panel.
	Coordinator *db.Thread `json:"coordinator,omitempty"`
	// CoordinatorQueued counts inputs (messages, thread reports, settings
	// changes) waiting behind the running turn.
	CoordinatorQueued int `json:"coordinator_queued"`
}

// CreateProjectRequest is the New project dialog, or opens a folder.
type CreateProjectRequest struct {
	// Type is project (the default) or folder.
	Type string `json:"type,omitempty" enum:"project,folder"`
	// Name is required for a project; a folder is named after its folder
	// unless a name is given.
	Name string `json:"name,omitempty"`
	// Path is the folder to open for type folder: it must exist, and the
	// folder's project is returned when it already has one.
	Path string `json:"path,omitempty"`
	Goal string `json:"goal,omitempty"`
	// Repos are local repository paths; each thread branches from the
	// repository's current branch.
	Repos           []string            `json:"repos,omitempty"`
	Directories     *[]rt.DirectorySpec `json:"directories,omitempty"`
	PrimaryRepoPath *string             `json:"primary_repo_path,omitempty"`
	DisableWorktree *bool               `json:"disable_worktree,omitempty"`
}

// ForkProjectRequest names the fork; empty is "<source name> (fork)".
type ForkProjectRequest struct {
	Name string `json:"name,omitempty"`
}

// UpdateProjectRequest changes the fields that are set.
type UpdateProjectRequest struct {
	Name                 *string             `json:"name,omitempty"`
	Goal                 *string             `json:"goal,omitempty"`
	Instructions         *string             `json:"instructions,omitempty"`
	Repos                *[]string           `json:"repos,omitempty"`
	Directories          *[]rt.DirectorySpec `json:"directories,omitempty"`
	PrimaryRepoPath      *string             `json:"primary_repo_path,omitempty"`
	PrimaryDirectoryMode *rt.DirectorySpec   `json:"primary_directory_mode,omitempty"`
	CoordinatorProfile   *string             `json:"coordinator_profile,omitempty"`
	ThreadProfile        *string             `json:"thread_profile,omitempty"`
	CoordinatorModel     *string             `json:"coordinator_model,omitempty"`
	ThreadModel          *string             `json:"thread_model,omitempty"`
	CoordinatorEffort    *string             `json:"coordinator_effort,omitempty" enum:",low,medium,high,max"`
	ThreadEffort         *string             `json:"thread_effort,omitempty" enum:",low,medium,high,max"`
	ThreadPermissionMode *string             `json:"thread_permission_mode,omitempty" enum:"default,acceptEdits,auto,bypassPermissions"`
	DisableWorktree      *bool               `json:"disable_worktree,omitempty"`
	// MaxRunningThreads is how many threads of the project may run at once;
	// the rest wait. 1..100; a new project starts at 4.
	MaxRunningThreads *int  `json:"max_running_threads,omitempty"`
	Starred           *bool `json:"starred,omitempty"`
	Paused            *bool `json:"paused,omitempty"`
	Archived          *bool `json:"archived,omitempty"`
}

// RecoverCoordinatorRequest resolves an interrupted input after reviewing its effects.
type RecoverCoordinatorRequest struct {
	InputID string `json:"input_id" binding:"required"`
	Action  string `json:"action" binding:"required" enum:"retry,skip"`
}

// MemoryResponse lists memory files.
type MemoryResponse struct {
	Dir   string          `json:"dir"`
	Files []rt.MemoryFile `json:"files"`
}

// MemoryFileResponse is one memory file.
type MemoryFileResponse struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// WriteMemoryRequest writes a memory file.
type WriteMemoryRequest struct {
	Content string `json:"content"`
}

// LibraryResponse lists library files.
type LibraryResponse struct {
	Dir   string           `json:"dir"`
	Files []rt.LibraryFile `json:"files"`
}

// LibraryTreeResponse lists one directory of the library.
type LibraryTreeResponse struct {
	Dir     string            `json:"dir"`
	Path    string            `json:"path"`
	Entries []rt.LibraryEntry `json:"entries"`
}

// UsageResponse is token use by model and by thread.
type UsageResponse struct {
	ByModel  []db.ModelUsage `json:"by_model"`
	ByThread []ThreadUsage   `json:"by_thread"`
}

// ThreadUsage is one thread's token use.
type ThreadUsage struct {
	ThreadID     string `json:"thread_id"`
	Title        string `json:"title"`
	Model        string `json:"model"`
	Turns        int    `json:"turns"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// InspectRepoRequest checks a local repository path.
type InspectRepoRequest struct {
	Path string `json:"path" binding:"required"`
}

// InspectRepoResponse is the concrete repository a path resolves to.
type InspectRepoResponse struct {
	OK     bool   `json:"ok"`
	Root   string `json:"root,omitempty"`
	Branch string `json:"branch,omitempty"`
	Dirty  bool   `json:"dirty,omitempty"`
	Error  string `json:"error,omitempty"`
}

// InspectDirectoryResponse describes a Git repository or a plain directory.
type InspectDirectoryResponse struct {
	OK     bool   `json:"ok"`
	Root   string `json:"root"`
	Kind   string `json:"kind" enum:",git,directory"`
	Branch string `json:"branch"`
	Dirty  bool   `json:"dirty"`
	Error  string `json:"error"`
}

// ProjectModelsResponse lists the models the host offers. Error is set (and
// Models may be empty) when the list could not be fetched; a model ID can
// still be typed by hand.
type ProjectModelsResponse struct {
	Models []rt.Model `json:"models"`
	Error  string     `json:"error,omitempty"`
}

// ProjectRuntimeResponse is what threads run on.
type ProjectRuntimeResponse struct {
	// CLIPath is the Claude Code CLI; empty when none is installed.
	CLIPath string `json:"cli_path"`
	// BaseURL is the gateway threads send model calls to; empty when
	// Claude Code uses its own login.
	BaseURL string `json:"base_url"`
	// MaxRunningThreadsLimit is the most threads a project can be limited to.
	MaxRunningThreadsLimit int `json:"max_running_threads_limit"`
}

// OKResponse acknowledges an action.
type OKResponse struct {
	OK bool `json:"ok"`
}

// Handler is the Projects HTTP module.
type Handler struct {
	svc      *rt.Service
	updateMu sync.Mutex
}

// NewHandler builds the module.
func NewHandler(svc *rt.Service) *Handler { return &Handler{svc: svc} }

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts the Projects endpoints.
func (h *Handler) RegisterRoutes(r *module.Routes) {
	api := r.API
	tag := swagger.WithTags("projects")
	api.GET("/projects", h.list, tag,
		swagger.WithDescription("List projects and folders (each has a type) with thread counts per Overview group"),
		swagger.WithQuery("q", "string", "Search name, goal and folder path"),
		swagger.WithQuery("archived", "boolean", "List archived projects"),
		swagger.WithResponseModel(ProjectListResponse{}))
	api.POST("/projects", h.create, tag,
		swagger.WithDescription("Create a project, or open a folder (type folder with its path; the folder's existing project is returned when there is one)"),
		swagger.WithRequestModel(CreateProjectRequest{}),
		swagger.WithResponseModel(db.Project{}))
	api.GET("/projects/models", h.models, tag,
		swagger.WithDescription("Models the coordinator and threads can use"),
		swagger.WithResponseModel(ProjectModelsResponse{}))
	api.GET("/projects/runtime", h.runtime, tag,
		swagger.WithDescription("What threads run on: the Claude Code CLI and the gateway it is pointed at"),
		swagger.WithResponseModel(ProjectRuntimeResponse{}))
	api.POST("/projects/directories", h.browseDirectories, tag,
		swagger.WithDescription("Browse non-hidden directories under the server user home"),
		swagger.WithRequestModel(BrowseDirectoriesRequest{}),
		swagger.WithResponseModel(BrowseDirectoriesResponse{}))
	api.POST("/projects/directories/inspect", h.inspectDirectory, tag,
		swagger.WithDescription("Inspect a local directory or Git repository"),
		swagger.WithRequestModel(InspectRepoRequest{}),
		swagger.WithResponseModel(InspectDirectoryResponse{}))
	api.POST("/projects/repos/inspect", h.inspectRepo, tag,
		swagger.WithDescription("Check a local repository path and return its root and current branch"),
		swagger.WithRequestModel(InspectRepoRequest{}),
		swagger.WithResponseModel(InspectRepoResponse{}))
	api.GET("/projects/:id", h.get, tag,
		swagger.WithDescription("Get a project with its threads"),
		swagger.WithResponseModel(ProjectDetailResponse{}))
	api.POST("/projects/:id/coordinator/recover", h.recoverCoordinator, tag,
		swagger.WithDescription("Resolve an interrupted coordinator input; retry may repeat side effects"),
		swagger.WithRequestModel(RecoverCoordinatorRequest{}),
		swagger.WithResponseModel(ProjectDetailResponse{}))
	api.POST("/projects/:id/coordinator/stop", h.stopCoordinator, tag,
		swagger.WithDescription("Interrupt the running coordinator turn the way the user's stop button means it: the message and the reply produced so far stay, nothing needs recovery"),
		swagger.WithResponseModel(ProjectDetailResponse{}))
	api.PATCH("/projects/:id", h.update, tag,
		swagger.WithDescription("Update project settings; pause, resume, archive, unarchive, star. A folder has no repos or thread model: setting either on a folder is a 400."),
		swagger.WithRequestModel(UpdateProjectRequest{}),
		swagger.WithResponseModel(db.Project{}))
	api.DELETE("/projects/:id", h.delete, tag,
		swagger.WithDescription("Archive a project or folder (the same as PATCH archived=true): its threads stop and stay readable, its IM chats are unbound, nothing is deleted"),
		swagger.WithResponseModel(OKResponse{}))
	api.POST("/projects/:id/fork", h.fork, tag,
		swagger.WithDescription("Fork a project into a new one: its settings, library files and memory are copied; its conversation, threads and usage are not (the fork starts like a new project). A folder cannot be forked"),
		swagger.WithRequestModel(ForkProjectRequest{}),
		swagger.WithResponseModel(db.Project{}))

	api.GET("/projects/events", h.allEvents, tag,
		swagger.WithDescription("Server-sent events for every project, for the sidebar and lists: project, thread and coordinator frames only. Transcript entries (thread_event) and streamed coordinator text are not sent here; subscribe to /projects/{id}/events for those"),
		swagger.WithResponseModel(rt.Event{}))
	api.GET("/projects/:id/events", h.events, tag,
		swagger.WithDescription("Server-sent events for one project: message_delta (streamed coordinator text), thread, thread_event (a main thread's are the project conversation), project, coordinator"),
		swagger.WithResponseModel(rt.Event{}))

	api.GET("/projects/:id/memory", h.listMemory, tag,
		swagger.WithDescription("List memory files"),
		swagger.WithResponseModel(MemoryResponse{}))
	api.GET("/projects/:id/memory/:name", h.readMemory, tag,
		swagger.WithDescription("Read a memory file"),
		swagger.WithResponseModel(MemoryFileResponse{}))
	api.PUT("/projects/:id/memory/:name", h.writeMemory, tag,
		swagger.WithDescription("Write a memory file"),
		swagger.WithRequestModel(WriteMemoryRequest{}),
		swagger.WithResponseModel(MemoryFileResponse{}))
	api.DELETE("/projects/:id/memory/:name", h.deleteMemory, tag,
		swagger.WithDescription("Delete a memory file"),
		swagger.WithResponseModel(OKResponse{}))

	api.GET("/projects/:id/library", h.listLibrary, tag,
		swagger.WithDescription("List library files (uploads and thread outputs)"),
		swagger.WithResponseModel(LibraryResponse{}))
	api.GET("/projects/:id/library/tree", h.libraryTree, tag,
		swagger.WithDescription("List one directory level of the library (folders first); expand a folder by asking for its path"),
		swagger.WithQuery("path", "string", "Library-relative directory; empty is the library root"),
		swagger.WithResponseModel(LibraryTreeResponse{}))
	api.GET("/projects/:id/library/archive", h.archiveLibrary, tag,
		swagger.WithDescription("Download a library folder as a zip (at most 200 files and 40 MB uncompressed; over that is 413 with the reason). Hidden entries and symlinks are left out."),
		swagger.WithQuery("path", "string", "Library-relative folder; empty is the library root"))
	api.POST("/projects/:id/library", h.uploadLibrary, tag,
		swagger.WithDescription("Upload files (multipart field \"files\") to the library"),
		swagger.WithResponseModel(LibraryResponse{}))
	api.POST("/projects/:id/attachments", h.uploadAttachments, tag,
		swagger.WithDescription("Upload files for a message (multipart field \"files\"); they are kept in the library under uploads/"),
		swagger.WithResponseModel(rt.AttachmentsResponse{}))
	api.GET("/projects/:id/library/file", h.downloadLibrary, tag,
		swagger.WithDescription("Download a library file"),
		swagger.WithQuery("path", "string", "Library-relative path"))
	api.DELETE("/projects/:id/library/file", h.deleteLibrary, tag,
		swagger.WithDescription("Delete a library file"),
		swagger.WithQuery("path", "string", "Library-relative path"),
		swagger.WithResponseModel(OKResponse{}))

	api.GET("/projects/:id/usage", h.usage, tag,
		swagger.WithDescription("Token use by model and by thread"),
		swagger.WithResponseModel(UsageResponse{}))
}

func (h *Handler) project(c *gin.Context) (*db.Project, bool) {
	p, err := h.svc.Store.GetProject(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "project")
		return nil, false
	}
	return p, true
}

func projectView(p db.Project) db.Project {
	p.Repos = append([]db.Repo(nil), p.Repos...)
	p.ApplyDirectoryDefaults()
	return p
}

func (h *Handler) list(c *gin.Context) {
	projects, err := h.svc.Store.ListProjects(c.Query("q"), c.Query("archived") == "true")
	if err != nil {
		apierr.Store(c, err, "projects")
		return
	}
	all, err := h.svc.Store.ThreadCounts()
	if err != nil {
		apierr.Store(c, err, "threads")
		return
	}
	out := make([]ProjectSummary, 0, len(projects))
	for _, p := range projects {
		out = append(out, ProjectSummary{Project: projectView(p), Threads: db.CountsOf(all[p.ID])})
	}
	c.JSON(http.StatusOK, ProjectListResponse{Projects: out})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateProjectRequest
	if !apierr.Bind(c, &req) {
		return
	}
	if req.Repos != nil && (req.Directories != nil || req.PrimaryRepoPath != nil) {
		apierr.BadRequest(c, "repos cannot be combined with directory settings")
		return
	}
	if req.Directories != nil && req.DisableWorktree != nil {
		apierr.BadRequest(c, "directories cannot be combined with disable_worktree")
		return
	}
	if req.Type == db.ProjectFolder && (req.Directories != nil || req.PrimaryRepoPath != nil || req.DisableWorktree != nil) {
		apierr.BadRequest(c, "directory settings do not apply to a folder")
		return
	}
	p, err := h.svc.Create(c.Request.Context(), rt.NewProjectSpec{Type: req.Type, Name: req.Name, Goal: req.Goal, Repos: req.Repos, Path: req.Path, Directories: req.Directories, PrimaryRepoPath: req.PrimaryRepoPath})
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	status := http.StatusCreated
	if p.Type == db.ProjectFolder {
		// Opening a folder returns its project, new or not.
		status = http.StatusOK
	}
	c.JSON(status, projectView(*p))
}

func (h *Handler) fork(c *gin.Context) {
	var req ForkProjectRequest
	if !apierr.Bind(c, &req) {
		return
	}
	fork, err := h.svc.Fork(c.Param("id"), req.Name)
	if err != nil {
		// An unknown project is 404; everything else (a folder, a failed
		// copy) is the caller's mistake in words.
		if errors.Is(err, db.ErrNotFound) {
			apierr.Store(c, err, "project")
			return
		}
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, projectView(*fork))
}

func (h *Handler) models(c *gin.Context) {
	models, err := h.svc.Host.Models(c.Request.Context())
	resp := ProjectModelsResponse{Models: models}
	if resp.Models == nil {
		resp.Models = []rt.Model{}
	}
	if err != nil {
		resp.Error = err.Error()
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) runtime(c *gin.Context) {
	env, _ := h.svc.Host.ThreadEnv(c.Request.Context())
	resp := ProjectRuntimeResponse{CLIPath: rt.ClaudeCLIPath(), MaxRunningThreadsLimit: rt.MaxRunningThreadsLimit}
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "ANTHROPIC_BASE_URL="); ok {
			resp.BaseURL = v
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) inspectDirectory(c *gin.Context) {
	var req InspectRepoRequest
	if !apierr.Bind(c, &req) {
		return
	}
	info, err := rt.InspectDirectory(c.Request.Context(), req.Path)
	if err != nil {
		c.JSON(http.StatusOK, InspectDirectoryResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, InspectDirectoryResponse{OK: true, Root: info.Root, Kind: info.Kind, Branch: info.Branch, Dirty: info.Dirty})
}

func (h *Handler) inspectRepo(c *gin.Context) {
	var req InspectRepoRequest
	if !apierr.Bind(c, &req) {
		return
	}
	repos, err := rt.ResolveRepos(c.Request.Context(), []string{req.Path})
	if err != nil {
		c.JSON(http.StatusOK, InspectRepoResponse{Error: err.Error()})
		return
	}
	info, err := rt.InspectRepo(c.Request.Context(), repos[0].Path)
	if err != nil {
		c.JSON(http.StatusOK, InspectRepoResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, InspectRepoResponse{OK: true, Root: info.Root, Branch: info.Branch, Dirty: info.Dirty})
}

func (h *Handler) recoverCoordinator(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	var req RecoverCoordinatorRequest
	if !apierr.Bind(c, &req) {
		return
	}
	if req.Action != "retry" && req.Action != "skip" {
		apierr.BadRequest(c, "action must be retry or skip")
		return
	}
	if err := h.svc.Coordinator.RecoverInput(p.ID, req.InputID, req.Action == "retry"); err != nil {
		apierr.Message(c, http.StatusConflict, err.Error())
		return
	}
	h.get(c)
}

// stopCoordinator interrupts the running coordinator turn the way the stop
// button means it: nothing is left uncertain, the conversation carries on.
func (h *Handler) stopCoordinator(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	if err := h.svc.Coordinator.StopTurn(p.ID); err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	h.get(c)
}

func (h *Handler) get(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	threads, err := h.svc.Store.ListThreads(p.ID)
	if err != nil {
		apierr.Store(c, err, "threads")
		return
	}
	m := map[string]int{}
	for _, t := range threads {
		m[t.Status]++
	}
	if threads == nil {
		threads = []db.Thread{}
	}
	h.svc.Threads.Annotate(threads)
	recovery, err := h.svc.Store.UncertainCoordinatorInput(p.ID)
	if err != nil {
		apierr.Store(c, err, "coordinator recovery")
		return
	}
	queued, err := h.svc.Store.PendingCoordinatorInputs(p.ID)
	if err != nil {
		apierr.Store(c, err, "coordinator queue")
		return
	}
	var coordinator *db.Thread
	if p.MainThreadID != "" {
		if coordinator, err = h.svc.Store.GetThread(p.MainThreadID); err != nil && !errors.Is(err, db.ErrNotFound) {
			apierr.Store(c, err, "coordinator")
			return
		}
	}
	busy := h.svc.Coordinator.Busy(p.ID)
	c.JSON(http.StatusOK, ProjectDetailResponse{Project: projectView(*p), Threads: threads, Counts: db.CountsOf(m), CoordinatorBusy: busy, CoordinatorRecovery: recovery, Coordinator: coordinatorView(coordinator, busy, recovery), CoordinatorQueued: queued})
}

// coordinatorView gives the main thread a thread's own status, so the page reads
// the coordinator like any thread: working during a turn, waiting on the user
// while an interrupted input needs a decision, otherwise idle. The row itself
// does not track this; the coordinator's queue does.
func coordinatorView(main *db.Thread, busy bool, recovery string) *db.Thread {
	if main == nil {
		return nil
	}
	view := *main
	switch {
	case recovery != "":
		view.Status, view.WaitReason = db.ThreadWaiting, "An interrupted message needs your decision."
	case busy:
		view.Status, view.WaitReason = db.ThreadWorking, ""
	default:
		view.Status, view.WaitReason = db.ThreadIdle, ""
	}
	return &view
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateProjectRequest
	if !apierr.Bind(c, &req) {
		return
	}
	h.updateMu.Lock()
	defer h.updateMu.Unlock()
	p, ok := h.project(c)
	if !ok {
		return
	}
	if req.PrimaryDirectoryMode != nil && (req.Repos != nil || req.Directories != nil || req.PrimaryRepoPath != nil || req.DisableWorktree != nil) {
		apierr.BadRequest(c, "primary_directory_mode cannot be combined with directory settings")
		return
	}
	if req.Repos != nil && (req.Directories != nil || req.PrimaryRepoPath != nil) {
		apierr.BadRequest(c, "repos cannot be combined with directory settings")
		return
	}
	if req.Directories != nil && req.DisableWorktree != nil {
		apierr.BadRequest(c, "directories cannot be combined with disable_worktree")
		return
	}
	if p.Type == db.ProjectFolder && (req.Repos != nil || req.Directories != nil || req.PrimaryRepoPath != nil || req.PrimaryDirectoryMode != nil || req.ThreadModel != nil || req.ThreadEffort != nil || req.ThreadProfile != nil || req.CoordinatorProfile != nil || req.DisableWorktree != nil || req.MaxRunningThreads != nil) {
		apierr.BadRequest(c, "repository and project thread settings do not apply to a folder")
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			apierr.BadRequest(c, "name cannot be empty")
			return
		}
		p.Name = name
	}
	if req.Goal != nil {
		p.Goal = strings.TrimSpace(*req.Goal)
	}
	if req.Instructions != nil {
		p.Instructions = *req.Instructions
	}
	if req.Repos != nil || req.Directories != nil || req.PrimaryRepoPath != nil {
		repos := p.Repos
		var err error
		if req.Repos != nil {
			specs, specErr := rt.LegacyDirectorySpecs(*req.Repos)
			if specErr != nil {
				apierr.BadRequest(c, specErr.Error())
				return
			}
			repos, err = rt.ResolveDirectories(c.Request.Context(), specs, p.Repos, p.DisableWorktree)
		} else if req.Directories != nil {
			repos, err = rt.ResolveDirectories(c.Request.Context(), *req.Directories, p.Repos, p.DisableWorktree)
		}
		if err != nil {
			apierr.BadRequest(c, err.Error())
			return
		}
		primary := p.PrimaryRepoPath
		if req.PrimaryRepoPath != nil {
			primary = *req.PrimaryRepoPath
		} else if req.Repos != nil || req.Directories != nil {
			found := false
			for _, repo := range repos {
				if repo.Path == primary {
					found = true
					break
				}
			}
			if !found {
				primary = ""
				if len(repos) > 0 {
					primary = repos[0].Path
				}
			}
		}
		if err := rt.SetProjectDirectories(p, repos, primary); err != nil {
			apierr.BadRequest(c, err.Error())
			return
		}
	}
	if req.PrimaryDirectoryMode != nil {
		primary := p.PrimaryRepo()
		change := req.PrimaryDirectoryMode
		if primary == nil || primary.Path != change.Path {
			apierr.Message(c, http.StatusConflict, "primary directory changed; reload and try again")
			return
		}
		if change.WorkspaceMode != db.WorkspaceShared && change.WorkspaceMode != db.WorkspaceWorktree {
			apierr.BadRequest(c, "invalid workspace mode")
			return
		}
		if p.EffectiveRepo(*primary).Kind == db.RepoDirectory && change.WorkspaceMode != db.WorkspaceShared {
			apierr.BadRequest(c, "non-Git directory must use shared mode")
			return
		}
		primary.WorkspaceMode = change.WorkspaceMode
	}
	for _, item := range []struct {
		in  *string
		out *string
	}{{req.CoordinatorProfile, &p.CoordinatorProfile}, {req.ThreadProfile, &p.ThreadProfile}} {
		if item.in != nil {
			v := strings.TrimSpace(*item.in)
			if !h.svc.Threads.Launchable(v) {
				apierr.BadRequest(c, "profile unavailable")
				return
			}
			*item.out = v
		}
	}
	if req.CoordinatorModel != nil {
		p.CoordinatorModel = strings.TrimSpace(*req.CoordinatorModel)
	}
	if req.ThreadModel != nil {
		p.ThreadModel = strings.TrimSpace(*req.ThreadModel)
	}
	if l, ok := h.svc.Host.(rt.Launcher); ok {
		for _, item := range []struct {
			changed        bool
			profile, model string
		}{
			{req.CoordinatorProfile != nil || req.CoordinatorModel != nil, p.CoordinatorProfile, p.CoordinatorModel},
			{req.ThreadProfile != nil || req.ThreadModel != nil, p.ThreadProfile, p.ThreadModel},
		} {
			if item.changed && item.profile != "" {
				if err := l.CheckLaunch(c.Request.Context(), item.profile, item.model); err != nil {
					apierr.BadRequest(c, err.Error())
					return
				}
			}
		}
	}
	for _, effort := range []struct {
		req *string
		dst *string
	}{
		{req.CoordinatorEffort, &p.CoordinatorEffort},
		{req.ThreadEffort, &p.ThreadEffort},
	} {
		if effort.req == nil {
			continue
		}
		v := strings.TrimSpace(*effort.req)
		if !rt.ValidEffort(v) {
			apierr.BadRequest(c, "unknown effort level")
			return
		}
		*effort.dst = v
	}
	if req.ThreadPermissionMode != nil {
		switch *req.ThreadPermissionMode {
		case "default", "acceptEdits", "auto", "bypassPermissions":
			p.ThreadPermissionMode = *req.ThreadPermissionMode
		default:
			apierr.BadRequest(c, "unknown permission mode")
			return
		}
	}
	if req.DisableWorktree != nil {
		for i := range p.Repos {
			if p.EffectiveRepo(p.Repos[i]).Kind == db.RepoDirectory || *req.DisableWorktree {
				p.Repos[i].WorkspaceMode = db.WorkspaceShared
			} else {
				p.Repos[i].WorkspaceMode = db.WorkspaceWorktree
			}
		}
		p.DisableWorktree = *req.DisableWorktree
	}
	if req.MaxRunningThreads != nil {
		if !rt.ValidMaxRunningThreads(*req.MaxRunningThreads) {
			apierr.BadRequest(c, fmt.Sprintf("max_running_threads must be a whole number from 1 to %d", rt.MaxRunningThreadsLimit))
			return
		}
		p.MaxRunningThreads = *req.MaxRunningThreads
	}
	if req.Starred != nil {
		p.Starred = *req.Starred
	}
	var err error
	switch {
	case req.Archived != nil && (*req.Archived != (p.ArchivedAt != nil)):
		err = h.svc.SetArchived(p, *req.Archived)
	case req.Paused != nil && *req.Paused != p.Paused:
		err = h.svc.SetPaused(p, *req.Paused)
	default:
		err = h.svc.Save(p)
	}
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, projectView(*p))
}

func (h *Handler) delete(c *gin.Context) {
	h.updateMu.Lock()
	defer h.updateMu.Unlock()
	p, ok := h.project(c)
	if !ok {
		return
	}
	if p.ArchivedAt == nil {
		if err := h.svc.SetArchived(p, true); err != nil {
			apierr.BadRequest(c, err.Error())
			return
		}
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) events(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	h.stream(c, p.ID)
}

func (h *Handler) allEvents(c *gin.Context) { h.stream(c, rt.AllProjects) }

func (h *Handler) stream(c *gin.Context, key string) {
	ch, cancel := h.svc.Hub.Subscribe(key)
	defer cancel()
	w := c.Writer
	// A stream outlives the server's WriteTimeout; cut at it, every page
	// would reconnect and re-fetch everything it shows.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
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

func (h *Handler) listMemory(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	files, err := h.svc.Layout.ListMemory(p.ID)
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, MemoryResponse{Dir: h.svc.Layout.MemoryDir(p.ID), Files: files})
}

func (h *Handler) readMemory(c *gin.Context) {
	content, err := h.svc.Layout.ReadMemory(c.Param("id"), c.Param("name"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			apierr.Message(c, http.StatusNotFound, "memory file not found")
			return
		}
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, MemoryFileResponse{Name: c.Param("name"), Content: content})
}

func (h *Handler) writeMemory(c *gin.Context) {
	var req WriteMemoryRequest
	if !apierr.Bind(c, &req) {
		return
	}
	p, ok := h.project(c)
	if !ok {
		return
	}
	if err := h.svc.Layout.WriteMemory(p.ID, c.Param("name"), req.Content); err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, MemoryFileResponse{Name: c.Param("name"), Content: req.Content})
}

func (h *Handler) deleteMemory(c *gin.Context) {
	if err := h.svc.Layout.DeleteMemory(c.Param("id"), c.Param("name")); err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) listLibrary(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	files, err := h.svc.Layout.ListLibrary(p.ID)
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, LibraryResponse{Dir: h.svc.Layout.LibraryDir(p.ID), Files: files})
}

func (h *Handler) libraryTree(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	rel := c.Query("path")
	entries, err := h.svc.Layout.ListLibraryDir(p.ID, rel)
	switch {
	case errors.Is(err, rt.ErrInvalidLibraryPath), errors.Is(err, rt.ErrNotDirectory):
		apierr.BadRequest(c, err.Error())
		return
	case errors.Is(err, fs.ErrNotExist):
		apierr.Message(c, http.StatusNotFound, "folder not found")
		return
	case err != nil:
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, LibraryTreeResponse{Dir: h.svc.Layout.LibraryDir(p.ID), Path: strings.Trim(rel, "/"), Entries: entries})
}

// maxUpload caps one uploaded library file.
const maxUpload = 200 << 20

func (h *Handler) uploadLibrary(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		apierr.BadRequest(c, `expected multipart form with field "files"`)
		return
	}
	for _, fh := range form.File["files"] {
		if fh.Size > maxUpload {
			apierr.BadRequest(c, fmt.Sprintf("%s is larger than 200 MB", fh.Filename))
			return
		}
		if err := h.saveUpload(p.ID, fh); err != nil {
			if errors.Is(err, rt.ErrInvalidLibraryPath) {
				apierr.BadRequest(c, err.Error())
			} else {
				apierr.Message(c, http.StatusInternalServerError, err.Error())
			}
			return
		}
	}
	h.listLibrary(c)
}

func (h *Handler) uploadAttachments(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		apierr.BadRequest(c, `expected multipart form with field "files"`)
		return
	}
	// A folder's uploads go to its library like a project's: they are shared
	// by all of its threads, including the one the first message starts.
	atts, err := h.svc.Layout.SaveUploads(p.ID, form.File["files"], maxUpload)
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, rt.AttachmentsResponse{Attachments: atts})
}

func (h *Handler) saveUpload(projectID string, fh *multipart.FileHeader) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = h.svc.Layout.WriteLibraryFile(projectID, filepath.Base(fh.Filename), src, false)
	return err
}

// libraryErr answers a failed library file lookup: 400 for a path the library
// refuses, 404 for one that does not exist.
func libraryErr(c *gin.Context, err error) {
	if errors.Is(err, rt.ErrInvalidLibraryPath) {
		apierr.BadRequest(c, err.Error())
		return
	}
	apierr.Message(c, http.StatusNotFound, "file not found")
}

// downloadLibrary serves the file from the handle it opened under the
// library's access checks (not by re-opening its path), as an attachment.
func (h *Handler) downloadLibrary(c *gin.Context) {
	f, info, err := h.svc.Layout.OpenLibraryFile(c.Param("id"), c.Query("path"))
	if err != nil {
		libraryErr(c, err)
		return
	}
	defer f.Close()
	module.ServeDownload(c, f, info, rt.DownloadName(c.Query("path")))
}

// archiveLibrary streams a library folder as a zip. The folder is counted
// first, so a refusal (bad path, missing, too big) is a JSON error before any
// of the zip is sent; nothing is written to disk.
func (h *Handler) archiveLibrary(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	a, err := h.svc.Layout.OpenLibraryArchive(p.ID, c.Query("path"))
	var tooBig *rt.ArchiveLimitError
	switch {
	case errors.As(err, &tooBig):
		apierr.Message(c, http.StatusRequestEntityTooLarge, err.Error())
		return
	case errors.Is(err, rt.ErrInvalidLibraryPath), errors.Is(err, rt.ErrNotDirectory):
		apierr.BadRequest(c, err.Error())
		return
	case errors.Is(err, fs.ErrNotExist):
		apierr.Message(c, http.StatusNotFound, "folder not found")
		return
	case err != nil:
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer a.Close()
	if len(a.Files()) == 0 {
		apierr.BadRequest(c, "this folder has no files to download")
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename()}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	// Headers are out by now. Ending the response normally would give the
	// client a truncated zip as a success, so the connection is closed
	// instead and the download fails (a panic with http.ErrAbortHandler would
	// do that too, but gin.Recovery swallows it).
	if err := a.Stream(c.Writer); err != nil {
		_ = c.Error(err)
		if conn, _, herr := c.Writer.Hijack(); herr == nil {
			_ = conn.Close()
		}
	}
}

func (h *Handler) deleteLibrary(c *gin.Context) {
	if err := h.svc.Layout.RemoveLibraryFile(c.Param("id"), c.Query("path")); err != nil {
		libraryErr(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) usage(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	byModel, err := h.svc.Store.ProjectUsage(p.ID)
	if err != nil {
		apierr.Store(c, err, "usage")
		return
	}
	threads, _ := h.svc.Store.ListThreads(p.ID)
	resp := UsageResponse{ByModel: byModel, ByThread: []ThreadUsage{}}
	if resp.ByModel == nil {
		resp.ByModel = []db.ModelUsage{}
	}
	for _, t := range threads {
		resp.ByThread = append(resp.ByThread, ThreadUsage{ThreadID: t.ID, Title: t.Title, Model: t.Model, Turns: t.Turns, InputTokens: t.InputTokens, OutputTokens: t.OutputTokens})
	}
	c.JSON(http.StatusOK, resp)
}
