# Enterprise Edition Specification

**Created:** 2026-02-07
**Status:** Design Phase
**Based on:** `20260207-arch.md`

## Executive Summary

This specification defines the Enterprise Edition for Tingly Box, adding multi-user support, multi-token management, and administrative capabilities while maintaining complete isolation from the community edition codebase.

**Key Requirements:**
- Multi-user authentication and authorization
- Multi-token management with role-based access control (RBAC)
- Administrative interface for user and token management
- Complete code isolation for optional enterprise enablement
- Comprehensive test coverage

## Design Principles

### 1. Code Isolation Strategy

Enterprise features will be completely isolated using:
- **Separate Package:** `internal/enterprise/` for all enterprise code
- **Feature Flag:** `enterprise_enabled` in config for activation
- **Database Prefix:** All enterprise tables prefixed with `ent_`
- **API Prefix:** `/enterprise/` for all enterprise endpoints
- **Frontend Route:** `/enterprise/` for admin UI

### 2. Backward Compatibility

- Community edition operates unchanged when enterprise is disabled
- Existing single-token system remains functional
- No breaking changes to existing APIs
- Configuration migration path provided

## Architecture Overview

### Package Structure

```
internal/enterprise/
├── auth/               # Multi-user authentication
│   ├── service.go      # AuthService interface
│   ├── jwt_service.go  # JWT-based implementation
│   ├── session.go      # Session management
│   └── password.go     # Password hashing/validation
├── user/               # User management
│   ├── service.go      # UserService interface
│   ├── repository.go   # User data access
│   └── model.go        # User domain model
├── token/              # Token management
│   ├── service.go      # TokenService interface
│   ├── repository.go   # Token data access
│   └── model.go        # Token domain model
├── rbac/               # Role-based access control
│   ├── permissions.go  # Permission definitions
│   ├── roles.go        # Role definitions
│   └── middleware.go   # RBAC middleware
├── admin/              # Admin panel APIs
│   ├── handler.go      # Admin HTTP handlers
│   └── router.go       # Admin route setup
└── db/                 # Enterprise database schema
    ├── user.go         # User table
    ├── token.go        # API token table
    ├── session.go      # Session table
    └── audit_log.go    # Audit log table
```

### Database Schema

#### New Tables

```sql
-- Enterprise users table
CREATE TABLE ent_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,
    username TEXT UNIQUE NOT NULL,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user', -- 'admin', 'user', 'readonly'
    full_name TEXT,
    is_active BOOLEAN DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_login_at INTEGER
);

-- Enterprise API tokens table
CREATE TABLE ent_api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,
    user_id INTEGER NOT NULL,
    token_hash TEXT UNIQUE NOT NULL,
    token_prefix TEXT NOT NULL, -- First 8 chars for identification
    name TEXT NOT NULL,
    scopes TEXT NOT NULL, -- JSON array of scopes
    expires_at INTEGER,
    last_used_at INTEGER,
    is_active BOOLEAN DEFAULT 1,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES ent_users(id) ON DELETE CASCADE
);

-- Enterprise sessions table
CREATE TABLE ent_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,
    user_id INTEGER NOT NULL,
    session_token TEXT UNIQUE NOT NULL,
    refresh_token TEXT UNIQUE,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES ent_users(id) ON DELETE CASCADE
);

-- Enterprise audit log table
CREATE TABLE ent_audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    action TEXT NOT NULL, -- 'user.login', 'token.create', 'user.delete', etc.
    resource_type TEXT, -- 'user', 'token', 'provider', 'rule'
    resource_id TEXT,
    details TEXT, -- JSON context
    ip_address TEXT,
    user_agent TEXT,
    status TEXT, -- 'success', 'failure'
    created_at INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES ent_users(id) ON DELETE SET NULL
);

-- Indexes for performance
CREATE INDEX idx_ent_users_username ON ent_users(username);
CREATE INDEX idx_ent_users_email ON ent_users(email);
CREATE INDEX idx_ent_users_role ON ent_users(role);
CREATE INDEX idx_ent_api_tokens_user_id ON ent_api_tokens(user_id);
CREATE INDEX idx_ent_api_tokens_token_hash ON ent_api_tokens(token_hash);
CREATE INDEX idx_ent_sessions_user_id ON ent_sessions(user_id);
CREATE INDEX idx_ent_sessions_token ON ent_sessions(session_token);
CREATE INDEX idx_ent_audit_logs_user_id ON ent_audit_logs(user_id);
CREATE INDEX idx_ent_audit_logs_action ON ent_audit_logs(action);
CREATE INDEX idx_ent_audit_logs_created_at ON ent_audit_logs(created_at);
```

## Domain Models

### User Model

```go
// Role defines user roles
type Role string

const (
    RoleAdmin    Role = "admin"
    RoleUser     Role = "user"
    RoleReadOnly Role = "readonly"
)

// User represents an enterprise user
type User struct {
    ID           int64     `json:"id" db:"id"`
    UUID         string    `json:"uuid" db:"uuid"`
    Username     string    `json:"username" db:"username"`
    Email        string    `json:"email" db:"email"`
    PasswordHash string    `json:"-" db:"password_hash"`
    Role         Role      `json:"role" db:"role"`
    FullName     string    `json:"full_name" db:"full_name"`
    IsActive     bool      `json:"is_active" db:"is_active"`
    CreatedAt    time.Time `json:"created_at" db:"created_at"`
    UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
    LastLoginAt  *time.Time `json:"last_login_at" db:"last_login_at"`
}
```

### Token Model

```go
// Scope defines token permission scopes
type Scope string

const (
    ScopeReadProviders   Scope = "read:providers"
    ScopeWriteProviders  Scope = "write:providers"
    ScopeReadRules       Scope = "read:rules"
    ScopeWriteRules      Scope = "write:rules"
    ScopeReadUsage       Scope = "read:usage"
    ScopeReadUsers       Scope = "read:users"
    ScopeWriteUsers      Scope = "write:users"
    ScopeReadTokens      Scope = "read:tokens"
    ScopeWriteTokens     Scope = "write:tokens"
    ScopeAdminAll        Scope = "admin:all"
)

// APIToken represents an enterprise API token
type APIToken struct {
    ID          int64      `json:"id" db:"id"`
    UUID        string     `json:"uuid" db:"uuid"`
    UserID      int64      `json:"user_id" db:"user_id"`
    TokenHash   string     `json:"-" db:"token_hash"`
    TokenPrefix string     `json:"token_prefix" db:"token_prefix"`
    Name        string     `json:"name" db:"name"`
    Scopes      []Scope    `json:"scopes" db:"scopes"`
    ExpiresAt   *time.Time `json:"expires_at" db:"expires_at"`
    LastUsedAt  *time.Time `json:"last_used_at" db:"last_used_at"`
    IsActive    bool       `json:"is_active" db:"is_active"`
    CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}
```

### Session Model

```go
// Session represents a user session
type Session struct {
    ID           int64     `json:"id" db:"id"`
    UUID         string    `json:"uuid" db:"uuid"`
    UserID       int64     `json:"user_id" db:"user_id"`
    SessionToken string    `json:"-" db:"session_token"`
    RefreshToken string    `json:"-" db:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
    CreatedAt    time.Time `json:"created_at" db:"created_at"`
}
```

## Authentication Flow

### Login Flow

1. User submits credentials to `/api/v1/auth/login`
2. AuthService validates credentials
3. On success:
   - Creates session record
   - Generates JWT access token (15min expiry)
   - Generates refresh token (7 day expiry)
   - Returns tokens to client
4. Client stores tokens and uses access token for API calls

### Token Refresh Flow

1. Client sends refresh token to `/api/v1/auth/refresh`
2. Validates refresh token and session
3. Issues new access token
4. Optionally rotates refresh token

### Token Validation

For enterprise mode, authentication middleware:
1. Checks if enterprise is enabled
2. Validates JWT signature and expiry
3. Loads user from database
4. Checks user active status
5. Sets user context in Gin

## Role-Based Access Control

### Permissions Matrix

| Action | Admin | User | ReadOnly |
|--------|-------|------|----------|
| List providers | ✓ | ✓ | ✓ |
| Create provider | ✓ | ✓ | ✗ |
| Update provider | ✓ | ✓ | ✗ |
| Delete provider | ✓ | ✗ | ✗ |
| List rules | ✓ | ✓ | ✓ |
| Create rule | ✓ | ✓ | ✗ |
| Update rule | ✓ | ✓ | ✗ |
| Delete rule | ✓ | ✗ | ✗ |
| View usage | ✓ | ✓ | ✓ |
| List users | ✓ | ✗ | ✗ |
| Create user | ✓ | ✗ | ✗ |
| Update user | ✓ | ✗ | ✗ |
| Delete user | ✓ | ✗ | ✗ |
| List tokens | ✓ | ✓ (own) | ✗ |
| Create token | ✓ | ✓ | ✗ |
| Delete token | ✓ | ✓ (own) | ✗ |
| View audit logs | ✓ | ✗ | ✗ |

### Middleware Implementation

```go
// RequireRole checks if user has required role
func RequireRole(allowedRoles ...Role) gin.HandlerFunc {
    return func(c *gin.Context) {
        user := GetCurrentUser(c)
        if user == nil {
            c.JSON(401, gin.H{"error": "unauthorized"})
            c.Abort()
            return
        }

        for _, role := range allowedRoles {
            if user.Role == role {
                c.Next()
                return
            }
        }

        c.JSON(403, gin.H{"error": "forbidden"})
        c.Abort()
    }
}

// RequireScope checks if token has required scope
func RequireScope(scope Scope) gin.HandlerFunc {
    return func(c *gin.Context) {
        tokenScopes := GetTokenScopes(c)
        for _, s := range tokenScopes {
            if s == scope || s == ScopeAdminAll {
                c.Next()
                return
            }
        }

        c.JSON(403, gin.H{"error": "insufficient scope"})
        c.Abort()
    }
}
```

## API Endpoints

### Authentication Endpoints

```
POST   /api/v1/auth/login        - User login
POST   /api/v1/auth/logout       - User logout
POST   /api/v1/auth/refresh      - Refresh access token
GET    /api/v1/auth/me           - Get current user info
```

### Admin Endpoints

```
# User Management
GET    /enterprise/admin/users           - List users
POST   /enterprise/admin/users           - Create user
GET    /enterprise/admin/users/:id       - Get user details
PUT    /enterprise/admin/users/:id       - Update user
DELETE /enterprise/admin/users/:id       - Delete user
POST   /enterprise/admin/users/:id/activate   - Activate user
POST   /enterprise/admin/users/:id/deactivate - Deactivate user
POST   /enterprise/admin/users/:id/password  - Reset password

# Token Management
GET    /enterprise/admin/tokens          - List all tokens
POST   /enterprise/admin/tokens          - Create token
GET    /enterprise/admin/tokens/:id      - Get token details
PUT    /enterprise/admin/tokens/:id      - Update token
DELETE /enterprise/admin/tokens/:id      - Delete token

# Current User's Tokens
GET    /enterprise/user/tokens           - List my tokens
POST   /enterprise/user/tokens           - Create my token
DELETE /enterprise/user/tokens/:id       - Delete my token

# Audit Logs
GET    /enterprise/admin/audit           - List audit logs
GET    /enterprise/admin/audit/:id       - Get audit log details

# System Info
GET    /enterprise/admin/stats           - System statistics
```

## Frontend Components

### Enterprise Pages

```
frontend/src/pages/enterprise/
├── LoginPage.tsx           # Enhanced login with username/password
├── DashboardPage.tsx       # Admin dashboard
├── UsersPage.tsx           # User management
├── UserFormDialog.tsx      # Create/edit user dialog
├── TokensPage.tsx          # Token management (admin view)
├── MyTokensPage.tsx        # Token management (user view)
├── TokenFormDialog.tsx     # Create/edit token dialog
├── AuditLogsPage.tsx       # Audit log viewer
└── SystemStatsPage.tsx     # System statistics
```

### Enterprise Context

```typescript
// EnterpriseContext for enterprise-specific state
interface EnterpriseContextType {
    isEnabled: boolean;
    currentUser: User | null;
    hasPermission: (permission: string) => boolean;
    hasRole: (role: Role) => boolean;
    refreshUser: () => Promise<void>;
}
```

### Enhanced AuthContext

```typescript
// Extended to support enterprise login
interface EnterpriseAuthContextType extends AuthContextType {
    login: (username: string, password: string) => Promise<void>;
    refreshToken: () => Promise<void>;
    currentUser: User | null;
    isEnterprise: boolean;
}
```

## Configuration

### Enterprise Configuration

```go
// EnterpriseConfig holds enterprise-specific settings
type EnterpriseConfig struct {
    Enabled          bool   `json:"enabled"`
    JWTSecret        string `json:"jwt_secret"`
    SessionExpiry    int    `json:"session_expiry_hours"`    // Default: 24
    RefreshExpiry    int    `json:"refresh_expiry_days"`    // Default: 7
    DefaultAdminUser string `json:"default_admin_user"`     // For setup
    DefaultAdminPass string `json:"default_admin_pass"`     // For setup
}
```

### Feature Flag Integration

The enterprise mode is controlled via the existing scenario flag system:

```go
// In config.go
func (c *Config) IsEnterpriseEnabled() bool {
    return c.GetScenarioFlag(typ.ScenarioEnterprise, "enabled")
}
```

## Testing Strategy

### Backend Tests

```
internal/enterprise/
├── auth/
│   ├── service_test.go
│   └── password_test.go
├── user/
│   ├── service_test.go
│   └── repository_test.go
├── token/
│   ├── service_test.go
│   └── repository_test.go
├── rbac/
│   └── middleware_test.go
└── admin/
    └── handler_test.go
```

### Frontend Tests

```
frontend/src/pages/enterprise/
├── __tests__/
│   ├── LoginPage.test.tsx
│   ├── UsersPage.test.tsx
│   ├── TokensPage.test.tsx
│   └── AuditLogsPage.test.tsx
```

### Integration Tests

```
tests/enterprise/
├── authentication_test.go
├── authorization_test.go
├── user_management_test.go
├── token_management_test.go
└── audit_log_test.go
```

## Implementation Phases

### Phase 1: Foundation (Week 1)
- Database schema creation
- Domain models
- Repository layer
- Basic configuration

### Phase 2: Authentication (Week 1-2)
- AuthService implementation
- Password hashing
- JWT token generation
- Session management
- Login/logout endpoints

### Phase 3: Authorization (Week 2)
- RBAC middleware
- Permission checking
- Role-based route protection
- Token scopes

### Phase 4: User Management (Week 2-3)
- UserService implementation
- User CRUD endpoints
- User activation/deactivation
- Password reset flow

### Phase 5: Token Management (Week 3)
- TokenService implementation
- Token CRUD endpoints
- Token validation middleware
- Token expiration handling

### Phase 6: Admin Panel (Week 3-4)
- Admin API handlers
- Admin router setup
- Audit logging
- System statistics

### Phase 7: Frontend (Week 4-5)
- Enhanced login page
- User management UI
- Token management UI
- Audit log viewer
- Admin dashboard

### Phase 8: Testing & Polish (Week 5-6)
- Unit tests
- Integration tests
- E2E tests
- Documentation
- Performance optimization

## Migration Path

### From Community to Enterprise

1. **Enable Enterprise Mode:**
   ```json
   {
     "scenarios": [
       {
         "scenario": "enterprise",
         "extensions": {
           "enabled": true
         }
       }
     ]
   }
   ```

2. **Run Migration:**
   ```bash
   tingly-box migrate-to-enterprise
   ```

3. **Create Admin User:**
   ```bash
   tingly-box create-admin-user --username admin --email admin@example.com
   ```

4. **Access Admin Panel:**
   - Navigate to `/enterprise/admin`
   - Login with admin credentials

### From Enterprise to Community

Enterprise features can be disabled by setting `enabled: false`. Existing data remains in database for potential re-enabling.

## Security Considerations

1. **Password Security:**
   - bcrypt hashing with cost factor 12
   - Minimum password length: 8 characters
   - Optional complexity requirements

2. **Token Security:**
   - Tokens hashed using SHA-256 before storage
   - Only first 8 characters stored in plaintext for identification
   - Automatic expiration of refresh tokens

3. **Session Security:**
   - HTTPS-only cookies for web sessions
   - CSRF protection for state-changing operations
   - Session fixation prevention

4. **Audit Trail:**
   - All administrative actions logged
   - IP address and user agent tracking
   - Failed login attempt logging

5. **Rate Limiting:**
   - Login attempt rate limiting
   - API token rate limiting per user
   - Brute force protection

## Performance Considerations

1. **Database Indexing:**
   - Indexes on frequently queried fields
   - Composite indexes for common query patterns

2. **Caching:**
   - User session caching
   - Permission caching
   - Token validation caching

3. **Connection Pooling:**
   - Reuse existing SQLite connection
   - Proper connection lifecycle management

## Future Enhancements

Out of scope for initial implementation but worth considering:

1. SSO integration (SAML, OIDC)
2. Two-factor authentication (TOTP)
3. LDAP/Active Directory integration
4. OAuth2 for third-party integrations
5. Team/organization support
6. Resource-level permissions
7. Time-based access control
8. IP whitelisting
