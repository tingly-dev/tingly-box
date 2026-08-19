package team

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/db"
)

type Handler struct {
	store *db.TeamStore
}

func NewHandler(store *db.TeamStore) *Handler {
	return &Handler{store: store}
}

func sendError(c *gin.Context, status int, err error, errType string) {
	c.JSON(status, gin.H{"error": gin.H{"message": err.Error(), "type": errType}})
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
		sendError(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	record, err := h.store.Create(req.Name, req.Slug)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			status = http.StatusConflict
		}
		sendError(c, status, err, "invalid_request_error")
		return
	}
	c.JSON(http.StatusCreated, recordToInfo(record))
}

func (h *Handler) Update(c *gin.Context) {
	id := c.Param("team_id")
	if id == "" {
		sendError(c, http.StatusBadRequest, errors.New("team_id is required"), "invalid_request_error")
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}
	record, err := h.store.Update(id, req.Name, req.Slug)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(strings.ToLower(err.Error()), "unique") {
			status = http.StatusConflict
		}
		sendError(c, status, err, "invalid_request_error")
		return
	}
	c.JSON(http.StatusOK, recordToInfo(record))
}

func (h *Handler) Enable(c *gin.Context)  { h.setEnabled(c, true) }
func (h *Handler) Disable(c *gin.Context) { h.setEnabled(c, false) }

func (h *Handler) setEnabled(c *gin.Context, enabled bool) {
	id := c.Param("team_id")
	if err := h.store.SetEnabled(id, enabled); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		sendError(c, status, err, "invalid_request_error")
		return
	}
	record, _ := h.store.Get(id)
	c.JSON(http.StatusOK, recordToInfo(record))
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("team_id")
	if err := h.store.Delete(id); err != nil {
		status := http.StatusConflict
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		sendError(c, status, err, "conflict_error")
		return
	}
	c.Status(http.StatusNoContent)
}
