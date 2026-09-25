// Package desk exposes the /api/v1/desk HTTP endpoints
// backed by internal/desk.Service — a web front door onto the same
// remote/session + agentboot machinery IM's @cc already drives.
package desk

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/desk"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	"github.com/tingly-dev/tingly-box/remote/session"
)

type Handler struct {
	svc    *desk.Service
	routes RouteResolver // optional: nil leaves the status's routing and quota empty
}

// RouteResolver resolves where a model request is routed and the quota it
// draws on, the way the terminal status line does (statusline.Handler).
type RouteResolver interface {
	ResolveRoute(ctx context.Context, scenario, modelID string) *statusline.Route
}

func NewHandler(svc *desk.Service, routes RouteResolver) *Handler {
	return &Handler{svc: svc, routes: routes}
}

// sendServiceError maps a desk.Service error to an HTTP response
// using the typed sentinels every Service method returns through.
func sendServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, desk.ErrNotFound):
		apierr.Send(c, http.StatusNotFound, err, "not_found_error")
	case errors.Is(err, desk.ErrValidation):
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
	case errors.Is(err, desk.ErrConflict):
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
		folders = []desk.RecentFolder{}
	}
	c.JSON(http.StatusOK, RecentFoldersResponse{Folders: folders})
}

func (h *Handler) PermissionModes(c *gin.Context) {
	c.JSON(http.StatusOK, PermissionModesResponse{Modes: desk.PermissionModes})
}

// ---------- sessions ----------

func (h *Handler) CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	sess, err := h.svc.CreateSession(c.Request.Context(), desk.CreateSessionInput{
		Path: req.Path, Prompt: req.Prompt, PermissionMode: req.PermissionMode, Profile: req.Profile, Model: req.Model,
	})
	if err != nil {
		sendServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, h.info(sess))
}

func (h *Handler) ListSessions(c *gin.Context) {
	active := c.Query("active") == "true"
	sessions := h.svc.ListSessions(active)
	out := make([]SessionInfo, len(sessions))
	for i := range sessions {
		out[i] = h.info(&sessions[i])
	}
	c.JSON(http.StatusOK, SessionListResponse{Sessions: out})
}

func (h *Handler) GetSession(c *gin.Context) {
	sess, err := h.svc.GetSession(c.Param("session_id"))
	if err != nil {
		sendServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.info(sess))
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
	c.JSON(http.StatusOK, h.info(sess))
}

// info is sessionToInfo plus what only the live service knows.
func (h *Handler) info(sess *session.Session) SessionInfo {
	out := sessionToInfo(sess)
	out.AwaitingInput = h.svc.AwaitingInput(sess.ID)
	return out
}

func (h *Handler) Handoff(c *gin.Context) {
	cmd, err := h.svc.Handoff(c.Param("session_id"))
	if err != nil {
		sendServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, HandoffResponse{Command: cmd})
}

func (h *Handler) Status(c *gin.Context) {
	sess, err := h.svc.GetSession(c.Param("session_id"))
	if err != nil {
		sendServiceError(c, err)
		return
	}
	resp := SessionStatusResponse{Scenario: desk.Scenario(sess.Profile), Quota: []QuotaSegmentInfo{}}
	resp.RequestedModel = h.svc.TierModel(c.Request.Context(), sess)
	if resp.RequestedModel == "" {
		resp.RequestedModel = h.svc.RequestedModel(sess.ID)
	}
	if h.routes != nil && resp.RequestedModel != "" {
		if route := h.routes.ResolveRoute(c.Request.Context(), resp.Scenario, resp.RequestedModel); route != nil {
			resp.ProviderName, resp.ProviderModel = route.ProviderName, route.Model
			for _, q := range route.Quota {
				resp.Quota = append(resp.Quota, QuotaSegmentInfo{
					Type: q.Type, Balance: q.Balance, Text: q.Text,
					UsedPercent: q.UsedPercent, ResetsAt: q.ResetsAt, LimitReached: q.LimitReached,
				})
			}
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) SetModel(c *gin.Context) {
	var req SetModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	sess, err := h.svc.SetModel(c.Request.Context(), c.Param("session_id"), req.Model)
	if err != nil {
		sendServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.info(sess))
}

func (h *Handler) SetProfile(c *gin.Context) {
	var req SetProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	sess, err := h.svc.SetProfile(c.Request.Context(), c.Param("session_id"), req.Profile)
	if err != nil {
		sendServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.info(sess))
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
	c.JSON(http.StatusOK, h.info(sess))
}
