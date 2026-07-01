// Package apitoken implements CRUD HTTP endpoints for shared API tokens.
// It is intentionally free of any internal/server import — all error
// responses are written inline so the package has no circular dependency.
package sharing

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// --- handler ----------------------------------------------------------------

// Handler handles API-token HTTP requests.
type Handler struct {
	store *db.APITokenStore
}

// NewHandler creates a Handler backed by the given store.
func NewHandler(store *db.APITokenStore) *Handler {
	return &Handler{store: store}
}

// --- helpers ----------------------------------------------------------------

func sendError(c *gin.Context, status int, err error, errType string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": err.Error(),
			"type":    errType,
		},
	})
}

func recordToInfo(r *db.APITokenRecord) APITokenInfo {
	return APITokenInfo{
		TokenID:     r.TokenID,
		UserID:      r.UserID,
		DisplayName: r.DisplayName,
		Enabled:     r.Enabled,
		LastUsedAt:  r.LastUsedAt,
		CreatedAt:   r.CreatedAt,
		CreatedBy:   r.CreatedBy,
	}
}

func generateRandomToken() (string, error) {
	b := make([]byte, 24) // 24 bytes → 48 hex chars
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- handlers ---------------------------------------------------------------

// Create handles POST /tokens — creates a new shared API token.
func (h *Handler) Create(c *gin.Context) {
	var req TokenCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}

	// Each token gets its own unique user_id for data isolation.
	userUUID := uuid.New().String()

	randomToken, err := generateRandomToken()
	if err != nil {
		sendError(c, http.StatusInternalServerError, errors.New("failed to generate token: "+err.Error()), "internal_error")
		return
	}
	tokenString := "tb-share-" + randomToken

	record, err := h.store.CreateTokenWithTokenID(userUUID, tokenString, req.DisplayName, "admin", nil)
	if err != nil {
		sendError(c, http.StatusInternalServerError, errors.New("failed to create token: "+err.Error()), "internal_error")
		return
	}

	c.JSON(http.StatusCreated, TokenCreateResponse{
		Token:       tokenString,
		TokenID:     record.TokenID,
		UserID:      record.UserID,
		DisplayName: record.DisplayName,
		CreatedAt:   record.CreatedAt,
	})
}

// List handles GET /tokens — lists tokens with optional filters.
func (h *Handler) List(c *gin.Context) {
	userUUID := c.Query("user_id")

	var enabled *bool
	if s := c.Query("enabled"); s != "" {
		if b, err := strconv.ParseBool(s); err == nil {
			enabled = &b
		}
	}

	limit := 100
	if s := c.Query("limit"); s != "" {
		if l, err := strconv.Atoi(s); err == nil && l > 0 {
			limit = l
		}
	}

	offset := 0
	if s := c.Query("offset"); s != "" {
		if o, err := strconv.Atoi(s); err == nil && o >= 0 {
			offset = o
		}
	}

	records, total, err := h.store.ListTokens(userUUID, enabled, limit, offset)
	if err != nil {
		sendError(c, http.StatusInternalServerError, errors.New("failed to list tokens: "+err.Error()), "internal_error")
		return
	}

	tokens := make([]APITokenInfo, len(records))
	for i := range records {
		tokens[i] = recordToInfo(&records[i])
	}
	c.JSON(http.StatusOK, TokenListResponse{Tokens: tokens, Total: total})
}

// Get handles GET /tokens/:token_id.
func (h *Handler) Get(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		sendError(c, http.StatusBadRequest, errors.New("token_id is required"), "invalid_request_error")
		return
	}

	record, err := h.store.GetToken(tokenID)
	if err != nil {
		sendError(c, http.StatusNotFound, errors.New("token not found"), "not_found_error")
		return
	}

	c.JSON(http.StatusOK, recordToInfo(record))
}

// Delete handles DELETE /tokens/:token_id.
func (h *Handler) Delete(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		sendError(c, http.StatusBadRequest, errors.New("token_id is required"), "invalid_request_error")
		return
	}

	if err := h.store.DeleteToken(tokenID); err != nil {
		sendError(c, http.StatusInternalServerError, errors.New("failed to delete token: "+err.Error()), "internal_error")
		return
	}
	c.Status(http.StatusNoContent)
}

// Enable handles PUT /tokens/:token_id/enable.
func (h *Handler) Enable(c *gin.Context) {
	h.setEnabled(c, true)
}

// Disable handles PUT /tokens/:token_id/disable.
func (h *Handler) Disable(c *gin.Context) {
	h.setEnabled(c, false)
}

func (h *Handler) setEnabled(c *gin.Context, enabled bool) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		sendError(c, http.StatusBadRequest, errors.New("token_id is required"), "invalid_request_error")
		return
	}

	action := "disable"
	if enabled {
		action = "enable"
	}
	if err := h.store.SetTokenEnabled(tokenID, enabled); err != nil {
		sendError(c, http.StatusInternalServerError, errors.New("failed to "+action+" token: "+err.Error()), "internal_error")
		return
	}
	c.Status(http.StatusNoContent)
}

// Regenerate handles POST /tokens/:token_id/regenerate — keeps the same
// token_id but issues a new token string.
func (h *Handler) Regenerate(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		sendError(c, http.StatusBadRequest, errors.New("token_id is required"), "invalid_request_error")
		return
	}

	record, err := h.store.GetToken(tokenID)
	if err != nil {
		sendError(c, http.StatusNotFound, errors.New("token not found"), "not_found_error")
		return
	}

	randomToken, err := generateRandomToken()
	if err != nil {
		sendError(c, http.StatusInternalServerError, errors.New("failed to generate token: "+err.Error()), "internal_error")
		return
	}
	newTokenString := "tb-share-" + randomToken

	if err := h.store.UpdateTokenString(tokenID, newTokenString); err != nil {
		sendError(c, http.StatusInternalServerError, errors.New("failed to regenerate token: "+err.Error()), "internal_error")
		return
	}

	c.JSON(http.StatusOK, TokenCreateResponse{
		Token:       newTokenString,
		TokenID:     record.TokenID,
		UserID:      record.UserID,
		DisplayName: record.DisplayName,
		CreatedAt:   record.CreatedAt,
	})
}
