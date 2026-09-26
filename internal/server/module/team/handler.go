package team

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
)

type Handler struct {
	store *db.TeamStore
}

func NewHandler(store *db.TeamStore) *Handler {
	return &Handler{store: store}
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
		Enabled: record.Enabled, QuotaVisible: record.QuotaVisible,
		IsDefault: record.ID == db.DefaultTeamID,
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
	if req.QuotaVisible != nil {
		if err := h.store.SetQuotaVisible(id, *req.QuotaVisible); err != nil {
			sendStoreError(c, err, http.StatusBadRequest, "invalid_request_error")
			return
		}
		record, _ = h.store.Get(id)
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
	if err := h.store.Delete(id); err != nil {
		sendStoreError(c, err, http.StatusConflict, "conflict_error")
		return
	}
	c.Status(http.StatusNoContent)
}
