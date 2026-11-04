package team

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
)

type Handler struct {
	store *db.TeamStore
	// config owns a team's routing ("team:<id>" rules): Delete removes it
	// with the team, Duplicate copies it. Nil only for schema generation and
	// tests of the store-only endpoints.
	config *config.Config
}

func NewHandler(store *db.TeamStore, cfg *config.Config) *Handler {
	return &Handler{store: store, config: cfg}
}

// sendStoreError maps a TeamStore error to an HTTP response. All four
// mutating handlers below share the same "not found" / "already exists"
// substring inference and differ only in their default status/errType.
func sendStoreError(c *gin.Context, err error, defaultStatus int, errType string) {
	status := defaultStatus
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		status = http.StatusNotFound
	case strings.Contains(msg, "unique") || strings.Contains(msg, "already exists"):
		status = http.StatusConflict
	}
	apierr.Send(c, status, err, errType)
}

func recordToInfo(record *db.TeamRecord) TeamInfo {
	return TeamInfo{
		ID: record.ID, Name: record.Name, Slug: record.Slug,
		Enabled: record.Enabled, IsDefault: record.ID == db.DefaultTeamID,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func (h *Handler) List(c *gin.Context) {
	records := h.store.List()
	teams := make([]TeamInfo, len(records))
	for i := range records {
		teams[i] = recordToInfo(&records[i])
	}
	c.JSON(http.StatusOK, ListResponse{Teams: teams})
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	record, err := h.store.Create(req.Name)
	if err != nil {
		sendStoreError(c, err, http.StatusBadRequest, "invalid_request_error")
		return
	}
	c.JSON(http.StatusCreated, recordToInfo(record))
}

// Duplicate creates a team as a copy of :team_id: same routing, no sharing
// keys. The name is optional.
func (h *Handler) Duplicate(c *gin.Context) {
	if h.config == nil {
		apierr.Send(c, http.StatusInternalServerError, errors.New("config not available"), "api_error")
		return
	}
	var req TeamDuplicateRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	record, err := h.config.DuplicateTeam(c.Param("team_id"), req.Name)
	if err != nil {
		sendStoreError(c, err, http.StatusBadRequest, "invalid_request_error")
		return
	}
	c.JSON(http.StatusCreated, recordToInfo(record))
}

func (h *Handler) Update(c *gin.Context) {
	id := c.Param("team_id")
	if id == "" {
		apierr.Send(c, http.StatusBadRequest, errors.New("team_id is required"), "invalid_request_error")
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Send(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	record, err := h.store.Update(id, req.Name)
	if err != nil {
		sendStoreError(c, err, http.StatusBadRequest, "invalid_request_error")
		return
	}
	c.JSON(http.StatusOK, recordToInfo(record))
}

func (h *Handler) Enable(c *gin.Context)  { h.setEnabled(c, true) }
func (h *Handler) Disable(c *gin.Context) { h.setEnabled(c, false) }

func (h *Handler) setEnabled(c *gin.Context, enabled bool) {
	id := c.Param("team_id")
	if err := h.store.SetEnabled(id, enabled); err != nil {
		sendStoreError(c, err, http.StatusBadRequest, "invalid_request_error")
		return
	}
	record, _ := h.store.Get(id)
	c.JSON(http.StatusOK, recordToInfo(record))
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("team_id")
	deleteTeam := h.store.Delete
	if h.config != nil {
		deleteTeam = h.config.DeleteTeam
	}
	if err := deleteTeam(id); err != nil {
		sendStoreError(c, err, http.StatusConflict, "conflict_error")
		return
	}
	c.Status(http.StatusNoContent)
}
