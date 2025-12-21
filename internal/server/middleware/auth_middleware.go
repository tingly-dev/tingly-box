package middleware

import (
	"net/http"
	"strings"

	"tingly-box/internal/auth"
	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware provides authentication middleware for different types of authentication
type AuthMiddleware struct {
	config     *config.Config
	jwtManager *auth.JWTManager
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(cfg *config.Config, jwtManager *auth.JWTManager) *AuthMiddleware {
	return &AuthMiddleware{
		config:     cfg,
		jwtManager: jwtManager,
	}
}

// UserAuthMiddleware middleware for UI and control API authentication
func (am *AuthMiddleware) UserAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Authorization header required",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>" format
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Invalid authorization header format. Expected: 'Bearer <token>'",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		token := tokenParts[1]

		// Check against global config user token first
		cfg := am.config
		if cfg != nil && cfg.HasUserToken() {
			configToken := cfg.GetUserToken()

			// Remove "Bearer " prefix if present in the token
			if strings.HasPrefix(token, "Bearer ") {
				token = token[7:]
			}

			// Direct token comparison
			if token == configToken || strings.TrimPrefix(token, "Bearer ") == configToken {
				// Token matches the one in global config, allow access
				c.Set("client_id", "user_authenticated")
				c.Next()
				return
			}
		}

		// If not matching global config user token, validate as JWT token
		claims, err := am.jwtManager.ValidateAPIKey(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Invalid or expired token",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		// Store client ID in context
		c.Set("client_id", claims.ClientID)
		c.Next()
	}
}

// ModelAuthMiddleware middleware for OpenAI and Anthropic API authentication
// The auth will support both `Authorization` and `X-Api-Key`
func (am *AuthMiddleware) ModelAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		xApiKey := c.GetHeader("X-Api-Key")
		if authHeader == "" && xApiKey == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Authorization header required",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		token := authHeader
		// Remove "Bearer " prefix if present in the token
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		}

		// Check against global config model token first
		cfg := am.config
		if cfg != nil && cfg.HasModelToken() {
			configToken := cfg.GetModelToken()

			// Direct token comparison
			if token == configToken || xApiKey == configToken {
				// Token matches the one in global config, allow access
				c.Set("client_id", "model_authenticated")
				c.Next()
				return
			}
		}

		// If not matching global config model token, validate as JWT token
		claims, err := am.jwtManager.ValidateAPIKey(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Invalid or expired token",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		// Store client ID in context
		c.Set("client_id", claims.ClientID)
		c.Next()
	}
}

// AuthMiddleware validates the authentication token
func (am *AuthMiddleware) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the auth token from global config
		cfg := am.config
		if cfg == nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Global config not available",
			})
			c.Abort()
			return
		}

		expectedToken := cfg.GetUserToken()
		if expectedToken == "" {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "User auth token not configured",
			})
			c.Abort()
			return
		}

		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization header required",
			})
			c.Abort()
			return
		}

		// Support both "Bearer token" and just "token" formats
		token := strings.TrimPrefix(authHeader, "Bearer ")
		token = strings.TrimSpace(token)

		if token != expectedToken {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid authentication token",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
