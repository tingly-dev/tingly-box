// Package github exposes the /api/v1/github endpoints and the browser /
// agent callbacks of the GitHub integration (.design/github.md).
package github

import (
	"errors"
	"html"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	tbgithub "github.com/tingly-dev/tingly-box/internal/github"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
)

type Handler struct {
	svc     *tbgithub.Service
	enabled func() bool
}

func NewHandler(svc *tbgithub.Service) *Handler { return &Handler{svc: svc} }

// WithGate sets the feature-flag predicate every route is gated on.
func (h *Handler) WithGate(enabled func() bool) *Handler {
	h.enabled = enabled
	return h
}

func (h *Handler) available(c *gin.Context) bool {
	if h.svc == nil {
		apierr.Send(c, http.StatusServiceUnavailable, errors.New("github integration is unavailable"), "service_unavailable")
		return false
	}
	return true
}

func (h *Handler) Status(c *gin.Context) {
	if !h.available(c) {
		return
	}
	st, err := h.svc.Status(c.Request.Context(), c.Query("live") == "true")
	if err != nil {
		apierr.Send(c, http.StatusInternalServerError, err, "internal_error")
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *Handler) Connect(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var req ConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	plan, err := h.svc.StartConnect(req.OwnerType, req.Org, req.BaseURL)
	if err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	c.JSON(http.StatusOK, plan)
}

// Sync refreshes the installations from GitHub (after the user changed the
// App's repositories on github.com).
func (h *Handler) Sync(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.svc.Sync(c.Request.Context()); err != nil {
		apierr.Send(c, http.StatusBadGateway, err, "upstream_error")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) Disconnect(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.svc.Disconnect(); err != nil {
		apierr.Send(c, http.StatusInternalServerError, err, "internal_error")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) SetPAT(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var req PATRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	if err := h.svc.SetPAT(req.Token); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) ClearPAT(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.svc.ClearPAT(); err != nil {
		apierr.Send(c, http.StatusInternalServerError, err, "internal_error")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

// settingsPath is where the browser lands when a flow ends.
const settingsPath = "/system/experimental"

// ManifestCallback is GitHub's redirect after the user confirmed the App. It
// is unauthenticated by necessity (a browser redirect from github.com); the
// one-time state from StartConnect is what authorizes it.
func (h *Handler) ManifestCallback(c *gin.Context) {
	if !h.available(c) {
		return
	}
	installURL, err := h.svc.FinishManifest(c.Request.Context(), c.Query("code"), c.Query("state"))
	if err != nil {
		logrus.WithError(err).Warn("github: manifest callback")
		failPage(c, err)
		return
	}
	c.Redirect(http.StatusFound, installURL)
}

// InstallCallback is GitHub's redirect after the user installed the App.
func (h *Handler) InstallCallback(c *gin.Context) {
	if !h.available(c) {
		return
	}
	id, _ := strconv.ParseInt(c.Query("installation_id"), 10, 64)
	back, err := h.svc.FinishInstall(c.Request.Context(), id, c.Query("state"))
	if err != nil {
		logrus.WithError(err).Warn("github: install callback")
		failPage(c, err)
		return
	}
	c.Redirect(http.StatusFound, back+settingsPath)
}

func failPage(c *gin.Context, err error) {
	c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(
		"<!doctype html><meta charset=utf-8><title>GitHub</title><body style=\"font-family:system-ui;max-width:32rem;margin:4rem auto\">"+
			"<h1>Could not connect GitHub</h1><p>"+html.EscapeString(err.Error())+"</p>"+
			"<p><a href=\""+settingsPath+"\">Back to settings</a></p></body>"))
}
