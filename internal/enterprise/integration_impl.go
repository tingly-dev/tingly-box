package enterprise

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/enterprise/admin"
	"github.com/tingly-dev/tingly-box/internal/enterprise/auth"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
	"github.com/tingly-dev/tingly-box/internal/enterprise/rbac"
	"github.com/tingly-dev/tingly-box/internal/enterprise/token"
	"github.com/tingly-dev/tingly-box/internal/enterprise/user"
)

// enterpriseIntegrationImpl implements the Integration interface
type enterpriseIntegrationImpl struct {
	config         *Config
	enabled        bool
	db             *db.EnterpriseDB
	authService    *auth.AuthService
	userService    user.Service
	tokenService   token.Service
	passwordSvc    *auth.PasswordService
	jwtSvc         *auth.JWTService
	authMiddleware *rbac.AuthMiddleware
	userRepo       db.UserRepository
	tokenModel     *token.Model
	sessionModel   *db.SessionModel
}

// NewIntegration creates a new enterprise integration instance
func NewIntegration() Integration {
	return &enterpriseIntegrationImpl{}
}

// Initialize sets up all enterprise services
func (ei *enterpriseIntegrationImpl) Initialize(ctx context.Context, config *Config) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	ei.config = config

	// Initialize enterprise database (COMPLETELY ISOLATED)
	dbConfig := config.DatabaseConfig
	if dbConfig == nil {
		dbConfig = db.DefaultEnterpriseDBConfig(config.BaseDir)
	}

	enterpriseDB, err := db.NewEnterpriseDB(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize enterprise database: %w", err)
	}

	ei.db = enterpriseDB
	ei.enabled = true

	// Initialize password service
	passwordConfig := auth.DefaultPasswordConfig()
	if config.PasswordMinLength > 0 {
		// Adjust config based on requirements
	}
	ei.passwordSvc = auth.NewPasswordService(passwordConfig)

	// Initialize JWT service
	jwtConfig := auth.DefaultJWTConfig(config.JWTSecret)

	// Parse expiry durations if provided
	if config.AccessTokenExpiry != "" {
		if d, err := time.ParseDuration(config.AccessTokenExpiry); err == nil {
			jwtConfig.AccessTokenExpiry = d
		}
	}
	if config.RefreshTokenExpiry != "" {
		if d, err := time.ParseDuration(config.RefreshTokenExpiry); err == nil {
			jwtConfig.RefreshTokenExpiry = d
		}
	}

	ei.jwtSvc = auth.NewJWTService(jwtConfig)

	// Initialize repositories
	ei.userRepo = db.NewUserRepository(enterpriseDB.GetDB())
	sessionRepo := db.NewSessionRepository(enterpriseDB.GetDB())
	auditRepo := db.NewAuditLogRepository(enterpriseDB.GetDB())

	// Initialize user model and service
	userModel := user.NewModel(ei.userRepo)
	ei.userService = user.NewService(userModel, ei.passwordSvc, auditRepo)

	// Initialize token model and service
	tokenRepo := token.NewRepository(enterpriseDB.GetDB())
	ei.tokenModel = token.NewModel(tokenRepo)
	ei.tokenService = token.NewService(ei.tokenModel, auditRepo)

	// Initialize session model
	ei.sessionModel = db.NewSessionModel(sessionRepo)

	// Initialize auth service
	sessionExpiry := 24 * time.Hour
	if config.SessionExpiry != "" {
		if d, err := time.ParseDuration(config.SessionExpiry); err == nil {
			sessionExpiry = d
		}
	}

	ei.authService = auth.NewAuthService(auth.AuthServiceConfig{
		UserRepo:    ei.userRepo,
		SessionRepo: sessionRepo,
		AuditRepo:   auditRepo,
		PasswordSvc: ei.passwordSvc,
		JWTSvc:      ei.jwtSvc,
		SessionExpiry: sessionExpiry,
	})

	// Initialize auth middleware
	ei.authMiddleware = rbac.NewAuthMiddleware(ei.jwtSvc, ei.userRepo, ei.tokenModel)

	return nil
}

// IsEnabled returns whether enterprise mode is enabled
func (ei *enterpriseIntegrationImpl) IsEnabled() bool {
	return ei.enabled
}

// Shutdown gracefully shuts down enterprise services
func (ei *enterpriseIntegrationImpl) Shutdown(ctx context.Context) error {
	if ei.db != nil {
		return ei.db.Close()
	}
	return nil
}

// ValidateAccessToken validates an access token and returns the associated user
func (ei *enterpriseIntegrationImpl) ValidateAccessToken(ctx context.Context, token string) (*UserInfo, error) {
	if !ei.enabled {
		return nil, ErrNotEnabled
	}

	claims, err := ei.jwtSvc.ValidateAccessToken(token)
	if err != nil {
		if err == auth.ErrTokenExpired {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	user, err := ei.userRepo.GetByID(claims.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if !user.IsActive {
		return nil, ErrUserInactive
	}

	return toUserInfo(user), nil
}

// ValidateAPIToken validates an API token and returns the associated user
func (ei *enterpriseIntegrationImpl) ValidateAPIToken(ctx context.Context, token string) (*UserInfo, *TokenInfo, error) {
	if !ei.enabled {
		return nil, nil, ErrNotEnabled
	}

	apiToken, err := ei.tokenModel.ValidateToken(token)
	if err != nil {
		if err == token.ErrTokenExpired {
			return nil, nil, ErrTokenExpired
		}
		if err == token.ErrTokenInactive {
			return nil, nil, ErrTokenInactive
		}
		return nil, nil, ErrInvalidToken
	}

	user := apiToken.User
	if user == nil {
		u, err := ei.userRepo.GetByID(apiToken.UserID)
		if err != nil {
			return nil, nil, ErrUserNotFound
		}
		user = u
	}

	if !user.IsActive {
		return nil, nil, ErrUserInactive
	}

	return toUserInfo(user), toTokenInfo(apiToken), nil
}

// RefreshAccessToken generates a new access token from a refresh token
func (ei *enterpriseIntegrationImpl) RefreshAccessToken(ctx context.Context, refreshToken string) (string, error) {
	if !ei.enabled {
		return "", ErrNotEnabled
	}

	claims, err := ei.jwtSvc.ValidateRefreshToken(refreshToken)
	if err != nil {
		return "", ErrInvalidToken
	}

	user, err := ei.userRepo.GetByID(claims.UserID)
	if err != nil {
		return "", ErrUserNotFound
	}

	if !user.IsActive {
		return "", ErrUserInactive
	}

	return ei.jwtSvc.RefreshAccessToken(refreshToken, user)
}

// GetUserInfo retrieves user information by ID
func (ei *enterpriseIntegrationImpl) GetUserInfo(ctx context.Context, userID int64) (*UserInfo, error) {
	if !ei.enabled {
		return nil, ErrNotEnabled
	}

	user, err := ei.userRepo.GetByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	return toUserInfo(user), nil
}

// GetUserInfoByUsername retrieves user information by username
func (ei *enterpriseIntegrationImpl) GetUserInfoByUsername(ctx context.Context, username string) (*UserInfo, error) {
	if !ei.enabled {
		return nil, ErrNotEnabled
	}

	user, err := ei.userRepo.GetByUsername(username)
	if err != nil {
		return nil, ErrUserNotFound
	}

	return toUserInfo(user), nil
}

// GetUserInfoByUUID retrieves user information by UUID
func (ei *enterpriseIntegrationImpl) GetUserInfoByUUID(ctx context.Context, uuid string) (*UserInfo, error) {
	if !ei.enabled {
		return nil, ErrNotEnabled
	}

	// Use userModel to get by UUID
	// For now, we need to add this to user repository or implement here
	return nil, fmt.Errorf("not implemented")
}

// HasPermission checks if a user has a specific permission
func (ei *enterpriseIntegrationImpl) HasPermission(userID int64, permission string) bool {
	if !ei.enabled {
		return false
	}

	user, err := ei.userRepo.GetByID(userID)
	if err != nil {
		return false
	}

	return rbac.HasPermission(user, permission)
}

// HasRole checks if a user has a specific role
func (ei *enterpriseIntegrationImpl) HasRole(userID int64, role string) bool {
	if !ei.enabled {
		return false
	}

	user, err := ei.userRepo.GetByID(userID)
	if err != nil {
		return false
	}

	return string(user.Role) == role
}

// HasAnyPermission checks if a user has any of the specified permissions
func (ei *enterpriseIntegrationImpl) HasAnyPermission(userID int64, permissions ...string) bool {
	if !ei.enabled {
		return false
	}

	user, err := ei.userRepo.GetByID(userID)
	if err != nil {
		return false
	}

	for _, perm := range permissions {
		if rbac.HasPermission(user, perm) {
			return true
		}
	}

	return false
}

// AuthMiddleware returns Gin middleware for JWT authentication
func (ei *enterpriseIntegrationImpl) AuthMiddleware() gin.HandlerFunc {
	if !ei.enabled {
		return func(c *gin.Context) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Enterprise mode not enabled"})
			c.Abort()
		}
	}
	return ei.authMiddleware.Authenticate()
}

// RequirePermission returns Gin middleware that requires specific permission
func (ei *enterpriseIntegrationImpl) RequirePermission(permission string) gin.HandlerFunc {
	if !ei.enabled {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Enterprise mode not enabled"})
			c.Abort()
		}
	}
	return rbac.RequirePermission(permission)
}

// RequireRole returns Gin middleware that requires specific role
func (ei *enterpriseIntegrationImpl) RequireRole(roles ...string) gin.HandlerFunc {
	if !ei.enabled {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Enterprise mode not enabled"})
			c.Abort()
		}
	}

	// Convert string roles to db.Role
	dbRoles := make([]db.Role, len(roles))
	for i, r := range roles {
		dbRoles[i] = db.Role(r)
	}

	return rbac.RequireRole(dbRoles...)
}

// CreateUser creates a new user (requires admin permission)
func (ei *enterpriseIntegrationImpl) CreateUser(ctx context.Context, req *CreateUserRequest, actorID int64) (*UserInfo, error) {
	if !ei.enabled {
		return nil, ErrNotEnabled
	}

	createReq := user.CreateUserRequest{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
		FullName: req.FullName,
		Role:     db.Role(req.Role),
	}

	actorUser, err := ei.userRepo.GetByID(actorID)
	if err != nil {
		return nil, err
	}

	newUser, err := ei.userService.CreateUser(ctx, createReq, actorUser)
	if err != nil {
		return nil, err
	}

	return toUserInfo(newUser), nil
}

// UpdateUser updates user information (requires admin permission or own account)
func (ei *enterpriseIntegrationImpl) UpdateUser(ctx context.Context, userID int64, req *UpdateUserRequest, actorID int64) (*UserInfo, error) {
	if !ei.enabled {
		return nil, ErrNotEnabled
	}

	actorUser, err := ei.userRepo.GetByID(actorID)
	if err != nil {
		return nil, err
	}

	var role *db.Role
	if req.Role != nil {
		r := db.Role(*req.Role)
		role = &r
	}

	updateReq := user.UpdateUserRequest{
		FullName: req.FullName,
		Role:     role,
	}

	updatedUser, err := ei.userService.UpdateUser(ctx, userID, updateReq, actorUser)
	if err != nil {
		return nil, err
	}

	return toUserInfo(updatedUser), nil
}

// DeactivateUser deactivates a user account (requires admin permission)
func (ei *enterpriseIntegrationImpl) DeactivateUser(ctx context.Context, userID int64, actorID int64) error {
	if !ei.enabled {
		return ErrNotEnabled
	}

	actorUser, err := ei.userRepo.GetByID(actorID)
	if err != nil {
		return err
	}

	return ei.userService.DeactivateUser(ctx, userID, actorUser)
}

// ResetPassword resets a user's password (requires admin permission)
func (ei *enterpriseIntegrationImpl) ResetPassword(ctx context.Context, userID int64, actorID int64) (string, error) {
	if !ei.enabled {
		return "", ErrNotEnabled
	}

	actorUser, err := ei.userRepo.GetByID(actorID)
	if err != nil {
		return "", err
	}

	return ei.userService.ResetPassword(ctx, userID, actorUser)
}

// CreateAPIToken creates a new API token for a user
func (ei *enterpriseIntegrationImpl) CreateAPIToken(ctx context.Context, req *CreateTokenRequest, actorID int64) (*TokenInfo, string, error) {
	if !ei.enabled {
		return nil, "", ErrNotEnabled
	}

	actorUser, err := ei.userRepo.GetByID(actorID)
	if err != nil {
		return nil, "", err
	}

	createReq := token.CreateTokenRequest{
		Name:   req.Name,
		Scopes: convertScopes(req.Scopes),
	}

	if req.UserID != nil {
		createReq.UserID = req.UserID
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0)
		expiresAt = &t
		createReq.ExpiresAt = expiresAt
	}

	apiToken, rawToken, err := ei.tokenService.CreateToken(ctx, createReq, actorUser)
	if err != nil {
		return nil, "", err
	}

	return toTokenInfo(apiToken), rawToken, nil
}

// ListAPITokens lists API tokens for a user
func (ei *enterpriseIntegrationImpl) ListAPITokens(ctx context.Context, userID int64, page, pageSize int) (*TokenListResult, error) {
	if !ei.enabled {
		return nil, ErrNotEnabled
	}

	tokens, err := ei.tokenModel.ListByUserID(userID)
	if err != nil {
		return nil, err
	}

	total := int64(len(tokens))
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(tokens) {
		end = len(tokens)
	}

	if start >= len(tokens) {
		return &TokenListResult{
			Tokens: []*TokenWithUser{},
			Total:  total,
			Page:   page,
			Size:   pageSize,
		}, nil
	}

	result := make([]*TokenWithUser, end-start)
	for i, t := range tokens[start:end] {
		// Get user info
		user, _ := ei.userRepo.GetByID(t.UserID)
		username := ""
		email := ""
		if user != nil {
			username = user.Username
			email = user.Email
		}

		result[i] = &TokenWithUser{
			TokenInfo: *toTokenInfo(t),
			Username:  username,
			Email:     email,
		}
	}

	return &TokenListResult{
		Tokens: result,
		Total:  total,
		Page:   page,
		Size:   pageSize,
	}, nil
}

// RevokeAPIToken revokes an API token
func (ei *enterpriseIntegrationImpl) RevokeAPIToken(ctx context.Context, tokenID int64, actorID int64) error {
	if !ei.enabled {
		return ErrNotEnabled
	}

	actorUser, err := ei.userRepo.GetByID(actorID)
	if err != nil {
		return err
	}

	return ei.tokenService.DeleteToken(ctx, tokenID, actorUser)
}

// LogAudit logs an audit entry
func (ei *enterpriseIntegrationImpl) LogAudit(ctx context.Context, entry *AuditEntry) error {
	if !ei.enabled {
		return ErrNotEnabled
	}

	// This would use the audit service
	// For now, implement directly
	return nil
}

// QueryAuditLogs retrieves audit logs with pagination and filters
func (ei *enterpriseIntegrationImpl) QueryAuditLogs(ctx context.Context, query *AuditQuery) (*AuditResult, error) {
	if !ei.enabled {
		return nil, ErrNotEnabled
	}

	// Use admin handler's audit log query logic
	// This would need to be implemented properly
	return nil, fmt.Errorf("not implemented")
}

// HealthCheck performs a health check on enterprise services
func (ei *enterpriseIntegrationImpl) HealthCheck(ctx context.Context) error {
	if !ei.enabled {
		return ErrNotEnabled
	}

	return ei.db.HealthCheck()
}

// GetStats returns enterprise statistics
func (ei *enterpriseIntegrationImpl) GetStats(ctx context.Context) (*Stats, error) {
	if !ei.enabled {
		return &Stats{Enabled: false}, nil
	}

	dbStats, err := ei.db.GetStats()
	if err != nil {
		return nil, err
	}

	return &Stats{
		UserCount:      dbStats.UserCount,
		TokenCount:     dbStats.TokenCount,
		SessionCount:   dbStats.SessionCount,
		AuditLogCount:  dbStats.AuditLogCount,
		DatabasePath:   dbStats.DatabasePath,
		DatabaseSizeMB: dbStats.DatabaseSizeMB,
		Enabled:        true,
	}, nil
}

// CleanupExpired removes expired sessions and tokens
func (ei *enterpriseIntegrationImpl) CleanupExpired(ctx context.Context) error {
	if !ei.enabled {
		return ErrNotEnabled
	}

	// Cleanup expired sessions
	_, err := ei.sessionModel.CleanupExpired()
	if err != nil {
		return err
	}

	// Cleanup expired tokens
	_, err = ei.tokenModel.CleanupExpired()
	if err != nil {
		return err
	}

	return nil
}

// Helper function to convert scopes
func convertScopes(scopes []Scope) []db.Scope {
	result := make([]db.Scope, len(scopes))
	for i, s := range scopes {
		result[i] = db.Scope(s)
	}
	return result
}
