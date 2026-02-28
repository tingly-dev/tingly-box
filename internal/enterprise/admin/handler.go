package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/enterprise/auth"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
	"github.com/tingly-dev/tingly-box/internal/enterprise/rbac"
	"github.com/tingly-dev/tingly-box/internal/enterprise/token"
	"github.com/tingly-dev/tingly-box/internal/enterprise/user"
)

// Handler handles admin HTTP requests
type Handler struct {
	authSvc    *auth.AuthService
	userSvc    user.Service
	tokenSvc   token.Service
	auditRepo  db.AuditLogRepository
}

// NewHandler creates a new admin handler
func NewHandler(
	authSvc *auth.AuthService,
	userSvc user.Service,
	tokenSvc token.Service,
	auditRepo db.AuditLogRepository,
) *Handler {
	return &Handler{
		authSvc:   authSvc,
		userSvc:   userSvc,
		tokenSvc:  tokenSvc,
		auditRepo: auditRepo,
	}
}

// RegisterRoutes registers admin routes
func (h *Handler) RegisterRoutes(router gin.IRouter, authMiddleware gin.HandlerFunc) {
	auth := router.Group("/auth")
	{
		auth.POST("/login", h.Login)
		auth.POST("/refresh", h.RefreshToken)
		auth.POST("/logout", authMiddleware, h.Logout)
		auth.GET("/me", authMiddleware, h.GetCurrentUser)
	}

	users := router.Group("/users")
	users.Use(authMiddleware, rbac.RequireRole(db.RoleAdmin))
	{
		users.GET("", h.ListUsers)
		users.POST("", h.CreateUser)
		users.GET("/:id", h.GetUser)
		users.PUT("/:id", h.UpdateUser)
		users.DELETE("/:id", h.DeleteUser)
		users.POST("/:id/activate", h.ActivateUser)
		users.POST("/:id/deactivate", h.DeactivateUser)
		users.POST("/:id/password", h.ResetPassword)
	}

	tokens := router.Group("/tokens")
	tokens.Use(authMiddleware)
	{
		tokens.GET("", h.ListTokens)
		tokens.POST("", h.CreateToken)
		tokens.GET("/:id", h.GetToken)
		tokens.PUT("/:id", h.UpdateToken)
		tokens.DELETE("/:id", h.DeleteToken)
	}

	myTokens := router.Group("/my-tokens")
	myTokens.Use(authMiddleware)
	{
		myTokens.GET("", h.ListMyTokens)
		myTokens.POST("", h.CreateMyToken)
		myTokens.DELETE("/:uuid", h.DeleteMyToken)
	}

	audit := router.Group("/audit")
	audit.Use(authMiddleware, rbac.RequireRole(db.RoleAdmin))
	{
		audit.GET("", h.ListAuditLogs)
		audit.GET("/:id", h.GetAuditLog)
	}

	stats := router.Group("/stats")
	stats.Use(authMiddleware, rbac.RequireRole(db.RoleAdmin))
	{
		stats.GET("", h.GetStats)
	}
}

// Login handles user login
func (h *Handler) Login(c *gin.Context) {
	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	resp, err := h.authSvc.Login(c.Request.Context(), req, ipAddress, userAgent)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}
		if errors.Is(err, auth.ErrUserInactive) {
			c.JSON(http.StatusForbidden, gin.H{"error": "user account is inactive"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authentication failed"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RefreshToken handles token refresh
func (h *Handler) RefreshToken(c *gin.Context) {
	var req auth.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	resp, err := h.authSvc.RefreshToken(c.Request.Context(), req, ipAddress, userAgent)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Logout handles user logout
func (h *Handler) Logout(c *gin.Context) {
	currentUser := rbac.GetCurrentUser(c)
	if currentUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// For JWT-based sessions, we don't have a session ID to invalidate
	// The client should simply discard the token
	// For session-based auth, you would invalidate the session here

	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

// GetCurrentUser returns the current authenticated user
func (h *Handler) GetCurrentUser(c *gin.Context) {
	user := rbac.GetCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// ListUsers returns a paginated list of users
func (h *Handler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	resp, err := h.userSvc.ListUsers(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// CreateUser creates a new user
func (h *Handler) CreateUser(c *gin.Context) {
	var req user.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	actor := rbac.GetCurrentUser(c)

	newUser, err := h.userSvc.CreateUser(c.Request.Context(), req, actor)
	if err != nil {
		if errors.Is(err, user.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "username or email already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, newUser)
}

// GetUser returns a user by ID
func (h *Handler) GetUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	user, err := h.userSvc.GetUser(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateUser updates a user
func (h *Handler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req user.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	actor := rbac.GetCurrentUser(c)

	updatedUser, err := h.userSvc.UpdateUser(c.Request.Context(), id, req, actor)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, updatedUser)
}

// DeleteUser deletes a user
func (h *Handler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	actor := rbac.GetCurrentUser(c)

	if err := h.userSvc.DeleteUser(c.Request.Context(), id, actor); err != nil {
		if errors.Is(err, user.ErrSelfAction) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete yourself"})
			return
		}
		if errors.Is(err, user.ErrLastAdmin) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the last admin user"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted successfully"})
}

// ActivateUser activates a user account
func (h *Handler) ActivateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	actor := rbac.GetCurrentUser(c)

	if err := h.userSvc.ActivateUser(c.Request.Context(), id, actor); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user activated successfully"})
}

// DeactivateUser deactivates a user account
func (h *Handler) DeactivateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	actor := rbac.GetCurrentUser(c)

	if err := h.userSvc.DeactivateUser(c.Request.Context(), id, actor); err != nil {
		if errors.Is(err, user.ErrSelfAction) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot deactivate yourself"})
			return
		}
		if errors.Is(err, user.ErrLastAdmin) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot deactivate the last admin user"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to deactivate user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deactivated successfully"})
}

// ResetPassword resets a user's password
func (h *Handler) ResetPassword(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	actor := rbac.GetCurrentUser(c)

	newPassword, err := h.userSvc.ResetPassword(c.Request.Context(), id, actor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "password reset successfully",
		"new_password":  newPassword,
		"should_change": true,
	})
}

// ListTokens returns a paginated list of all tokens (admin only)
func (h *Handler) ListTokens(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	resp, err := h.tokenSvc.ListTokens(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tokens"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// CreateToken creates a new API token
func (h *Handler) CreateToken(c *gin.Context) {
	var req token.CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	actor := rbac.GetCurrentUser(c)

	apiToken, rawToken, err := h.tokenSvc.CreateToken(c.Request.Context(), req, actor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create token"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"token":       rawToken,
		"token_prefix": apiToken.TokenPrefix,
		"token_id":    apiToken.UUID,
		"name":        apiToken.Name,
		"scopes":      apiToken.Scopes,
		"expires_at":  apiToken.ExpiresAt,
	})
}

// GetToken returns a token by ID
func (h *Handler) GetToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token ID"})
		return
	}

	apiToken, err := h.tokenSvc.GetToken(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}

	c.JSON(http.StatusOK, apiToken)
}

// UpdateToken updates a token
func (h *Handler) UpdateToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token ID"})
		return
	}

	var req token.UpdateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	actor := rbac.GetCurrentUser(c)

	updatedToken, err := h.tokenSvc.UpdateToken(c.Request.Context(), id, req, actor)
	if err != nil {
		if errors.Is(err, token.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update token"})
		return
	}

	c.JSON(http.StatusOK, updatedToken)
}

// DeleteToken deletes a token
func (h *Handler) DeleteToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token ID"})
		return
	}

	actor := rbac.GetCurrentUser(c)

	if err := h.tokenSvc.DeleteToken(c.Request.Context(), id, actor); err != nil {
		if errors.Is(err, token.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "token deleted successfully"})
}

// ListMyTokens returns the current user's tokens
func (h *Handler) ListMyTokens(c *gin.Context) {
	currentUser := rbac.GetCurrentUser(c)
	if currentUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	resp, err := h.tokenSvc.ListMyTokens(c.Request.Context(), currentUser, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tokens"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// CreateMyToken creates a new API token for the current user
func (h *Handler) CreateMyToken(c *gin.Context) {
	currentUser := rbac.GetCurrentUser(c)
	if currentUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req token.CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Force UserID to current user
	req.UserID = &currentUser.ID

	apiToken, rawToken, err := h.tokenSvc.CreateToken(c.Request.Context(), req, currentUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create token"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"token":       rawToken,
		"token_prefix": apiToken.TokenPrefix,
		"token_id":    apiToken.UUID,
		"name":        apiToken.Name,
		"scopes":      apiToken.Scopes,
		"expires_at":  apiToken.ExpiresAt,
	})
}

// DeleteMyToken deletes a token by UUID for the current user
func (h *Handler) DeleteMyToken(c *gin.Context) {
	currentUser := rbac.GetCurrentUser(c)
	if currentUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	uuid := c.Param("uuid")

	if err := h.tokenSvc.DeleteTokenByUUID(c.Request.Context(), uuid, currentUser); err != nil {
		if errors.Is(err, token.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "token deleted successfully"})
}

// ListAuditLogs returns a paginated list of audit logs
func (h *Handler) ListAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	offset := (page - 1) * pageSize

	filters := db.AuditLogFilters{}

	// Apply filters
	if userID := c.Query("user_id"); userID != "" {
		if id, err := strconv.ParseInt(userID, 10, 64); err == nil {
			filters.UserID = &id
		}
	}
	filters.Action = c.Query("action")
	filters.ResourceType = c.Query("resource_type")
	filters.Status = c.Query("status")

	if startDate := c.Query("start_date"); startDate != "" {
		if t, err := time.Parse(time.RFC3339, startDate); err == nil {
			filters.StartDate = &t
		}
	}
	if endDate := c.Query("end_date"); endDate != "" {
		if t, err := time.Parse(time.RFC3339, endDate); err == nil {
			filters.EndDate = &t
		}
	}

	logs, total, err := h.auditRepo.List(offset, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list audit logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": total,
		"page":  page,
		"size":  pageSize,
	})
}

// GetAuditLog returns an audit log by ID
func (h *Handler) GetAuditLog(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit log ID"})
		return
	}

	log, err := h.auditRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audit log not found"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// GetStats returns system statistics
func (h *Handler) GetStats(c *gin.Context) {
	// This is a placeholder - implement actual stats gathering
	c.JSON(http.StatusOK, gin.H{
		"total_users":       0,
		"active_users":      0,
		"total_tokens":      0,
		"active_tokens":     0,
		"total_audit_logs":  0,
		"enterprise_enabled": true,
	})
}
