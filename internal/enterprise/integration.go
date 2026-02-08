// Package enterprise provides integration interfaces for enterprise features
// This package maintains COMPLETE ISOLATION from the community edition
// while providing clear integration points for external systems.
package enterprise

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/enterprise/auth"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
	"github.com/tingly-dev/tingly-box/internal/enterprise/rbac"
	"github.com/tingly-dev/tingly-box/internal/enterprise/token"
	"github.com/tingly-dev/tingly-box/internal/enterprise/user"
)

// Integration provides the main interface for integrating enterprise features
// This interface allows external systems to interact with enterprise functionality
// without accessing internal implementations directly.
type Integration interface {
	// Lifecycle methods

	// Initialize sets up all enterprise services
	// Returns error if enterprise mode is not enabled or initialization fails
	Initialize(ctx context.Context, config *Config) error

	// IsEnabled returns whether enterprise mode is enabled
	IsEnabled() bool

	// Shutdown gracefully shuts down enterprise services
	Shutdown(ctx context.Context) error

	// Authentication methods

	// ValidateAccessToken validates an access token and returns the associated user
	// Returns ErrInvalidToken if token is invalid
	// Returns ErrTokenExpired if token has expired
	// Returns ErrUserInactive if user account is disabled
	ValidateAccessToken(ctx context.Context, token string) (*UserInfo, error)

	// ValidateAPIToken validates an API token and returns the associated user
	// Returns ErrInvalidToken if token is invalid
	// Returns ErrTokenExpired if token has expired
	// Returns ErrTokenInactive if token is deactivated
	ValidateAPIToken(ctx context.Context, token string) (*UserInfo, *TokenInfo, error)

	// RefreshAccessToken generates a new access token from a refresh token
	RefreshAccessToken(ctx context.Context, refreshToken string) (string, error)

	// User information methods

	// GetUserInfo retrieves user information by ID
	GetUserInfo(ctx context.Context, userID int64) (*UserInfo, error)

	// GetUserInfoByUsername retrieves user information by username
	GetUserInfoByUsername(ctx context.Context, username string) (*UserInfo, error)

	// GetUserInfoByUUID retrieves user information by UUID
	GetUserInfoByUUID(ctx context.Context, uuid string) (*UserInfo, error)

	// Authorization methods

	// HasPermission checks if a user has a specific permission
	HasPermission(userID int64, permission string) bool

	// HasRole checks if a user has a specific role
	HasRole(userID int64, role string) bool

	// HasAnyPermission checks if a user has any of the specified permissions
	HasAnyPermission(userID int64, permissions ...string) bool

	// HTTP middleware for route protection

	// AuthMiddleware returns Gin middleware for JWT authentication
	AuthMiddleware() gin.HandlerFunc

	// RequirePermission returns Gin middleware that requires specific permission
	RequirePermission(permission string) gin.HandlerFunc

	// RequireRole returns Gin middleware that requires specific role
	RequireRole(roles ...string) gin.HandlerFunc

	// Admin management methods (for admin operations)

	// CreateUser creates a new user (requires admin permission)
	CreateUser(ctx context.Context, req *CreateUserRequest, actorID int64) (*UserInfo, error)

	// UpdateUser updates user information (requires admin permission or own account)
	UpdateUser(ctx context.Context, userID int64, req *UpdateUserRequest, actorID int64) (*UserInfo, error)

	// DeactivateUser deactivates a user account (requires admin permission)
	DeactivateUser(ctx context.Context, userID int64, actorID int64) error

	// ResetPassword resets a user's password (requires admin permission)
	ResetPassword(ctx context.Context, userID int64, actorID int64) (string, error)

	// Token management methods

	// CreateAPIToken creates a new API token for a user
	CreateAPIToken(ctx context.Context, req *CreateTokenRequest, actorID int64) (*TokenInfo, string, error)

	// ListAPITokens lists API tokens for a user
	ListAPITokens(ctx context.Context, userID int64, page, pageSize int) (*TokenListResult, error)

	// RevokeAPIToken revokes an API token
	RevokeAPIToken(ctx context.Context, tokenID int64, actorID int64) error

	// Audit methods

	// LogAudit logs an audit entry
	LogAudit(ctx context.Context, entry *AuditEntry) error

	// QueryAuditLogs retrieves audit logs with pagination and filters
	QueryAuditLogs(ctx context.Context, query *AuditQuery) (*AuditResult, error)

	// Health and statistics

	// HealthCheck performs a health check on enterprise services
	HealthCheck(ctx context.Context) error

	// GetStats returns enterprise statistics
	GetStats(ctx context.Context) (*Stats, error)

	// Cleanup methods

	// CleanupExpired removes expired sessions and tokens
	CleanupExpired(ctx context.Context) error
}

// Config holds enterprise configuration
type Config struct {
	// BaseDir is the base directory for enterprise data
	BaseDir string

	// JWTSecret is the secret key for JWT signing
	JWTSecret string

	// AccessTokenExpiry is the duration for access tokens (default: 15m)
	AccessTokenExpiry string

	// RefreshTokenExpiry is the duration for refresh tokens (default: 168h/7d)
	RefreshTokenExpiry string

	// SessionExpiry is the duration for sessions (default: 24h)
	SessionExpiry string

	// PasswordMinLength is the minimum password length (default: 8)
	PasswordMinLength int

	// Logger is the logger instance (should be logrus)
	Logger interface{ // Using interface to avoid direct logrus dependency
		WithFields(map[string]interface{}) interface{}
		WithError(error) interface{}
		Warn(args ...interface{})
		Info(args ...interface{})
		Debug(args ...interface{})
		Error(args ...interface{})
	}

	// DatabaseConfig allows custom database configuration
	// If nil, uses default SQLite configuration
	DatabaseConfig *db.EnterpriseDBConfig
}

// UserInfo represents user information returned by integration methods
type UserInfo struct {
	ID          int64  `json:"id"`
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	FullName    string `json:"full_name"`
	Role        string `json:"role"`
	IsActive    bool   `json:"is_active"`
	LastLoginAt *int64 `json:"last_login_at,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// TokenInfo represents API token information
type TokenInfo struct {
	ID          int64   `json:"id"`
	UUID        string  `json:"uuid"`
	UserID      int64   `json:"user_id"`
	Name        string  `json:"name"`
	TokenPrefix string  `json:"token_prefix"`
	Scopes      []Scope `json:"scopes"`
	ExpiresAt   *int64  `json:"expires_at,omitempty"`
	CreatedAt   int64   `json:"created_at"`
}

// Scope represents an API token permission scope
type Scope string

const (
	ScopeReadProviders  Scope = "read:providers"
	ScopeWriteProviders Scope = "write:providers"
	ScopeReadRules      Scope = "read:rules"
	ScopeWriteRules     Scope = "write:rules"
	ScopeReadUsage      Scope = "read:usage"
	ScopeReadUsers      Scope = "read:users"
	ScopeWriteUsers     Scope = "write:users"
	ScopeReadTokens     Scope = "read:tokens"
	ScopeWriteTokens    Scope = "write:tokens"
)

// CreateUserRequest holds data for creating a user
type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name,omitempty"`
	Role     string `json:"role"`
}

// UpdateUserRequest holds data for updating a user
type UpdateUserRequest struct {
	FullName *string `json:"full_name,omitempty"`
	Role     *string `json:"role,omitempty"`
}

// CreateTokenRequest holds data for creating an API token
type CreateTokenRequest struct {
	Name      string   `json:"name"`
	Scopes    []Scope `json:"scopes"`
	ExpiresAt *int64  `json:"expires_at,omitempty"`
	UserID    *int64  `json:"user_id,omitempty"` // nil = current user
}

// TokenListResult holds paginated token list results
type TokenListResult struct {
	Tokens []*TokenWithUser `json:"tokens"`
	Total  int64            `json:"total"`
	Page   int              `json:"page"`
	Size   int              `json:"size"`
}

// TokenWithUser extends TokenInfo with user information
type TokenWithUser struct {
	TokenInfo
	Username string `json:"username"`
	Email    string `json:"email"`
}

// AuditEntry represents an audit log entry
type AuditEntry struct {
	UserID       *int64                  `json:"user_id,omitempty"`
	Action       string                  `json:"action"`
	ResourceType string                  `json:"resource_type,omitempty"`
	ResourceID   string                  `json:"resource_id,omitempty"`
	Details      map[string]interface{}  `json:"details,omitempty"`
	IPAddress    string                  `json:"ip_address,omitempty"`
	UserAgent    string                  `json:"user_agent,omitempty"`
	Status       string                  `json:"status"` // success, failure
}

// AuditQuery holds query parameters for audit logs
type AuditQuery struct {
	Page         int                      `json:"page"`
	PageSize     int                      `json:"page_size"`
	UserID       *int64                   `json:"user_id,omitempty"`
	Action       string                   `json:"action,omitempty"`
	ResourceType string                   `json:"resource_type,omitempty"`
	Status       string                   `json:"status,omitempty"`
	StartDate    *int64                   `json:"start_date,omitempty"`
	EndDate      *int64                   `json:"end_date,omitempty"`
}

// AuditResult holds audit log query results
type AuditResult struct {
	Logs  []*AuditLogEntry `json:"logs"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

// AuditLogEntry represents a single audit log entry
type AuditLogEntry struct {
	ID           int64                  `json:"id"`
	UserID       *int64                 `json:"user_id,omitempty"`
	Username     string                 `json:"username,omitempty"`
	Action       string                 `json:"action"`
	ResourceType string                 `json:"resource_type,omitempty"`
	ResourceID   string                 `json:"resource_id,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	IPAddress    string                 `json:"ip_address,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	Status       string                 `json:"status"`
	CreatedAt    int64                  `json:"created_at"`
}

// Stats holds enterprise statistics
type Stats struct {
	UserCount       int     `json:"user_count"`
	TokenCount      int     `json:"token_count"`
	SessionCount    int     `json:"session_count"`
	AuditLogCount   int     `json:"audit_log_count"`
	DatabasePath    string  `json:"database_path"`
	DatabaseSizeMB  float64 `json:"database_size_mb"`
	Enabled         bool    `json:"enabled"`
}

// Errors
var (
	ErrNotEnabled        = &EnterpriseError{Code: "NOT_ENABLED", Message: "Enterprise mode is not enabled"}
	ErrInvalidToken      = &EnterpriseError{Code: "INVALID_TOKEN", Message: "Invalid token"}
	ErrTokenExpired      = &EnterpriseError{Code: "TOKEN_EXPIRED", Message: "Token has expired"}
	ErrTokenInactive     = &EnterpriseError{Code: "TOKEN_INACTIVE", Message: "Token is inactive"}
	ErrUserInactive      = &EnterpriseError{Code: "USER_INACTIVE", Message: "User account is inactive"}
	ErrUserNotFound      = &EnterpriseError{Code: "USER_NOT_FOUND", Message: "User not found"}
	ErrForbidden         = &EnterpriseError{Code: "FORBIDDEN", Message: "Access forbidden"}
	ErrInvalidCredential = &EnterpriseError{Code: "INVALID_CREDENTIAL", Message: "Invalid username or password"}
)

// EnterpriseError represents an enterprise-specific error
type EnterpriseError struct {
	Code    string
	Message string
}

func (e *EnterpriseError) Error() string {
	return e.Message
}

// NewIntegration creates a new enterprise integration instance
func NewIntegration() Integration {
	return &enterpriseIntegration{}
}

// enterpriseIntegration implements the Integration interface
type enterpriseIntegration struct {
	config        *Config
	enabled       bool
	db            *db.EnterpriseDB
	authService   *auth.AuthService
	userService   user.Service
	tokenService  token.Service
	passwordSvc   *auth.PasswordService
	jwtSvc        *auth.JWTService
	authMiddleware *rbac.AuthMiddleware
}

// ... (implementation would be in separate files)

// Helper function to convert db.User to UserInfo
func toUserInfo(u *db.User) *UserInfo {
	var lastLogin *int64
	if u.LastLoginAt != nil {
		timestamp := u.LastLoginAt.Unix()
		lastLogin = &timestamp
	}

	return &UserInfo{
		ID:          u.ID,
		UUID:        u.UUID,
		Username:    u.Username,
		Email:       u.Email,
		FullName:    u.FullName,
		Role:        string(u.Role),
		IsActive:    u.IsActive,
		LastLoginAt: lastLogin,
		CreatedAt:   u.CreatedAt.Unix(),
	}
}

// Helper function to convert db.APIToken to TokenInfo
func toTokenInfo(t *db.APIToken) *TokenInfo {
	var expiresAt *int64
	if t.ExpiresAt != nil {
		timestamp := t.ExpiresAt.Unix()
		expiresAt = &timestamp
	}

	// Parse scopes from JSON
	scopes := []Scope{}
	// TODO: Parse t.Scopes JSON field

	return &TokenInfo{
		ID:          t.ID,
		UUID:        t.UUID,
		UserID:      t.UserID,
		Name:        t.Name,
		TokenPrefix: t.TokenPrefix,
		Scopes:      scopes,
		ExpiresAt:   expiresAt,
		CreatedAt:   t.CreatedAt.Unix(),
	}
}
