# Enterprise Edition - Integration Guide

**Version:** 1.0
**Last Updated:** 2026-02-07

## Overview

The Enterprise Edition is a **completely isolated module** that provides multi-user authentication, role-based access control, and API token management. It maintains full separation from the community edition while providing clear integration points.

## Architecture Principles

### 1. Complete Isolation

```
Community Edition:
  ├─ internal/server/       (main server)
  ├─ internal/data/         (community data)
  └─ ~/.tingly-box/db/tingly.db

Enterprise Edition:
  ├─ internal/enterprise/   (enterprise module)
  └─ ~/.tingly-box/db/tingly_enterprise.db  (COMPLETELY ISOLATED)
```

**Key Points:**
- Separate database file (`tingly_enterprise.db`)
- No direct access to community database
- All access through integration interface
- No code modifications to community edition

### 2. Integration Interface

All enterprise functionality is accessed through the `Integration` interface:

```go
package enterprise

type Integration interface {
    // Lifecycle
    Initialize(ctx context.Context, config *Config) error
    IsEnabled() bool
    Shutdown(ctx context.Context) error

    // Authentication
    ValidateAccessToken(ctx context.Context, token string) (*UserInfo, error)
    ValidateAPIToken(ctx context.Context, token string) (*UserInfo, *TokenInfo, error)
    RefreshAccessToken(ctx context.Context, refreshToken string) (string, error)

    // User Info
    GetUserInfo(ctx context.Context, userID int64) (*UserInfo, error)
    GetUserInfoByUsername(ctx context.Context, username string) (*UserInfo, error)

    // Authorization
    HasPermission(userID int64, permission string) bool
    HasRole(userID int64, role string) bool

    // HTTP Middleware
    AuthMiddleware() gin.HandlerFunc
    RequirePermission(permission string) gin.HandlerFunc
    RequireRole(roles ...string) gin.HandlerFunc

    // Admin Management
    CreateUser(ctx context.Context, req *CreateUserRequest, actorID int64) (*UserInfo, error)
    UpdateUser(ctx context.Context, userID int64, req *UpdateUserRequest, actorID int64) (*UserInfo, error)
    DeactivateUser(ctx context.Context, userID int64, actorID int64) error
    ResetPassword(ctx context.Context, userID int64, actorID int64) (string, error)

    // Token Management
    CreateAPIToken(ctx context.Context, req *CreateTokenRequest, actorID int64) (*TokenInfo, string, error)
    ListAPITokens(ctx context.Context, userID int64, page, pageSize int) (*TokenListResult, error)
    RevokeAPIToken(ctx context.Context, tokenID int64, actorID int64) error

    // System
    HealthCheck(ctx context.Context) error
    GetStats(ctx context.Context) (*Stats, error)
    CleanupExpired(ctx context.Context) error
}
```

## Integration Steps

### Step 1: Initialize Enterprise Module

```go
import (
    "context"
    enterprise "github.com/tingly-dev/tingly-box/internal/enterprise"
)

func InitializeEnterprise() (enterprise.Integration, error) {
    integrator := enterprise.NewIntegration()

    config := &enterprise.Config{
        BaseDir:       "/path/to/config",
        JWTSecret:     "your-secret-key",
        Logger:        logrus.StandardLogger(),
        DatabaseConfig: nil, // Use default SQLite config
    }

    if err := integrator.Initialize(context.Background(), config); err != nil {
        return nil, err
    }

    return integrator, nil
}
```

### Step 2: Add Authentication Middleware to Your Router

```go
import (
    "github.com/gin-gonic/gin"
    enterprise "github.com/tingly-dev/tingly-box/internal/enterprise"
)

func SetupRoutes(router *gin.RouterGroup, enterpriseInt enterprise.Integration) {
    // Protect routes with enterprise authentication
    api := router.Group("/api/v1")
    api.Use(enterpriseInt.AuthMiddleware())

    // Your existing routes...
}
```

### Step 3: Validate Tokens in Request Handlers

```go
func MyHandler(c *gin.Context) {
    // Token is automatically validated by middleware
    // Get user info from context
    if userIDVal exists := c.Get("user_id"); userIDVal {
        userID := userIDVal.(int64)
        userInfo, err := enterpriseInt.GetUserInfo(c, userID)
        // Handle user info
    }

    c.JSON(200, gin.H{"message": "success"})
}
```

### Step 4: Check Permissions

```go
func AdminOnlyHandler(c *gin.Context, enterpriseInt enterprise.Integration) {
    userID := c.GetInt64("user_id")

    if !enterpriseInt.HasRole(userID, "admin") {
        c.JSON(403, gin.H{"error": "forbidden"})
        return
    }

    // Admin logic...
}
```

## API Endpoints

Enterprise module provides HTTP handlers at `/enterprise/api/v1/`:

### Authentication
```
POST   /enterprise/api/v1/auth/login
POST   /enterprise/api/v1/auth/refresh
POST   /enterprise/api/v1/auth/logout
GET    /enterprise/api/v1/auth/me
```

### User Management (Admin)
```
GET    /enterprise/api/v1/users
POST   /enterprise/api/v1/users
GET    /enterprise/api/v1/users/:id
PUT    /enterprise/api/v1/users/:id
DELETE /enterprise/api/v1/users/:id
POST   /enterprise/api/v1/users/:id/activate
POST   /enterprise/api/v1/users/:id/deactivate
POST   /enterprise/api/v1/users/:id/password
```

### Token Management
```
GET    /enterprise/api/v1/tokens
POST   /enterprise/api/v1/tokens
GET    /enterprise/api/v1/tokens/:id
PUT    /enterprise/api/v1/tokens/:id
DELETE /enterprise/api/v1/tokens/:id
```

### My Tokens
```
GET    /enterprise/api/v1/my-tokens
POST   /enterprise/api/v1/my-tokens
DELETE /enterprise/api/v1/my-tokens/:uuid
```

## Example: Protecting Existing Routes

```go
import (
    "github.com/gin-gonic/gin"
    enterprise "github.com/tingly-dev/tingly-box/internal/enterprise"
)

func main() {
    r := gin.Default()

    // Initialize enterprise
    entInt, _ := enterprise.NewIntegration()
    entInt.Initialize(context.Background(), &enterprise.Config{
        BaseDir:   "/path/to/config",
        JWTSecret: "secret",
    })

    // Option 1: Protect all routes
    api := r.Group("/api")
    api.Use(entInt.AuthMiddleware())
    {
        api.GET("/providers", providersHandler)
        api.GET("/rules", rulesHandler)
    }

    // Option 2: Protect specific routes
    api.GET("/public", publicHandler)
    api.GET("/protected", entInt.AuthMiddleware(), protectedHandler)

    // Option 3: Require specific permission
    admin := r.Group("/admin")
    admin.Use(entInt.AuthMiddleware())
    admin.Use(entInt.RequirePermission("users:write"))
    {
        admin.GET("/users", usersHandler)
    }

    // Option 4: Require specific role
    adminOnly := r.Group("/admin-only")
    adminOnly.Use(entInt.AuthMiddleware())
    adminOnly.Use(entInt.RequireRole("admin"))
    {
        adminOnly.GET("/settings", settingsHandler)
    }
}
```

## Example: Using Integration Interface Directly

```go
// Validate a token from anywhere in your code
userInfo, err := enterpriseInt.ValidateAccessToken(ctx, tokenString)
if err != nil {
    // Handle error
}

// Check if user has permission
if enterpriseInt.HasPermission(userInfo.ID, "providers:write") {
    // Allow provider modification
}

// Create a new API token for a user
tokenInfo, rawToken, err := enterpriseInt.CreateAPIToken(ctx, &enterprise.CreateTokenRequest{
    Name:   "My Token",
    Scopes: []enterprise.Scope{enterprise.ScopeReadProviders},
}, actorID)
```

## Database Schema

Enterprise uses a **separate database file**: `~/.tingly-box/db/tingly_enterprise.db`

Tables:
- `ent_users` - User accounts
- `ent_api_tokens` - API tokens
- `ent_sessions` - Active sessions
- `ent_audit_logs` - Audit trail

## Security Features

1. **Password Security:**
   - Argon2id hashing (memory-hard KDF)
   - Configurable time/memory/parallelism
   - Minimum 8 chars, 1 uppercase, 1 lowercase, 1 digit

2. **Token Security:**
   - JWT access tokens (15 min expiry)
   - Refresh tokens (7 day expiry)
   - SHA-256 hashed API tokens
   - Token expiration enforcement

3. **Audit Trail:**
   - All admin actions logged
   - IP address and user agent tracking
   - Resource-level tracking

## Running Tests

```bash
# Run all enterprise tests
./tests/enterprise/run_tests.sh

# Run specific package
go test -v ./internal/enterprise/auth
```

## Troubleshooting

### "Enterprise mode not enabled"

The integration will return `ErrNotEnabled` if:
- Database file doesn't exist
- No users in database
- `Initialize()` not called

### "Invalid token"

Common causes:
- Token expired (use refresh token)
- Token malformed
- User account inactive

### "Forbidden"

Common causes:
- User lacks required role
- User lacks required permission
- Token lacks required scope

## Migration Path

1. **Keep running community edition** (no changes needed)
2. **Deploy enterprise module** alongside
3. **Initialize enterprise database** (auto-creates default admin)
4. **Change default admin password**
5. **Migrate users** to enterprise auth
6. **Phase out community auth**

## Support

For issues or questions:
- Documentation: `docs/enterprise/README.md`
- Test Coverage: `docs/enterprise/TEST_COVERAGE.md`
- Specification: `docs/spec/20260207-enterprise-edition.md`
