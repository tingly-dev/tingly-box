// HTTP server example demonstrating AgentService.
// Shows: list projects, list sessions, execute new session, resume session.
//
// Run: go run main.go
// Requires: claude CLI installed and available in PATH.
//
// Endpoints:
//   GET  /projects                          - list known projects
//   GET  /sessions?project=<path>&limit=20  - list sessions for a project
//   GET  /sessions/:id                      - get session metadata
//   GET  /sessions/:id/summary              - get session summary (query: ?head=5&tail=5)
//   POST /execute                           - run a new agent session
//   POST /sessions/:id/execute              - resume an existing session

//go:build ignore

package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// executeRequest is the body for POST /execute and POST /sessions/:id/execute
type executeRequest struct {
	Prompt      string `json:"prompt" binding:"required"`
	ProjectPath string `json:"project_path"` // only for POST /execute
	AgentType   string `json:"agent_type"`   // default: "claude"
	MaxTurns    int    `json:"max_turns"`
	Model       string `json:"model"`
}

// executeResponse is the body returned after execution completes.
type executeResponse struct {
	SessionID string  `json:"session_id"`
	Output    string  `json:"output"`
	Success   bool    `json:"success"`
	DurationS float64 `json:"duration_s"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
	Error     string  `json:"error,omitempty"`
}

func main() {
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		log.Fatalf("init AgentService: %v", err)
	}
	svc.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgent(agentboot.DefaultConfig()))

	r := gin.Default()
	h := &handler{svc: svc}

	r.GET("/projects", h.listProjects)
	r.GET("/sessions", h.listSessions)
	r.GET("/sessions/:id", h.getSession)
	r.GET("/sessions/:id/summary", h.getSessionSummary)
	r.POST("/execute", h.execute)
	r.POST("/sessions/:id/execute", h.resumeSession)

	log.Println("listening on :9090")
	if err := r.Run(":9090"); err != nil {
		log.Fatalf("server: %v", err)
	}
}

type handler struct {
	svc *agentboot.AgentService
}

// GET /projects
func (h *handler) listProjects(c *gin.Context) {
	projects, err := h.svc.ListProjects(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// GET /sessions?project=<path>&limit=20
func (h *handler) listSessions(c *gin.Context) {
	projectPath := c.Query("project")
	if projectPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query param 'project' is required"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	sessions, err := h.svc.ListSessions(c.Request.Context(), projectPath, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions, "project": projectPath})
}

// GET /sessions/:id
func (h *handler) getSession(c *gin.Context) {
	meta, err := h.svc.GetSession(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if isNotFound(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, meta)
}

// GET /sessions/:id/summary?head=5&tail=5
func (h *handler) getSessionSummary(c *gin.Context) {
	head, _ := strconv.Atoi(c.DefaultQuery("head", "5"))
	tail, _ := strconv.Atoi(c.DefaultQuery("tail", "5"))

	summary, err := h.svc.GetSessionSummary(c.Request.Context(), c.Param("id"), head, tail)
	if err != nil {
		status := http.StatusInternalServerError
		if isNotFound(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// POST /execute — run a new agent session
func (h *handler) execute(c *gin.Context) {
	var req executeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agentType := agentboot.AgentType(req.AgentType)
	if agentType == "" {
		agentType = agentboot.AgentTypeClaude
	}
	projectPath := req.ProjectPath
	if projectPath == "" {
		projectPath = "/tmp"
	}

	opts := agentboot.ExecutionOptions{
		MaxTurns: req.MaxTurns,
		Model:    req.Model,
	}

	resp := h.runExecution(c.Request.Context(), func(ctx context.Context) (agentboot.ExecutionHandle, error) {
		return h.svc.Execute(ctx, agentType, projectPath, req.Prompt, opts)
	})
	c.JSON(statusCode(resp), resp)
}

// POST /sessions/:id/execute — resume an existing session
func (h *handler) resumeSession(c *gin.Context) {
	var req executeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sessionID := c.Param("id")
	opts := agentboot.ExecutionOptions{
		MaxTurns: req.MaxTurns,
		Model:    req.Model,
	}

	resp := h.runExecution(c.Request.Context(), func(ctx context.Context) (agentboot.ExecutionHandle, error) {
		return h.svc.ExecuteSession(ctx, sessionID, req.Prompt, opts)
	})
	c.JSON(statusCode(resp), resp)
}

// runExecution calls the supplied factory to get a handle, then drains events
// with auto-approve for all permission/ask requests (suitable for a server).
// Replace the auto-approve prompter with a real one if you need user approval.
func (h *handler) runExecution(
	ctx context.Context,
	factory func(context.Context) (agentboot.ExecutionHandle, error),
) executeResponse {
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	handle, err := factory(execCtx)
	if err != nil {
		return executeResponse{Error: err.Error()}
	}

	start := time.Now()
	result, err := agentboot.RunWithPrompter(execCtx, handle, autoApprove{}, nil)
	if err != nil {
		return executeResponse{Error: err.Error(), DurationS: time.Since(start).Seconds()}
	}

	return executeResponse{
		SessionID: result.GetSessionID(),
		Output:    result.TextOutput(),
		Success:   result.IsSuccess(),
		DurationS: result.Duration.Seconds(),
		CostUSD:   result.GetCostUSD(),
	}
}

// autoApprove is a Prompter that approves all requests without user interaction.
// For production use, replace with a real approval mechanism.
type autoApprove struct{}

func (autoApprove) OnApproval(_ context.Context, _ agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	return agentboot.ApprovalResponse{Approved: true}, nil
}

func (autoApprove) OnAsk(_ context.Context, _ agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	return agentboot.AskResponse{Approved: true}, nil
}

func isNotFound(err error) bool {
	type notFound interface{ IsNotFound() bool }
	if nf, ok := err.(notFound); ok {
		return nf.IsNotFound()
	}
	return false
}

func statusCode(resp executeResponse) int {
	if resp.Error != "" {
		return http.StatusInternalServerError
	}
	if !resp.Success {
		return http.StatusUnprocessableEntity
	}
	return http.StatusOK
}

// Ensure autoApprove satisfies the Prompter interface.
var _ agentboot.Prompter = autoApprove{}
