package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/server/config"
)

// ValidateAuthToken validates an authentication token without requiring auth
// This is used during login flow to verify a token before establishing session
func (s *Server) ValidateAuthToken(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"valid":   false,
		})
		return
	}

	// Extract token from "Bearer <token>" format
	tokenParts := strings.Split(authHeader, " ")
	if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"valid":   false,
		})
		return
	}

	token := tokenParts[1]

	// Check against global config user token
	cfg := s.config
	if cfg != nil && cfg.HasUserToken() {
		configToken := cfg.GetUserToken()

		// Direct token comparison
		if token == configToken || strings.TrimPrefix(token, "Bearer ") == configToken {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"valid":   true,
			})
			return
		}
	}

	// Token is invalid
	c.JSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"valid":   false,
	})
}

// GetUserToken returns the current user token (masked)
// Requires authentication
func (s *Server) GetUserToken(c *gin.Context) {
	token := s.config.GetUserToken()
	isDefault := token == constant.DefaultUserToken

	// Return full token - frontend will handle masking
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"token":      token,
			"is_default": isDefault,
		},
	})
}

// ResetUserToken generates a new secure random token and updates the configuration
// Requires authentication
func (s *Server) ResetUserToken(c *gin.Context) {
	newToken, err := config.GenerateUserToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate token",
		})
		return
	}

	if err := s.config.SetUserToken(newToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save token",
		})
		return
	}

	logrus.Info("User token has been reset via web UI")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"token": newToken,
		},
	})
}

// ResetModelToken generates a new secure random model token and updates the configuration
// Requires authentication
func (s *Server) ResetModelToken(c *gin.Context) {
	newToken, err := config.GenerateModelToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate token",
		})
		return
	}

	if err := s.config.SetModelToken(newToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save token",
		})
		return
	}

	logrus.Info("Model token has been reset via web UI")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"token": newToken,
		},
	})
}

// Helper function to mask tokens for display
func maskToken(token string) string {
	if token == "" {
		return ""
	}

	// If already masked, return as is
	if strings.Contains(token, "...") {
		return token
	}

	// For very short tokens, mask all characters
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}

	// For longer tokens, show first 12 and last 4 characters
	// This works for both short and long tokens
	return token[:12] + "..." + token[len(token)-4:]
}
