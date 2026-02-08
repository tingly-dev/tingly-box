# Enterprise Edition Integration Example

This example demonstrates how to integrate and use the Tingly Box Enterprise Edition module with a simple Gin web service.

## Overview

The example shows:
- How to initialize the enterprise module
- How to set up authentication middleware
- How to protect routes with role-based access control
- How to use the Integration interface for user and token management

## Running the Example

```bash
# From the project root
cd examples/enterprise
go run main.go handlers.go
```

The server will start on `http://localhost:12581`

## API Endpoints

### Public Endpoints (No Authentication)

```bash
# Health check
curl http://localhost:12581/api/ping

# Login (returns JWT access token)
curl -X POST http://localhost:12581/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'

# Demo: Create a test user (not persisted)
curl -X POST http://localhost:12581/api/auth/demo-create-user \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"TestPass123","email":"test@example.com"}'
```

### Protected Endpoints (Require Authentication)

```bash
# Get current user profile
curl http://localhost:12581/api/profile \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

# Change password
curl -X POST http://localhost:12581/api/change-password \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"current_password":"old","new_password":"NewPass123"}'

# List my tokens
curl http://localhost:12581/api/my-tokens \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

# Create a new API token
curl -X POST http://localhost:12581/api/my-tokens \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"My Token","scopes":["read:providers"]}'

# Delete a token
curl -X DELETE http://localhost:12581/api/my-tokens/TOKEN_UUID \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

### Admin Endpoints (Require Admin Role)

```bash
# List all users
curl http://localhost:12581/api/admin/users \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"

# Create a new user
curl -X POST http://localhost:12581/api/admin/users \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"newuser","email":"new@example.com","password":"Pass123","full_name":"New User","role":"user"}'

# Get user by ID
curl http://localhost:12581/api/admin/users/1 \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"

# Update user
curl -X PUT http://localhost:12581/api/admin/users/2 \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"full_name":"Updated Name","role":"admin"}'

# Deactivate user
curl -X POST http://localhost:12581/api/admin/users/2/deactivate \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"

# Reset user password
curl -X POST http://localhost:12581/api/admin/users/2/password \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"

# List all tokens
curl http://localhost:12581/api/admin/tokens \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"

# Get system statistics
curl http://localhost:12581/api/admin/stats \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

## Default Admin User

On first run, the enterprise module creates a default admin user:
- **Username:** `admin`
- **Password:** `$CHANGE_REQUIRED$` (must be changed)
- **Email:** `admin@tingly-box.local`

You will need to implement the password change functionality to set a real password.

## Integration Points

### 1. Initialize the Enterprise Module

```go
integration := enterprise.NewIntegration()

config := &enterprise.Config{
    BaseDir:             configDir,
    JWTSecret:           "your-secret-key",
    AccessTokenExpiry:   "15m",
    RefreshTokenExpiry:  "168h",
    PasswordMinLength:   8,
    Logger:              logrus.StandardLogger(),
}

if err := integration.Initialize(context.Background(), config); err != nil {
    log.Fatal(err)
}
```

### 2. Add Authentication Middleware

```go
// Protect routes with authentication
protected := router.Group("/api")
protected.Use(integration.AuthMiddleware())
{
    protected.GET("/profile", handleProfile)
}
```

### 3. Require Specific Roles

```go
admin := router.Group("/admin")
admin.Use(integration.AuthMiddleware())
admin.Use(integration.RequireRole("admin"))
{
    admin.GET("/users", handleListUsers)
}
```

### 4. Use the Integration Interface

```go
func handleProfile(c *gin.Context) {
    userID := c.GetInt64("user_id")
    userInfo, err := integration.GetUserInfo(ctx, userID)
    // ...
}
```

## Architecture

```
examples/enterprise/
├── main.go       # Server setup and initialization
├── handlers.go   # HTTP handlers demonstrating Integration API usage
└── README.md     # This file
```

## Key Files

- **main.go:** Shows how to initialize the enterprise module and set up routes
- **handlers.go:** Demonstrates how to use the Integration interface for common operations

## Next Steps

1. Implement the login endpoint with real authentication
2. Implement user creation with database persistence
3. Add password change functionality
4. Integrate with your existing application routes

## Documentation

- [Integration Guide](../../docs/enterprise/INTEGRATION.md)
- [Test Coverage](../../docs/enterprise/TEST_COVERAGE.md)
- [Specification](../../docs/spec/20260207-enterprise-edition.md)
