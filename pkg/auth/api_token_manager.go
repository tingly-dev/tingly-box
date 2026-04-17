package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// APITokenClaims represents the JWT claims for API tokens
type APITokenClaims struct {
	UserUUID string `json:"user_uuid"`
	TokenID  string `json:"token_id"` // jti - for revocation check
	jwt.RegisteredClaims
}

// APITokenManager handles JWT token generation and validation for multi-tenant API tokens
type APITokenManager struct {
	secretKey     []byte
	signingMethod string // Store as string to avoid type issues
	issuer        string
}

// APITokenManagerConfig holds configuration for APITokenManager
type APITokenManagerConfig struct {
	SecretKey     string
	SigningMethod string // "HS256" or "RS256"
	Issuer        string // Default: "tingly-box"
}

// NewAPITokenManager creates a new API token manager
func NewAPITokenManager(config APITokenManagerConfig) (*APITokenManager, error) {
	if config.SecretKey == "" {
		return nil, fmt.Errorf("secret key cannot be empty")
	}

	// Validate signing method
	signingMethod := config.SigningMethod
	if signingMethod == "" {
		signingMethod = "HS256"
	}
	if signingMethod != "HS256" && signingMethod != "RS256" {
		return nil, fmt.Errorf("unsupported signing method: %s (must be HS256 or RS256)", signingMethod)
	}

	// Default issuer
	issuer := config.Issuer
	if issuer == "" {
		issuer = "tingly-box"
	}

	return &APITokenManager{
		secretKey:     []byte(config.SecretKey),
		signingMethod: signingMethod,
		issuer:        issuer,
	}, nil
}

// getSigningMethod returns the jwt.SigningMethod based on the algorithm string
func (m *APITokenManager) getSigningMethod() jwt.SigningMethod {
	switch m.signingMethod {
	case "RS256":
		return jwt.SigningMethodRS256
	default:
		return jwt.SigningMethodHS256
	}
}

// GenerateToken generates a JWT token for a user
func (m *APITokenManager) GenerateToken(userUUID, tokenID string, expiresAt time.Time) (string, error) {
	if userUUID == "" {
		return "", fmt.Errorf("user UUID cannot be empty")
	}
	if tokenID == "" {
		return "", fmt.Errorf("token ID cannot be empty")
	}

	claims := &APITokenClaims{
		UserUUID: userUUID,
		TokenID:  tokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userUUID,
			ID:        tokenID, // jti
			Issuer:    m.issuer,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(m.getSigningMethod(), claims)
	tokenString, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token and returns claims
func (m *APITokenManager) ValidateToken(tokenString string) (*APITokenClaims, error) {
	expectedMethod := m.getSigningMethod()

	token, err := jwt.ParseWithClaims(tokenString, &APITokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if token.Method.Alg() != expectedMethod.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*APITokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Validate required claims
	if claims.UserUUID == "" {
		return nil, fmt.Errorf("token missing user_uuid claim")
	}
	if claims.TokenID == "" {
		return nil, fmt.Errorf("token missing token_id claim")
	}

	return claims, nil
}

// GetIssuer returns the configured issuer
func (m *APITokenManager) GetIssuer() string {
	return m.issuer
}

