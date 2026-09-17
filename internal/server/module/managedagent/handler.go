// Package managedagent exposes the /api/v1/managed-agent HTTP endpoints
// backed by internal/managedagent.Service — a web front door onto the same
// remote/session + agentboot machinery IM's @cc already drives.
package managedagent

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
)

type Handler struct {
	svc *managedagent.Service
}

func NewHandler(svc *managedagent.Service) *Handler {
	return &Handler{svc: svc}
}

// sendServiceError maps a managedagent.Service error to an HTTP response
// using the typed sentinels every Service method returns through.
func sendServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, managedagent.ErrNotFound):
		apierr.Send(c, http.StatusNotFound, err, "not_found_error")
	case errors.Is(err, managedagent.ErrValidation):
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
	case errors.Is(err, managedagent.ErrConflict):
		apierr.Send(c, http.StatusConflict, err, "conflict_error")
	default:
		apierr.Send(c, http.StatusInternalServerError, err, "internal_error")
	}
}

// ---------- folders ----------

func (h *Handler) RecentFolders(c *gin.Context) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	folders := h.svc.RecentFolders(limit)
	if folders == nil {
		folders = []managedagent.RecentFolder{}
	}
	c.JSON(http.StatusOK, RecentFoldersResponse{Folders: folders})
}

func (h *Handler) PermissionModes(c *gin.Context) {
	c.JSON(http.StatusOK, PermissionModesResponse{Modes: managedagent.PermissionModes})
}

// ---------- sessions ----------

func (h *Handler) CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	sess, err := h.svc.CreateSession(c.Request.Context(), managedagent.CreateSessionInput{
		Path: req.Path, Prompt: req.Prompt, PermissionMode: req.PermissionMode,
	})
	if err != nil {
		sendServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, sessionToInfo(sess))
}

func (h *Handler) ListSessions(c *gin.Context) {
	active := c.Query("active") == "true"
	sessions := h.svc.ListSessions(active)
	out := make([]SessionInfo, len(sessions))
	for i := range sessions {
		out[i] = sessionToInfo(&sessions[i])
	}
	c.JSON(http.StatusOK, SessionListResponse{Sessions: out})
}

func (h *Handler) GetSession(c *gin.Context) {
	sess, err := h.svc.GetSession(c.Param("session_id"))
	if err != nil {
		sendServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessionToInfo(sess))
}

func (h *Handler) Messages(c *gin.Context) {
	msgs, err := h.svc.Messages(c.Param("session_id"))
	if err != nil {
		sendServiceError(c, err)
		return
	}
	out := make([]MessageInfo, len(msgs))
	for i := range msgs {
		out[i] = messageToInfo(msgs[i])
	}
	c.JSON(http.StatusOK, MessageListResponse{Messages: out})
}

func (h *Handler) SendMessage(c *gin.Context) {
	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	if err := h.svc.SendMessage(c.Request.Context(), c.Param("session_id"), req.Text); err != nil {
		sendServiceError(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}

func (h *Handler) Respond(c *gin.Context) {
	var req RespondRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	if err := h.svc.Respond(c.Param("session_id"), req.RequestID, req.Approved, req.Answer); err != nil {
		sendServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) SetPermissionMode(c *gin.Context) {
	var req SetPermissionModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	sess, err := h.svc.SetPermissionMode(c.Param("session_id"), req.Mode)
	if err != nil {
		sendServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessionToInfo(sess))
}

func (h *Handler) Interrupt(c *gin.Context) {
	if err := h.svc.Interrupt(c.Param("session_id")); err != nil {
		sendServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Archive(c *gin.Context) {
	sess, err := h.svc.Archive(c.Param("session_id"))
	if err != nil {
		sendServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessionToInfo(sess))
}
