package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
	"gorm.io/gorm"
)

var (
	// ErrInvalidCredentials is returned when login credentials are invalid
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrUserInactive is returned when trying to authenticate an inactive user
	ErrUserInactive = errors.New("user account is inactive")
	// ErrSessionNotFound is returned when a session is not found
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionExpired is returned when a session has expired
	ErrSessionExpired = errors.New("session has expired")
)

// LoginRequest contains login credentials
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse contains the response data for a successful login
type LoginResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         *db.User  `json:"user"`
}

// RefreshTokenRequest contains the refresh token
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RefreshTokenResponse contains the response data for a successful token refresh
type RefreshTokenResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// AuthService handles authentication operations
type AuthService struct {
	userRepo      db.UserRepository
	sessionRepo   db.SessionRepository
	auditRepo     db.AuditLogRepository
	passwordSvc   *PasswordService
	jwtSvc        *JWTService
	sessionExpiry time.Duration
}

// AuthServiceConfig configures the authentication service
type AuthServiceConfig struct {
	UserRepo      db.UserRepository
	SessionRepo   db.SessionRepository
	AuditRepo     db.AuditLogRepository
	PasswordSvc   *PasswordService
	JWTSvc        *JWTService
	SessionExpiry time.Duration
}

// NewAuthService creates a new authentication service
func NewAuthService(config AuthServiceConfig) *AuthService {
	if config.SessionExpiry == 0 {
		config.SessionExpiry = 24 * time.Hour
	}

	return &AuthService{
		userRepo:      config.UserRepo,
		sessionRepo:   config.SessionRepo,
		auditRepo:     config.AuditRepo,
		passwordSvc:   config.PasswordSvc,
		jwtSvc:        config.JWTSvc,
		sessionExpiry: config.SessionExpiry,
	}
}

// Login authenticates a user and returns tokens
func (s *AuthService) Login(ctx context.Context, req LoginRequest, ipAddress, userAgent string) (*LoginResponse, error) {
	// Get user by username
	user, err := s.userRepo.GetByUsername(req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logAudit(ctx, nil, "user.login", "user", "", nil, ipAddress, userAgent, "failure")
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Check if user is active
	if !user.IsActive {
		s.logAudit(ctx, user, "user.login", "user", user.UUID, nil, ipAddress, userAgent, "failure")
		return nil, ErrUserInactive
	}

	// Validate password
	valid, err := s.passwordSvc.ValidatePassword(req.Password, user.PasswordHash)
	if err != nil {
		return nil, err
	}

	if !valid {
		s.logAudit(ctx, user, "user.login", "user", user.UUID, nil, ipAddress, userAgent, "failure")
		return nil, ErrInvalidCredentials
	}

	// Generate JWT tokens
	accessToken, refreshToken, err := s.jwtSvc.GenerateTokenPair(user)
	if err != nil {
		return nil, err
	}

	// Create session
	sessionModel := db.NewSessionModel(s.sessionRepo)
	session, _, _, err := sessionModel.Create(user.ID, s.sessionExpiry)
	if err != nil {
		return nil, err
	}

	// Update last login
	if err := s.userRepo.UpdateLastLogin(user.ID); err != nil {
		// Log error but don't fail login
	}

	// Log successful login
	s.logAudit(ctx, user, "user.login", "user", user.UUID, map[string]interface{}{
		"session_id": session.UUID,
	}, ipAddress, userAgent, "success")

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(s.jwtSvc.GetTokenExpiry(TokenTypeAccess)),
		User:         user,
	}, nil
}

// RefreshToken refreshes an access token using a refresh token
func (s *AuthService) RefreshToken(ctx context.Context, req RefreshTokenRequest, ipAddress, userAgent string) (*RefreshTokenResponse, error) {
	// Validate refresh token
	claims, err := s.jwtSvc.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Get user
	userID := claims.UserID
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}

	// Check if user is still active
	if !user.IsActive {
		return nil, ErrUserInactive
	}

	// Generate new access token
	accessToken, err := s.jwtSvc.RefreshAccessToken(req.RefreshToken, user)
	if err != nil {
		return nil, err
	}

	return &RefreshTokenResponse{
		AccessToken: accessToken,
		ExpiresAt:   time.Now().Add(s.jwtSvc.GetTokenExpiry(TokenTypeAccess)),
	}, nil
}

// Logout logs out a user by invalidating their session
func (s *AuthService) Logout(ctx context.Context, sessionUUID string, user *db.User, ipAddress, userAgent string) error {
	// Delete session
	if err := s.sessionRepo.DeleteByUUID(sessionUUID); err != nil {
		return err
	}

	// Log logout
	s.logAudit(ctx, user, "user.logout", "user", user.UUID, map[string]interface{}{
		"session_id": sessionUUID,
	}, ipAddress, userAgent, "success")

	return nil
}

// LogoutAll logs out a user from all sessions
func (s *AuthService) LogoutAll(ctx context.Context, user *db.User, ipAddress, userAgent string) error {
	// Delete all user sessions
	if err := s.sessionRepo.DeleteByUserID(user.ID); err != nil {
		return err
	}

	// Log logout all
	s.logAudit(ctx, user, "user.logout_all", "user", user.UUID, nil, ipAddress, userAgent, "success")

	return nil
}

// ValidateSession validates a session token and returns the user
func (s *AuthService) ValidateSession(ctx context.Context, sessionToken string) (*db.User, error) {
	sessionModel := db.NewSessionModel(s.sessionRepo)
	session, err := sessionModel.ValidateSessionToken(sessionToken)
	if err != nil {
		if errors.Is(err, ErrSessionExpired) {
			return nil, ErrSessionExpired
		}
		return nil, ErrSessionNotFound
	}

	// Get user
	user, err := s.userRepo.GetByID(session.UserID)
	if err != nil {
		return nil, err
	}

	// Check if user is still active
	if !user.IsActive {
		return nil, ErrUserInactive
	}

	return user, nil
}

// logAudit logs an audit entry
func (s *AuthService) logAudit(ctx context.Context, user *db.User, action, resourceType, resourceID string, details map[string]interface{}, ipAddress, userAgent, status string) {
	if s.auditRepo == nil {
		return
	}

	var userID *int64
	if user != nil {
		userID = &user.ID
	}

	detailsJSON := ""
	if details != nil {
		// In production, properly serialize to JSON
		// For now, using simple format
		detailsJSON = formatMap(details)
	}

	auditLog := &db.AuditLog{
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      detailsJSON,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Status:       status,
	}

	// Log asynchronously, don't block on errors
	_ = s.auditRepo.Create(auditLog)
}

// formatMap formats a map to a simple string representation
func formatMap(m map[string]interface{}) string {
	if len(m) == 0 {
		return ""
	}

	result := "{"
	for k, v := range m {
		result += k + ":" + formatValue(v) + ","
	}
	result += "}"
	return result
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int, int64, uint, uint64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%f", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return ""
	}
}
