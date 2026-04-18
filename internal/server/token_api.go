package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// TokenCreateRequest represents the request to create a new API token
type TokenCreateRequest struct {
	DisplayName string `json:"display_name" binding:"required"`
}

// TokenCreateResponse represents the response after creating a new API token
type TokenCreateResponse struct {
	Token       string    `json:"token"`
	TokenID     string    `json:"token_id"`
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// APITokenInfo represents information about an API token (without the actual token)
type APITokenInfo struct {
	TokenID     string     `json:"token_id"`
	UserID      string     `json:"user_id"`
	DisplayName string     `json:"display_name"`
	Enabled     bool       `json:"enabled"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"created_by,omitempty"`
}

// TokenListResponse represents the response when listing tokens
type TokenListResponse struct {
	Tokens []APITokenInfo `json:"tokens"`
	Total  int64          `json:"total"`
}

// registerTokenManagementAPI registers the token management API endpoints
func (s *Server) registerTokenManagementAPI(router *gin.RouterGroup) {
	if s.config == nil {
		return
	}

	tokens := router.Group("/tokens")
	{
		// POST /api/v1/tokens - Create a new API token
		tokens.POST("", s.createAPIToken)

		// GET /api/v1/tokens - List all tokens
		tokens.GET("", s.listAPITokens)

		// GET /api/v1/tokens/:token_id - Get a specific token
		tokens.GET("/:token_id", s.getAPIToken)

		// PUT /api/v1/tokens/:token_id/enable - Enable a token
		tokens.PUT("/:token_id/enable", s.enableAPIToken)

		// PUT /api/v1/tokens/:token_id/disable - Disable a token
		tokens.PUT("/:token_id/disable", s.disableAPIToken)

		// POST /api/v1/tokens/:token_id/regenerate - Regenerate a token
		tokens.POST("/:token_id/regenerate", s.regenerateAPIToken)

		// DELETE /api/v1/tokens/:token_id - Delete a token
		tokens.DELETE("/:token_id", s.deleteAPIToken)
	}
}

// getAPITokenStore retrieves the API token store, sending an error response if unavailable.
// Returns (store, true) on success, (nil, false) if an error response was sent.
func (s *Server) getAPITokenStore(c *gin.Context) (*db.APITokenStore, bool) {
	sm := s.config.StoreManager()
	if sm == nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("store manager not available"), "internal_error")
		return nil, false
	}

	store := sm.APIToken()
	if store == nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("API token store not available"), "internal_error")
		return nil, false
	}

	return store, true
}

// recordToAPITokenInfo converts a database record to API response format
func recordToAPITokenInfo(record *db.APITokenRecord) APITokenInfo {
	return APITokenInfo{
		TokenID:     record.TokenID,
		UserID:      record.UserID,
		DisplayName: record.DisplayName,
		Enabled:     record.Enabled,
		LastUsedAt:  record.LastUsedAt,
		CreatedAt:   record.CreatedAt,
		CreatedBy:   record.CreatedBy,
	}
}

// generateRandomToken generates a random API token string
func generateRandomToken() (string, error) {
	bytes := make([]byte, 24) // 24 bytes = 48 hex chars
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// createAPIToken creates a new API token
// POST /api/v1/tokens
func (s *Server) createAPIToken(c *gin.Context) {
	var req TokenCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendErrorResponse(c, http.StatusBadRequest, err, "invalid_request_error")
		return
	}

	apiTokenStore, ok := s.getAPITokenStore(c)
	if !ok {
		return
	}

	// Generate a new user_id for this token
	// Each token gets its own unique user_id for data isolation
	userUUID := uuid.New().String()

	// Generate random API token string
	randomToken, err := generateRandomToken()
	if err != nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("failed to generate token: "+err.Error()), "internal_error")
		return
	}

	// Add prefix to identify this as a shared API token
	tokenString := "tb-share-" + randomToken

	// Create token record with the random token as TokenID (no expiration)
	record, err := apiTokenStore.CreateTokenWithTokenID(userUUID, tokenString, req.DisplayName, "admin", nil)
	if err != nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("failed to create token: "+err.Error()), "internal_error")
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

// listAPITokens lists all API tokens
// GET /api/v1/tokens
func (s *Server) listAPITokens(c *gin.Context) {
	apiTokenStore, ok := s.getAPITokenStore(c)
	if !ok {
		return
	}

	// Parse query parameters
	userUUID := c.Query("user_id")
	var enabled *bool
	if enabledStr := c.Query("enabled"); enabledStr != "" {
		if enabledBool, err := strconv.ParseBool(enabledStr); err == nil {
			enabled = &enabledBool
		}
	}

	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// List tokens
	records, total, err := apiTokenStore.ListTokens(userUUID, enabled, limit, offset)
	if err != nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("failed to list tokens: "+err.Error()), "internal_error")
		return
	}

	// Convert to response format
	tokens := make([]APITokenInfo, len(records))
	for i, record := range records {
		tokens[i] = recordToAPITokenInfo(&record)
	}

	c.JSON(http.StatusOK, TokenListResponse{
		Tokens: tokens,
		Total:  total,
	})
}

// getAPIToken gets a specific token
// GET /api/v1/tokens/:token_id
func (s *Server) getAPIToken(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		SendErrorResponse(c, http.StatusBadRequest, errors.New("token_id is required"), "invalid_request_error")
		return
	}

	apiTokenStore, ok := s.getAPITokenStore(c)
	if !ok {
		return
	}

	record, err := apiTokenStore.GetToken(tokenID)
	if err != nil {
		SendErrorResponse(c, http.StatusNotFound, errors.New("token not found"), "not_found_error")
		return
	}

	c.JSON(http.StatusOK, recordToAPITokenInfo(record))
}

// deleteAPIToken deletes a token
// DELETE /api/v1/tokens/:token_id
func (s *Server) deleteAPIToken(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		SendErrorResponse(c, http.StatusBadRequest, errors.New("token_id is required"), "invalid_request_error")
		return
	}

	apiTokenStore, ok := s.getAPITokenStore(c)
	if !ok {
		return
	}

	if err := apiTokenStore.DeleteToken(tokenID); err != nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("failed to delete token: "+err.Error()), "internal_error")
		return
	}

	c.Status(http.StatusNoContent)
}

// enableAPIToken enables a token
// PUT /api/v1/tokens/:token_id/enable
func (s *Server) enableAPIToken(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		SendErrorResponse(c, http.StatusBadRequest, errors.New("token_id is required"), "invalid_request_error")
		return
	}

	apiTokenStore, ok := s.getAPITokenStore(c)
	if !ok {
		return
	}

	if err := apiTokenStore.SetTokenEnabled(tokenID, true); err != nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("failed to enable token: "+err.Error()), "internal_error")
		return
	}

	c.Status(http.StatusNoContent)
}

// disableAPIToken disables a token
// PUT /api/v1/tokens/:token_id/disable
func (s *Server) disableAPIToken(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		SendErrorResponse(c, http.StatusBadRequest, errors.New("token_id is required"), "invalid_request_error")
		return
	}

	apiTokenStore, ok := s.getAPITokenStore(c)
	if !ok {
		return
	}

	if err := apiTokenStore.SetTokenEnabled(tokenID, false); err != nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("failed to disable token: "+err.Error()), "internal_error")
		return
	}

	c.Status(http.StatusNoContent)
}

// regenerateAPIToken regenerates a token (keeps the same token_id but generates new token string)
// POST /api/v1/tokens/:token_id/regenerate
func (s *Server) regenerateAPIToken(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		SendErrorResponse(c, http.StatusBadRequest, errors.New("token_id is required"), "invalid_request_error")
		return
	}

	apiTokenStore, ok := s.getAPITokenStore(c)
	if !ok {
		return
	}

	// Get existing token record
	record, err := apiTokenStore.GetToken(tokenID)
	if err != nil {
		SendErrorResponse(c, http.StatusNotFound, errors.New("token not found"), "not_found_error")
		return
	}

	// Generate new random API token string
	randomToken, err := generateRandomToken()
	if err != nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("failed to generate token: "+err.Error()), "internal_error")
		return
	}

	// Add prefix
	newTokenString := "tb-share-" + randomToken

	// Update token with new token string (same token_id)
	if err := apiTokenStore.UpdateTokenString(tokenID, newTokenString); err != nil {
		SendErrorResponse(c, http.StatusInternalServerError, errors.New("failed to regenerate token: "+err.Error()), "internal_error")
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
