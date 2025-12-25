package auth

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTManager manages JWT tokens
type JWTManager struct {
	secretKey string
}

// Claims represents the JWT claims
type Claims struct {
	ClientID string `json:"client_id"`
	jwt.RegisteredClaims
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(secretKey string) *JWTManager {
	return &JWTManager{
		secretKey: secretKey,
	}
}

// GenerateToken generates a new JWT token
func (j *JWTManager) GenerateToken(clientID string) (string, error) {
	claims := &Claims{
		ClientID: clientID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(j.secretKey))
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token and returns the claims
func (j *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(j.secretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

// GenerateAPIKey generates a JWT token and encodes it with tingly-box- prefix
func (j *JWTManager) GenerateAPIKey(clientID string) (string, error) {
	// Generate regular JWT token
	jwtToken, err := j.GenerateToken(clientID)
	if err != nil {
		return "", fmt.Errorf("failed to generate JWT token: %w", err)
	}

	// Encode JWT token to base64
	encodedToken := base64.URLEncoding.EncodeToString([]byte(jwtToken))

	// Remove padding for cleaner format
	encodedToken = strings.TrimRight(encodedToken, "=")

	// Add tingly-box- prefix
	apiKey := "tingly-box-" + encodedToken

	return apiKey, nil
}

// ValidateAPIKey validates an API key with tingly-box- prefix and returns JWT claims
func (j *JWTManager) ValidateAPIKey(tokenString string) (*Claims, error) {
	// Remove "Bearer " prefix if present
	if strings.HasPrefix(tokenString, "Bearer ") {
		tokenString = tokenString[7:]
	}

	// Check if it starts with "tingly-box-"
	if !strings.HasPrefix(tokenString, "tingly-box-") {
		return nil, fmt.Errorf("invalid API key format: must start with 'tingly-box-'")
	}

	// Remove the prefix
	encodedToken := tokenString[len("tingly-box-"):]

	// Add padding back if needed (base64 decoding requires proper padding)
	padding := len(encodedToken) % 4
	if padding != 0 {
		encodedToken += strings.Repeat("=", 4-padding)
	}

	// Decode base64 to get JWT token
	jwtBytes, err := base64.URLEncoding.DecodeString(encodedToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode API key: %w", err)
	}

	jwtToken := string(jwtBytes)

	// Validate the JWT token
	return j.validateJWT(jwtToken)
}

// validateJWT validates a JWT token and returns the claims (internal helper)
func (j *JWTManager) validateJWT(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(j.secretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid JWT token")
	}

	return claims, nil
}

// IsAPIKeyFormat checks if the token follows API key format (tingly-box-xxx)
func (j *JWTManager) IsAPIKeyFormat(tokenString string) bool {
	if strings.HasPrefix(tokenString, "Bearer ") {
		tokenString = tokenString[7:]
	}
	return strings.HasPrefix(tokenString, "tingly-box-")
}
