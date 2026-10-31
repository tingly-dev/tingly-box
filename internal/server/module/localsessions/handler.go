package localsessions

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	reader "github.com/tingly-dev/tingly-box/internal/localsessions"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type LocalSessionStatus struct {
	LocalAccess  bool `json:"local_access"`
	LocalBinding bool `json:"local_binding"`
	Enabled      bool `json:"enabled"`
}
type LocalSessionStatusResponse struct {
	Success bool               `json:"success"`
	Data    LocalSessionStatus `json:"data"`
}
type LocalSessionListResponse struct {
	Success bool           `json:"success"`
	Data    reader.Listing `json:"data"`
}
type LocalSessionPreviewResponse struct {
	Success bool        `json:"success"`
	Data    reader.Page `json:"data"`
}

type Handler struct {
	enabled      func() bool
	localBinding bool
	root         string
	once         sync.Once
	store        *reader.Store
	initErr      error
	jobs         chan struct{}
}

func NewHandler(cfg *config.Config, host string) *Handler {
	h := newHandler("", func() bool {
		return cfg != nil && cfg.GetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionLocalSessions)
	})
	h.localBinding = loopback(strings.ToLower(host))
	return h
}

func newHandler(root string, enabled func() bool) *Handler {
	return &Handler{root: root, enabled: enabled, jobs: make(chan struct{}, 4), localBinding: true}
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Use the actual socket peer, never Gin.ClientIP or forwarded headers. A
// loopback Host requirement also rejects ordinary reverse-proxy publication.
func localRequest(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !loopback(peer) {
		return false
	}
	host := r.Host
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if !loopback(strings.ToLower(host)) {
		return false
	}
	for name := range r.Header {
		lower := strings.ToLower(name)
		if lower == "forwarded" || strings.HasPrefix(lower, "x-forwarded-") || lower == "x-real-ip" {
			return false
		}
	}
	origin := r.Header.Get("Origin")
	if origin == "wails://localhost" || origin == "http://wails.localhost" {
		return true
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, r.Host) || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return false
		}
	}
	return true
}
func (h *Handler) GetStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.JSON(http.StatusOK, LocalSessionStatusResponse{Success: true, Data: LocalSessionStatus{LocalAccess: h.localBinding && localRequest(c.Request), LocalBinding: h.localBinding, Enabled: h.enabled()}})
}
func (h *Handler) ready(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if !h.localBinding {
		apierr.Failure(c, http.StatusForbidden, "Session browsing requires a loopback-only server. Start with --host 127.0.0.1.")
		return false
	}
	if !localRequest(c.Request) {
		apierr.Failure(c, http.StatusForbidden, "Local sessions require a direct connection on this computer. Reverse proxies and remote access are not supported.")
		return false
	}
	if !h.enabled() {
		apierr.Failure(c, http.StatusForbidden, "Local session browsing is disabled.")
		return false
	}
	select {
	case h.jobs <- struct{}{}:
	default:
		c.Header("Retry-After", "1")
		apierr.Failure(c, http.StatusTooManyRequests, "Session preview is busy; retry shortly.")
		return false
	}
	h.once.Do(func() { h.store, h.initErr = reader.New(h.root) })
	if h.initErr != nil {
		<-h.jobs
		apierr.Failure(c, http.StatusServiceUnavailable, "Session preview is unavailable.")
		return false
	}
	return true
}
func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "Unable to read session history."
	switch {
	case errors.Is(err, reader.ErrInvalid):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, reader.ErrChanged):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, reader.ErrMissing):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, reader.ErrLimit):
		status, message = http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, message = http.StatusRequestTimeout, "Session request canceled or timed out."
	}
	apierr.Failure(c, status, message)
}
func (h *Handler) ListProjects(c *gin.Context) { h.list(c, "") }
func (h *Handler) ListSessions(c *gin.Context) { h.list(c, c.Param("project")) }
func (h *Handler) list(c *gin.Context, project string) {
	if !h.ready(c) {
		return
	}
	defer func() { <-h.jobs }()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	data, err := h.store.List(ctx, project, c.Query("cursor"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, LocalSessionListResponse{Success: true, Data: data})
}
func (h *Handler) Preview(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	defer func() { <-h.jobs }()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	direction := c.DefaultQuery("direction", "backward")
	data, err := h.store.Preview(ctx, c.Param("session"), c.Query("cursor"), direction)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, LocalSessionPreviewResponse{Success: true, Data: data})
}
