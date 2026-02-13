// Package main provides HTTP handlers for enterprise integration example
package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	enterprise "github.com/tingly-dev/tingly-box/internal/enterprise"
	"github.com/tingly-dev/tingly-box/internal/enterprise/auth"
)

// handleLogin handles user login requests
func handleLogin(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req auth.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Call enterprise login
		ipAddress := c.ClientIP()

		// For demo, we'll directly use the auth service
		// In production, you would use the integration interface

		logrus.WithFields(logrus.Fields{
			"username": req.Username,
			"ip":       ipAddress,
		}).Info("Login attempt")

		c.JSON(http.StatusNotImplemented, gin.H{
			"message": "Login endpoint - implement with integration interface",
			"hint":    "Use enterprise authService.Login() method",
		})
	}
}

// handleProfile returns the current user's profile
func handleProfile(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64("user_id")
		ctx := context.Background()

		userInfo, err := ent.GetUserInfo(ctx, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"user":  userInfo,
			"token": c.GetHeader("Authorization"),
		})
	}
}

// handleChangePassword handles password change requests
func handleChangePassword(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			CurrentPassword string `json:"current_password" binding:"required"`
			NewPassword     string `json:"new_password" binding:"required,min=8"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Implementation would use user service ChangePassword
		c.JSON(http.StatusNotImplemented, gin.H{
			"message": "Password change - implement with user.ChangePassword()",
		})
	}
}

// handleListUsers lists all users (admin only)
func handleListUsers(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

		// For demo, return mock data
		// In production, use userService.ListUsers()
		users := []*enterprise.UserInfo{
			{
				ID:          1,
				UUID:        "admin-uuid",
				Username:    "admin",
				Email:       "admin@tingly-box.local",
				Role:        "admin",
				FullName:    "Default Administrator",
				IsActive:    true,
				CreatedAt:   time.Now().Unix(),
			},
		}

		c.JSON(http.StatusOK, gin.H{
			"users": users,
			"page":  page,
			"size":  pageSize,
			"total": 1,
		})
	}
}

// handleCreateUser creates a new user (admin only)
func handleCreateUser(ent enterprise.Integration) gin.HandlerFunc {
	type CreateUserRequest struct {
		Username string `json:"username" binding:"required,min=3"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
		FullName string `json:"full_name"`
		Role     string `json:"role" binding:"required,oneof=admin user readonly"`
	}

	return func(c *gin.Context) {
		var req CreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		actorID := c.GetInt64("user_id")

		// Validate password strength
		passwordSvc := auth.NewPasswordService(auth.DefaultPasswordConfig())
		if err := passwordSvc.ValidatePasswordStrength(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid password",
				"details": err.Error(),
			})
			return
		}

		// Hash password
		passwordHash, err := passwordSvc.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}

		logrus.WithFields(logrus.Fields{
			"username":  req.Username,
			"email":      req.Email,
			"role":       req.Role,
			"created_by": actorID,
		}).Info("Creating user")

		// In production, use integration interface:
		// userInfo, err := ent.CreateUser(ctx, &enterprise.CreateUserRequest{
		//     Username: req.Username,
		//     Email:    req.Email,
		//     Password: req.Password,
		//     FullName: req.FullName,
		//     Role:     db.Role(req.Role),
		// }, actorID)

		c.JSON(http.StatusNotImplemented, gin.H{
			"message": "User creation - implement with ent.CreateUser()",
			"demo_data": gin.H{
				"username":       req.Username,
				"email":          req.Email,
				"full_name":      req.FullName,
				"role":           req.Role,
				"password_hash": passwordHash[:16] + "...",
			},
		})
	}
}

// handleGetUser retrieves a user by ID (admin only)
func handleGetUser(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		ctx := context.Background()
		userInfo, err := ent.GetUserInfo(ctx, id)
		if err != nil {
			if err == enterprise.ErrUserNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
			return
		}

		c.JSON(http.StatusOK, userInfo)
	}
}

// handleUpdateUser updates a user (admin only)
func handleUpdateUser(ent enterprise.Integration) gin.HandlerFunc {
	type UpdateUserRequest struct {
		FullName *string `json:"full_name"`
		Role     *string `json:"role"`
	}

	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		var req UpdateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		actorID := c.GetInt64("user_id")

		logrus.WithFields(logrus.Fields{
			"target_user_id": id,
			"updated_by":      actorID,
			"updates":         req,
		}).Info("Updating user")

		// In production, use integration interface:
		// userInfo, err := ent.UpdateUser(ctx, id, &enterprise.UpdateUserRequest{
		//     FullName: req.FullName,
		//     Role:     req.Role,
		// }, actorID)

		c.JSON(http.StatusNotImplemented, gin.H{
			"message": "User update - implement with ent.UpdateUser()",
			"user_id":  id,
			"updates": req,
		})
	}
}

// handleDeleteUser deletes a user (admin only)
func handleDeleteUser(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		_ = c.GetInt64("user_id")

		logrus.WithField("user_id", id).Info("Deleting user")

		// In production, use integration interface:
		// err := ent.DeactivateUser(ctx, id, actorID)
		// For deletion, you might need a separate DeleteUser method

		c.JSON(http.StatusNotImplemented, gin.H{
			"message": "User deletion - implement with appropriate service method",
		})
	}
}

// handleActivateUser activates a user account (admin only)
func handleActivateUser(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		_ = c.GetInt64("user_id")

		logrus.WithField("user_id", id).Info("Activating user")

		// In production, implement activation logic
		c.JSON(http.StatusNotImplemented, gin.H{
			"message": "User activation - implement with user service",
		})
	}
}

// handleDeactivateUser deactivates a user account (admin only)
func handleDeactivateUser(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		_ = c.GetInt64("user_id")

		logrus.WithField("user_id", id).Info("Deactivating user")

		c.JSON(http.StatusNotImplemented, gin.H{
			"message": "User deactivation - implement with user service",
		})
	}
}

// handleResetPassword resets a user's password (admin only)
func handleResetPassword(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
			return
		}

		_ = c.GetInt64("user_id")

		logrus.WithField("user_id", id).Info("Resetting password")

		// In production, use integration interface:
		// newPassword, err := ent.ResetPassword(ctx, id, actorID)

		// Generate random password for demo
		passwordSvc := auth.NewPasswordService(auth.DefaultPasswordConfig())
		newPassword, err := passwordSvc.GenerateRandomPassword(16)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate password"})
			return
		}

		logrus.WithField("user_id", id).Info("Password reset (demo)")

		c.JSON(http.StatusOK, gin.H{
			"message":       "Password reset successful",
			"new_password":  newPassword,
			"should_change": true,
			"user_id":       id,
		})
	}
}

// handleListTokens lists all tokens (admin only)
func handleListTokens(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

		// Demo data
		tokens := []*enterprise.TokenWithUser{
			{
				TokenInfo: enterprise.TokenInfo{
					ID:          1,
					UUID:        "token-uuid-1",
					UserID:      1,
					Name:        "Admin API Token",
					TokenPrefix: "ent-abc12345",
					Scopes:      []enterprise.Scope{enterprise.ScopeReadProviders, enterprise.ScopeWriteProviders},
					CreatedAt:   time.Now().Unix(),
				},
				Username: "admin",
				Email:    "admin@tingly-box.local",
			},
		}

		c.JSON(http.StatusOK, gin.H{
			"tokens": tokens,
			"page":   page,
			"size":   pageSize,
			"total":  1,
		})
	}
}

// handleCreateToken creates a new API token (admin only)
func handleCreateToken(ent enterprise.Integration) gin.HandlerFunc {
	type CreateTokenRequest struct {
		Name   string                      `json:"name" binding:"required"`
		Scopes []enterprise.Scope          `json:"scopes" binding:"required,min=1"`
		UserID *int64                      `json:"user_id"`
	}

	return func(c *gin.Context) {
		var req CreateTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		actorID := c.GetInt64("user_id")

		// If no user_id specified, use actor's user_id
		if req.UserID == nil {
			req.UserID = &actorID
		}

		logrus.WithFields(logrus.Fields{
			"name":     req.Name,
			"scopes":   req.Scopes,
			"user_id":  *req.UserID,
			"created_by": actorID,
		}).Info("Creating token")

		// In production, use integration interface:
		// tokenInfo, rawToken, err := ent.CreateAPIToken(ctx, &enterprise.CreateTokenRequest{
		//     Name:   req.Name,
		//     Scopes: req.Scopes,
		//     UserID: req.UserID,
		// }, actorID)

		// For demo, generate a demo token
		demoToken := fmt.Sprintf("ent-%s", generateRandomString(32))

		c.JSON(http.StatusCreated, gin.H{
			"message":    "Token created - implement with ent.CreateAPIToken()",
			"token":      demoToken,
			"token_info": gin.H{
				"name":        req.Name,
				"scopes":      req.Scopes,
				"user_id":     *req.UserID,
				"created_by":  actorID,
			},
		})
	}
}

// handleMyTokens lists current user's tokens
func handleMyTokens(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64("user_id")
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

		ctx := context.Background()
		result, err := ent.ListAPITokens(ctx, userID, page, pageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// handleCreateMyToken creates a new token for current user
func handleCreateMyToken(ent enterprise.Integration) gin.HandlerFunc {
	type CreateTokenRequest struct {
		Name   string                 `json:"name" binding:"required"`
		Scopes []enterprise.Scope       `json:"scopes" binding:"required,min=1"`
	}

	return func(c *gin.Context) {
		var req CreateTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		actorID := c.GetInt64("user_id")

		logrus.WithFields(logrus.Fields{
			"name":   req.Name,
			"scopes": req.Scopes,
			"user_id": actorID,
		}).Info("Creating my token")

		// In production, use integration interface:
		// tokenInfo, rawToken, err := ent.CreateAPIToken(ctx, &enterprise.CreateTokenRequest{
		//     Name:   req.Name,
		//     Scopes: req.Scopes,
		//     UserID: &actorID,
		// }, actorID)

		// For demo, generate a demo token
		demoToken := fmt.Sprintf("ent-%s", generateRandomString(32))

		c.JSON(http.StatusCreated, gin.H{
			"message":    "Token created - implement with ent.CreateAPIToken()",
			"token":      demoToken,
			"token_info": gin.H{
				"name":        req.Name,
				"scopes":      req.Scopes,
				"created_by":   actorID,
			},
		})
	}
}

// handleDeleteMyToken deletes one of current user's tokens
func handleDeleteMyToken(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		uuid := c.Param("uuid")
		actorID := c.GetInt64("user_id")

		logrus.WithFields(logrus.Fields{
			"token_uuid": uuid,
			"user_id":    actorID,
		}).Info("Deleting my token")

		// In production, you would need to get token ID by UUID first
		// tokenID := getIDByUUID(uuid)
		// err := ent.RevokeAPIToken(ctx, tokenID, actorID)

		c.JSON(http.StatusNotImplemented, gin.H{
			"message": "Token deletion - implement with ent.RevokeAPIToken()",
			"token_uuid": uuid,
		})
	}
}

// handleStats returns system statistics (admin only)
func handleStats(ent enterprise.Integration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()

		stats, err := ent.GetStats(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, stats)
	}
}

// handleDemoCreateUser creates a demo user for testing
func handleDemoCreateUser(ent enterprise.Integration) gin.HandlerFunc {
	type DemoUserRequest struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
		Email    string `json:"email" binding:"required,email"`
	}

	return func(c *gin.Context) {
		var req DemoUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		logrus.WithFields(logrus.Fields{
			"demo_username": req.Username,
		}).Info("Demo user creation")

		// Validate password strength
		passwordSvc := auth.NewPasswordService(auth.DefaultPasswordConfig())
		if err := passwordSvc.ValidatePasswordStrength(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Password too weak",
				"details": err.Error(),
			})
			return
		}

		// Hash password
		passwordHash, err := passwordSvc.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}

		// Return demo response
		c.JSON(http.StatusCreated, gin.H{
			"message": "Demo user created (not persisted in database)",
			"user": gin.H{
				"username":      req.Username,
				"email":         req.Email,
				"password_hash": passwordHash[:16] + "...",
				"role":           "user",
				"full_name":      "Demo User",
				"is_active":      true,
			},
			"note": "In production, this would persist to database",
		})
	}
}

// Helper function to generate random string for demo tokens
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// Helper function to check if a user has permission
func checkPermission(ent enterprise.Integration, userID int64, permission string) bool {
	return ent.HasPermission(userID, permission)
}

// Helper function to check if a user has role
func checkRole(ent enterprise.Integration, userID int64, role string) bool {
	return ent.HasRole(userID, role)
}
