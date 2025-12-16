package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// UserAuthMiddleware middleware for UI and control API authentication
func (s *Server) UserAuthMiddleware() gin.HandlerFunc {
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
		globalConfig := s.config
		if globalConfig != nil && globalConfig.HasUserToken() {
			configToken := globalConfig.GetUserToken()

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
		claims, err := s.jwtManager.ValidateAPIKey(token)
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
func (s *Server) ModelAuthMiddleware() gin.HandlerFunc {
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
		globalConfig := s.config
		if globalConfig != nil && globalConfig.HasModelToken() {
			configToken := globalConfig.GetModelToken()

			// Direct token comparison
			if token == configToken || xApiKey == configToken {
				// Token matches the one in global config, allow access
				c.Set("client_id", "model_authenticated")
				c.Next()
				return
			}
		}

		// If not matching global config model token, validate as JWT token
		claims, err := s.jwtManager.ValidateAPIKey(token)
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
