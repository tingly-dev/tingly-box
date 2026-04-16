package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// TokenCreateRequest represents the request to create a new API token
type TokenCreateRequest struct {
	UserUUID      string `json:"user_uuid" binding:"required"`
	DisplayName   string `json:"display_name" binding:"required"`
	ExpiresInDays int    `json:"expires_in_days"` // Optional, defaults to config default
}

// TokenCreateResponse represents the response after creating a new API token
type TokenCreateResponse struct {
	Token      string    `json:"token"`
	TokenID    string    `json:"token_id"`
	UserUUID   string    `json:"user_uuid"`
	DisplayName string   `json:"display_name"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// APITokenInfo represents information about an API token (without the actual token)
type APITokenInfo struct {
	TokenID     string     `json:"token_id"`
	UserUUID    string     `json:"user_uuid"`
	DisplayName string     `json:"display_name"`
	Enabled     bool       `json:"enabled"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"created_by,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// TokenListResponse represents the response when listing tokens
type TokenListResponse struct {
	Tokens []APITokenInfo `json:"tokens"`
	Total  int64          `json:"total"`
}

// TokenRevokeRequest represents the request to revoke a token
type TokenRevokeRequest struct {
	Reason string `json:"reason"`
}

// registerTokenManagementAPI registers the token management API endpoints
func (s *Server) registerTokenManagementAPI(router *gin.RouterGroup) {
	if s.config == nil {
		return
	}

	// Check if multi-tenant is enabled
	if !s.config.IsMultiTenantEnabled() {
		return
	}

	tokens := router.Group("/tokens")
	{
		// POST /api/v1/tokens - Create a new API token
		tokens.POST("", s.createAPIToken)

		// GET /api/v1/tokens - List all tokens (admin only)
		tokens.GET("", s.listAPITokens)

		// GET /api/v1/tokens/:token_id - Get a specific token
		tokens.GET("/:token_id", s.getAPIToken)

		// DELETE /api/v1/tokens/:token_id - Revoke a token
		tokens.DELETE("/:token_id", s.revokeAPIToken)
	}
}

// createAPIToken creates a new API token
// POST /api/v1/tokens
func (s *Server) createAPIToken(c *gin.Context) {
	var req TokenCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "Invalid request body: " + err.Error(),
			},
		})
		return
	}

	sm := s.config.StoreManager()
	if sm == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "Store manager not available",
			},
		})
		return
	}

	apiTokenStore := sm.APIToken()
	if apiTokenStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "API token store not available",
			},
		})
		return
	}

	// Calculate expiration
	var expiresAt *time.Time
	if req.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, req.ExpiresInDays)
		expiresAt = &t
	} else {
		// Use default TTL from config
		t := time.Now().AddDate(0, 0, s.config.GetAPITokenDefaultTTL())
		expiresAt = &t
	}

	// Create token record
	record, err := apiTokenStore.CreateToken(req.UserUUID, req.DisplayName, "admin", expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "Failed to create token: " + err.Error(),
			},
		})
		return
	}

	// Generate JWT token
	apiTokenManager, err := auth.NewAPITokenManager(auth.APITokenManagerConfig{
		SecretKey:     s.config.GetAPITokenSecret(),
		SigningMethod: s.config.GetAPITokenAlgorithm(),
		Issuer:        s.config.GetAPITokenIssuer(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "Failed to create token manager: " + err.Error(),
			},
		})
		return
	}

	tokenString, err := apiTokenManager.GenerateToken(record.UserUUID, record.TokenID, *expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "Failed to generate token: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, TokenCreateResponse{
		Token:      tokenString,
		TokenID:    record.TokenID,
		UserUUID:   record.UserUUID,
		DisplayName: record.DisplayName,
		ExpiresAt:  *expiresAt,
		CreatedAt:  record.CreatedAt,
	})
}

// listAPITokens lists all API tokens
// GET /api/v1/tokens
func (s *Server) listAPITokens(c *gin.Context) {
	sm := s.config.StoreManager()
	if sm == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "Store manager not available",
			},
		})
		return
	}

	apiTokenStore := sm.APIToken()
	if apiTokenStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "API token store not available",
			},
		})
		return
	}

	// Parse query parameters
	userUUID := c.Query("user_uuid")
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "Failed to list tokens: " + err.Error(),
			},
		})
		return
	}

	// Convert to response format
	tokens := make([]APITokenInfo, len(records))
	for i, record := range records {
		tokens[i] = APITokenInfo{
			TokenID:     record.TokenID,
			UserUUID:    record.UserUUID,
			DisplayName: record.DisplayName,
			Enabled:     record.Enabled,
			ExpiresAt:   record.ExpiresAt,
			LastUsedAt:  record.LastUsedAt,
			CreatedAt:   record.CreatedAt,
			CreatedBy:   record.CreatedBy,
			RevokedAt:   record.RevokedAt,
		}
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "token_id is required",
			},
		})
		return
	}

	sm := s.config.StoreManager()
	if sm == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "Store manager not available",
			},
		})
		return
	}

	apiTokenStore := sm.APIToken()
	if apiTokenStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "API token store not available",
			},
		})
		return
	}

	record, err := apiTokenStore.GetToken(tokenID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":    "not_found_error",
				"message": "Token not found",
			},
		})
		return
	}

	c.JSON(http.StatusOK, APITokenInfo{
		TokenID:     record.TokenID,
		UserUUID:    record.UserUUID,
		DisplayName: record.DisplayName,
		Enabled:     record.Enabled,
		ExpiresAt:   record.ExpiresAt,
		LastUsedAt:  record.LastUsedAt,
		CreatedAt:   record.CreatedAt,
		CreatedBy:   record.CreatedBy,
		RevokedAt:   record.RevokedAt,
	})
}

// revokeAPIToken revokes a token
// DELETE /api/v1/tokens/:token_id
func (s *Server) revokeAPIToken(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "token_id is required",
			},
		})
		return
	}

	// Parse request body for optional reason
	var req TokenRevokeRequest
	c.ShouldBindJSON(&req)

	sm := s.config.StoreManager()
	if sm == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "Store manager not available",
			},
		})
		return
	}

	apiTokenStore := sm.APIToken()
	if apiTokenStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "API token store not available",
			},
		})
		return
	}

	if err := apiTokenStore.RevokeToken(tokenID, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "internal_error",
				"message": "Failed to revoke token: " + err.Error(),
			},
		})
		return
	}

	c.Status(http.StatusNoContent)
}
