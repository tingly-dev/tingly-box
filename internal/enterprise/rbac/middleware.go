package rbac

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/enterprise/auth"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
	"github.com/tingly-dev/tingly-box/internal/enterprise/token"
)

// Context keys
const (
	UserKey  = "enterprise.user"
	TokenKey = "enterprise.token"
)

// Permission represents an action that can be performed on a resource
type Permission struct {
	Resource string
	Action   string
}

// All available permissions
const (
	// Provider permissions
	PermReadProviders  = "providers:read"
	PermWriteProviders = "providers:write"

	// Rule permissions
	PermReadRules  = "rules:read"
	PermWriteRules = "rules:write"

	// Usage permissions
	PermReadUsage = "usage:read"

	// User permissions
	PermReadUsers  = "users:read"
	PermWriteUsers = "users:write"

	// Token permissions
	PermReadTokens  = "tokens:read"
	PermWriteTokens = "tokens:write"

	// Audit permissions
	PermReadAudit = "audit:read"
)

// RolePermissions maps roles to their allowed permissions
var RolePermissions = map[db.Role][]string{
	db.RoleAdmin: {
		PermReadProviders,
		PermWriteProviders,
		PermReadRules,
		PermWriteRules,
		PermReadUsage,
		PermReadUsers,
		PermWriteUsers,
		PermReadTokens,
		PermWriteTokens,
		PermReadAudit,
	},
	db.RoleUser: {
		PermReadProviders,
		PermWriteProviders,
		PermReadRules,
		PermWriteRules,
		PermReadUsage,
		PermReadTokens,
		PermWriteTokens,
	},
	db.RoleReadOnly: {
		PermReadProviders,
		PermReadRules,
		PermReadUsage,
	},
}

// ScopeToPermission maps token scopes to permissions
var ScopeToPermission = map[db.Scope]string{
	db.ScopeReadProviders:  PermReadProviders,
	db.ScopeWriteProviders: PermWriteProviders,
	db.ScopeReadRules:      PermReadRules,
	db.ScopeWriteRules:     PermWriteRules,
	db.ScopeReadUsage:      PermReadUsage,
	db.ScopeReadUsers:      PermReadUsers,
	db.ScopeWriteUsers:     PermWriteUsers,
	db.ScopeReadTokens:     PermReadTokens,
	db.ScopeWriteTokens:    PermWriteTokens,
}

// GetCurrentUser retrieves the current user from context
func GetCurrentUser(c *gin.Context) *db.User {
	if user, exists := c.Get(UserKey); exists {
		if u, ok := user.(*db.User); ok {
			return u
		}
	}
	return nil
}

// GetCurrentToken retrieves the current API token from context
func GetCurrentToken(c *gin.Context) *db.APIToken {
	if tkn, exists := c.Get(TokenKey); exists {
		if t, ok := tkn.(*db.APIToken); ok {
			return t
		}
	}
	return nil
}

// RequireRole checks if the current user has one of the required roles
func RequireRole(roles ...db.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetCurrentUser(c)
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		for _, role := range roles {
			if user.Role == role {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: insufficient role"})
		c.Abort()
	}
}

// RequirePermission checks if the current user has a specific permission
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetCurrentUser(c)
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		// Check if user's role has the permission
		permissions, exists := RolePermissions[user.Role]
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: unknown role"})
			c.Abort()
			return
		}

		for _, p := range permissions {
			if p == permission {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: insufficient permissions"})
		c.Abort()
	}
}

// RequireScope checks if the current token has a specific scope
func RequireScope(scope db.Scope) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiToken := GetCurrentToken(c)
		if apiToken == nil {
			// No API token, check if it's a session-based request
			user := GetCurrentUser(c)
			if user == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				c.Abort()
				return
			}
			// For session-based auth, check user role permissions
			if permission, ok := ScopeToPermission[scope]; ok {
				permissions, exists := RolePermissions[user.Role]
				if !exists {
					c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
					c.Abort()
					return
				}
				for _, p := range permissions {
					if p == permission {
						c.Next()
						return
					}
				}
			}
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: insufficient scope"})
			c.Abort()
			return
		}

		// Check token scopes
		hasScope, err := token.HasScope(apiToken, scope)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error checking scopes"})
			c.Abort()
			return
		}

		if !hasScope {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: insufficient scope"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAnyScope checks if the current token has any of the specified scopes
func RequireAnyScope(scopes ...db.Scope) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiToken := GetCurrentToken(c)
		if apiToken == nil {
			// No API token, check if it's a session-based request
			user := GetCurrentUser(c)
			if user == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				c.Abort()
				return
			}
			// For session-based auth, check user role permissions
			permissions, exists := RolePermissions[user.Role]
			if !exists {
				c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				c.Abort()
				return
			}

			// Check if user's role has any of the required permissions
			for _, scope := range scopes {
				if permission, ok := ScopeToPermission[scope]; ok {
					for _, p := range permissions {
						if p == permission {
							c.Next()
							return
						}
					}
				}
			}

			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: insufficient permissions"})
			c.Abort()
			return
		}

		// Check token scopes
		hasAnyScope, err := token.HasAnyScope(apiToken, scopes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error checking scopes"})
			c.Abort()
			return
		}

		if !hasAnyScope {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: insufficient scope"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAdmin checks if the current user is an admin
func RequireAdmin() gin.HandlerFunc {
	return RequireRole(db.RoleAdmin)
}

// IsAdmin checks if a user is an admin
func IsAdmin(user *db.User) bool {
	return user != nil && user.Role == db.RoleAdmin
}

// HasPermission checks if a user has a specific permission
func HasPermission(user *db.User, permission string) bool {
	if user == nil {
		return false
	}

	permissions, exists := RolePermissions[user.Role]
	if !exists {
		return false
	}

	for _, p := range permissions {
		if p == permission {
			return true
		}
	}

	return false
}

// ExtractTokenFromHeader extracts the bearer token from the Authorization header
func ExtractTokenFromHeader(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
		return parts[1]
	}

	return ""
}

// SetCurrentUser sets the current user in the context
func SetCurrentUser(c *gin.Context, user *db.User) {
	c.Set(UserKey, user)
}

// SetCurrentToken sets the current token in the context
func SetCurrentToken(c *gin.Context, token *db.APIToken) {
	c.Set(TokenKey, token)
}

// AuthMiddleware creates a middleware that validates JWT tokens or API tokens
type AuthMiddleware struct {
	jwtSvc     *auth.JWTService
	userRepo   db.UserRepository
	tokenModel *token.Model
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(jwtSvc *auth.JWTService, userRepo db.UserRepository, tokenModel *token.Model) *AuthMiddleware {
	return &AuthMiddleware{
		jwtSvc:     jwtSvc,
		userRepo:   userRepo,
		tokenModel: tokenModel,
	}
}

// Authenticate validates JWT tokens (for session-based auth)
func (m *AuthMiddleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := ExtractTokenFromHeader(c)
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		// Try to validate as JWT access token
		claims, err := m.jwtSvc.ValidateAccessToken(tokenString)
		if err == nil {
			// Valid JWT token
			user, err := m.userRepo.GetByID(claims.UserID)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				c.Abort()
				return
			}

			if !user.IsActive {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "user account is inactive"})
				c.Abort()
				return
			}

			SetCurrentUser(c, user)
			c.Next()
			return
		}

		// Try to validate as API token
		apiToken, err := m.tokenModel.ValidateToken(tokenString)
		if err == nil {
			// Valid API token
			if !apiToken.IsActive {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "token is inactive"})
				c.Abort()
				return
			}

			if apiToken.User == nil || !apiToken.User.IsActive {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "user account is inactive"})
				c.Abort()
				return
			}

			SetCurrentUser(c, apiToken.User)
			SetCurrentToken(c, apiToken)

			// Record token usage
			_ = m.tokenModel.RecordUsage(apiToken.ID)

			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		c.Abort()
	}
}
