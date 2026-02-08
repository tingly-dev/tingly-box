package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
)

var (
	// ErrInvalidToken is returned when a token is invalid
	ErrInvalidToken = errors.New("invalid token")
	// ErrTokenExpired is returned when a token has expired
	ErrTokenExpired = errors.New("token expired")
)

// JWTClaims represents the JWT claims for enterprise authentication
type JWTClaims struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// TokenType defines the type of JWT token
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// JWTConfig configures JWT token generation
type JWTConfig struct {
	Secret              []byte
	AccessTokenExpiry   time.Duration
	RefreshTokenExpiry  time.Duration
	Issuer              string
}

// DefaultJWTConfig returns the default JWT configuration
func DefaultJWTConfig(secret string) JWTConfig {
	return JWTConfig{
		Secret:              []byte(secret),
		AccessTokenExpiry:   15 * time.Minute,
		RefreshTokenExpiry:  7 * 24 * time.Hour, // 7 days
		Issuer:              "tingly-box-enterprise",
	}
}

// JWTService handles JWT token generation and validation
type JWTService struct {
	config JWTConfig
}

// NewJWTService creates a new JWT service
func NewJWTService(config JWTConfig) *JWTService {
	return &JWTService{config: config}
}

// GenerateAccessToken generates an access token for a user
func (s *JWTService) GenerateAccessToken(user *db.User) (string, error) {
	return s.generateToken(user, TokenTypeAccess, s.config.AccessTokenExpiry)
}

// GenerateRefreshToken generates a refresh token for a user
func (s *JWTService) GenerateRefreshToken(user *db.User) (string, error) {
	return s.generateToken(user, TokenTypeRefresh, s.config.RefreshTokenExpiry)
}

// GenerateTokenPair generates both access and refresh tokens
func (s *JWTService) GenerateTokenPair(user *db.User) (accessToken, refreshToken string, err error) {
	accessToken, err = s.GenerateAccessToken(user)
	if err != nil {
		return "", "", err
	}

	refreshToken, err = s.GenerateRefreshToken(user)
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

// ValidateToken validates a JWT token and returns the claims
func (s *JWTService) ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.config.Secret, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Check issuer
	if claims.Issuer != s.config.Issuer {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateAccessToken validates an access token
func (s *JWTService) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != string(TokenTypeAccess) {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token
func (s *JWTService) ValidateRefreshToken(tokenString string) (*JWTClaims, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != string(TokenTypeRefresh) {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// RefreshAccessToken generates a new access token from a valid refresh token
func (s *JWTService) RefreshAccessToken(refreshToken string, user *db.User) (string, error) {
	// Validate the refresh token
	claims, err := s.ValidateRefreshToken(refreshToken)
	if err != nil {
		return "", err
	}

	// Verify the user ID matches
	if claims.UserID != user.ID {
		return "", ErrInvalidToken
	}

	// Generate new access token
	return s.GenerateAccessToken(user)
}

// generateToken generates a JWT token with the specified type and expiry
func (s *JWTService) generateToken(user *db.User, tokenType TokenType, expiry time.Duration) (string, error) {
	now := time.Now()

	claims := &JWTClaims{
		UserID:    user.ID,
		Username:  user.Username,
		Role:      string(user.Role),
		TokenType: string(tokenType),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   user.UUID,
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.config.Secret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// GetTokenExpiry returns the expiry duration for a token type
func (s *JWTService) GetTokenExpiry(tokenType TokenType) time.Duration {
	switch tokenType {
	case TokenTypeAccess:
		return s.config.AccessTokenExpiry
	case TokenTypeRefresh:
		return s.config.RefreshTokenExpiry
	default:
		return s.config.AccessTokenExpiry
	}
}
